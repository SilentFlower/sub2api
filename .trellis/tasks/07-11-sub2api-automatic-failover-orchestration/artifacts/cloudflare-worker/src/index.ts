import {
  CoordinatorState,
  ControlMode,
  EntryTunnel,
  HaError,
  HaState,
  NodeId,
  NodeReport,
  StateEvent,
  StateResult,
  createInitialState,
  isLeaseExpired,
} from "./model";
import {
  acquireForB,
  advanceTransition,
  bootstrapA,
  commitHandoffToA,
  createEvent,
  emergencyFreeze,
  ownerReportHealthy,
  renewLease,
  resumeFromPause,
  setControlMode,
  updateFailbackStability,
} from "./state-machine";

/** Worker 运行环境。 */
export interface Env {
  COORDINATOR: DurableObjectNamespace;
  NODE_A_SECRET: string;
  NODE_B_SECRET: string;
  ADMIN_TOKEN: string;
  CLOUDFLARE_API_TOKEN: string;
  CLOUDFLARE_ZONE_ID: string;
  CLOUDFLARE_DNS_RECORD_ID: string;
  A_TUNNEL_TARGET: string;
  B_TUNNEL_TARGET: string;
  DINGTALK_WEBHOOK_TOKEN?: string;
  API_HOSTNAME: string;
  LEASE_TTL_SECONDS: string;
  CONTROL_REQUEST_DAILY_LIMIT: string;
}

interface DailyUsage {
  date: string;
  count: number;
  warningLevel: number;
}

interface JsonEnvelope<T = unknown> {
  ok: boolean;
  data?: T;
  alerts?: StateEvent[];
  error?: {
    code: string;
    message: string;
  };
}

interface EntrySwitchRequest {
  target: NodeId;
  epoch: number;
  requestId: string;
}

const STATE_KEY = "coordinator-state";
const USAGE_KEY = "daily-usage";
const REPORT_PREFIX = "node-report:";
const NONCE_PREFIX = "nonce-cache:";

/** 返回 JSON 响应。 */
function jsonResponse(value: unknown, status = 200): Response {
  return Response.json(value, {
    status,
    headers: { "cache-control": "no-store" },
  });
}

/** 把未知异常转换为稳定错误响应。 */
function errorResponse(error: unknown): Response {
  if (error instanceof HaError) {
    return jsonResponse(
      { ok: false, error: { code: error.code, message: error.message } } satisfies JsonEnvelope,
      error.status,
    );
  }
  return jsonResponse(
    { ok: false, error: { code: "INTERNAL_ERROR", message: "控制面内部错误" } } satisfies JsonEnvelope,
    500,
  );
}

/** 读取并校验 JSON 请求体。 */
async function readJson<T>(request: Request): Promise<{ raw: string; value: T }> {
  const raw = await request.text();
  if (raw.length === 0) {
    return { raw, value: {} as T };
  }
  try {
    return { raw, value: JSON.parse(raw) as T };
  } catch {
    throw new HaError("INVALID_JSON", "请求体不是有效 JSON", 400);
  }
}

/** 使用 HMAC-SHA256 计算十六进制签名。 */
async function hmacHex(secret: string, message: string): Promise<string> {
  const encoder = new TextEncoder();
  const key = await crypto.subtle.importKey(
    "raw",
    encoder.encode(secret),
    { name: "HMAC", hash: "SHA-256" },
    false,
    ["sign"],
  );
  const signature = await crypto.subtle.sign("HMAC", key, encoder.encode(message));
  return [...new Uint8Array(signature)].map((value) => value.toString(16).padStart(2, "0")).join("");
}

/** 使用常量时间比较两个 ASCII 签名。 */
function constantTimeEqual(left: string, right: string): boolean {
  if (left.length !== right.length) {
    return false;
  }
  let difference = 0;
  for (let index = 0; index < left.length; index += 1) {
    difference |= left.charCodeAt(index) ^ right.charCodeAt(index);
  }
  return difference === 0;
}

/** 校验节点签名并返回节点身份。 */
async function authenticateNode(
  request: Request,
  env: Env,
  rawBody: string,
): Promise<{ node: NodeId; nonce: string; timestamp: string }> {
  const node = request.headers.get("x-ha-node");
  const timestamp = request.headers.get("x-ha-timestamp") ?? "";
  const nonce = request.headers.get("x-ha-nonce") ?? "";
  const supplied = request.headers.get("x-ha-signature") ?? "";
  if (node !== "A" && node !== "B") {
    throw new HaError("INVALID_NODE", "缺少有效节点身份", 401);
  }
  const timestampMs = Date.parse(timestamp);
  if (!Number.isFinite(timestampMs) || Math.abs(Date.now() - timestampMs) > 60_000) {
    throw new HaError("STALE_REQUEST", "请求时间超过允许窗口", 401);
  }
  if (!/^[A-Za-z0-9_-]{16,128}$/.test(nonce)) {
    throw new HaError("INVALID_NONCE", "请求 nonce 格式无效", 401);
  }
  const secret = node === "A" ? env.NODE_A_SECRET : env.NODE_B_SECRET;
  if (!secret) {
    throw new HaError("NODE_SECRET_MISSING", "节点密钥尚未配置", 503);
  }
  const url = new URL(request.url);
  const payload = [request.method, url.pathname, timestamp, nonce, rawBody].join("\n");
  const expected = await hmacHex(secret, payload);
  if (!constantTimeEqual(expected, supplied.toLowerCase())) {
    throw new HaError("INVALID_SIGNATURE", "节点签名验证失败", 401);
  }
  return { node, nonce, timestamp };
}

/** 校验管理员 Bearer Token。 */
function authenticateAdmin(request: Request, env: Env): void {
  const token = request.headers.get("authorization")?.replace(/^Bearer\s+/i, "") ?? "";
  if (!env.ADMIN_TOKEN || !constantTimeEqual(env.ADMIN_TOKEN, token)) {
    throw new HaError("ADMIN_UNAUTHORIZED", "管理员认证失败", 401);
  }
}

/** 获取唯一协调 Durable Object。 */
function coordinatorStub(env: Env): DurableObjectStub {
  return env.COORDINATOR.get(env.COORDINATOR.idFromName("sub2api-production"));
}

/**
 * 把 UTC 时间转换为固定的 Asia/Shanghai 展示文本。
 * @param value ISO-8601 时间。
 * @return 上海时区时间；无效输入原样返回。
 */
function formatAsiaShanghai(value: string): string {
  const timestamp = Date.parse(value);
  if (!Number.isFinite(timestamp)) {
    return value;
  }
  const local = new Date(timestamp + 8 * 60 * 60 * 1000);
  const pad = (part: number): string => String(part).padStart(2, "0");
  return `${local.getUTCFullYear()}-${pad(local.getUTCMonth() + 1)}-${pad(local.getUTCDate())} ` +
    `${pad(local.getUTCHours())}:${pad(local.getUTCMinutes())}:${pad(local.getUTCSeconds())} Asia/Shanghai`;
}

/**
 * 发送钉钉 Markdown 告警。
 * @param event 待发送状态事件。
 * @param env Worker 运行环境。
 * @return 发送、跳过或失败状态。
 */
export async function sendDingTalk(event: StateEvent, env: Env): Promise<"sent" | "skipped" | "failed"> {
  if (!env.DINGTALK_WEBHOOK_TOKEN) {
    return "skipped";
  }
  const atAll = event.level === "CRITICAL";
  const suffix = atAll ? "\n\n@10" : "";
  const text = [
    `### [${event.level}] Sub2API HA`,
    "",
    `- 权威节点：${event.authorityOwner}`,
    `- epoch：${event.epoch}`,
    `- 状态：${event.from} -> ${event.to}`,
    `- 原因：${event.reason}`,
    `- 结果：${event.result}`,
    `- 人工动作：${event.operatorAction}`,
    `- 时间：${formatAsiaShanghai(event.occurredAt)}`,
  ].join("\n") + suffix;
  const requestBody = JSON.stringify({
    msgtype: "markdown",
    markdown: { title: `[${event.level}] Sub2API HA`, text },
    at: { isAtAll: atAll },
  });
  for (let attempt = 1; attempt <= 3; attempt += 1) {
    try {
      const response = await fetch(
        `https://oapi.dingtalk.com/robot/send?access_token=${encodeURIComponent(env.DINGTALK_WEBHOOK_TOKEN)}`,
        {
          method: "POST",
          headers: { "content-type": "application/json" },
          body: requestBody,
        },
      );
      const result = await response.json() as { errcode?: number };
      if (response.ok && result.errcode === 0) {
        return "sent";
      }
    } catch {
      // 钉钉不可达不能阻塞租约和状态提交，只做有限次数重试。
    }
  }
  return "failed";
}

/** 为 Worker 内部两阶段调用派生不同 nonce。 */
function derivedInternalHeaders(headers: Headers, suffix: string): Headers {
  const derived = new Headers(headers);
  const nonce = derived.get("x-ha-nonce");
  if (nonce) {
    derived.set("x-ha-nonce", `${nonce}-${suffix}`);
  }
  return derived;
}

/** 更新 Cloudflare DNS CNAME 到目标 Tunnel。 */
async function updateEntryDns(target: NodeId, env: Env): Promise<void> {
  const required = [
    env.CLOUDFLARE_API_TOKEN,
    env.CLOUDFLARE_ZONE_ID,
    env.CLOUDFLARE_DNS_RECORD_ID,
    env.A_TUNNEL_TARGET,
    env.B_TUNNEL_TARGET,
    env.API_HOSTNAME,
  ];
  if (required.some((value) => !value)) {
    throw new HaError("DNS_CONFIG_MISSING", "Cloudflare DNS 配置尚未完成", 503);
  }
  const endpoint = `https://api.cloudflare.com/client/v4/zones/${env.CLOUDFLARE_ZONE_ID}/dns_records/${env.CLOUDFLARE_DNS_RECORD_ID}`;
  const headers = {
    authorization: `Bearer ${env.CLOUDFLARE_API_TOKEN}`,
    "content-type": "application/json",
  };
  const currentResponse = await fetch(endpoint, { headers });
  if (!currentResponse.ok) {
    throw new HaError("DNS_READ_FAILED", "读取 Cloudflare DNS 记录失败", 502);
  }
  const currentPayload = (await currentResponse.json()) as {
    success: boolean;
    result?: { type: string; name: string; content: string };
  };
  const targetContent = target === "A" ? env.A_TUNNEL_TARGET : env.B_TUNNEL_TARGET;
  const allowedContents = new Set([env.A_TUNNEL_TARGET, env.B_TUNNEL_TARGET, targetContent]);
  if (
    !currentPayload.success ||
    currentPayload.result?.type !== "CNAME" ||
    currentPayload.result.name !== env.API_HOSTNAME ||
    !allowedContents.has(currentPayload.result.content)
  ) {
    throw new HaError("DNS_RECORD_DRIFT", "公共入口 DNS 记录与 HA 配置不一致");
  }
  if (currentPayload.result.content === targetContent) {
    return;
  }
  const updateResponse = await fetch(endpoint, {
    method: "PUT",
    headers,
    body: JSON.stringify({
      type: "CNAME",
      name: env.API_HOSTNAME,
      content: targetContent,
      proxied: true,
      ttl: 1,
    }),
  });
  if (!updateResponse.ok) {
    throw new HaError("DNS_UPDATE_FAILED", "切换 Cloudflare HA Tunnel 入口失败", 502);
  }
}

/** 转发到 Durable Object，并在状态提交后异步发送告警。 */
async function forwardAndAlert(request: Request, env: Env, ctx: ExecutionContext): Promise<Response> {
  const response = await coordinatorStub(env).fetch(request);
  const envelope = (await response.json()) as JsonEnvelope<StateResult | CoordinatorState>;
  const alerts = envelope.alerts ?? [];
  if (alerts.length > 0) {
    ctx.waitUntil(Promise.all(alerts.map((event) => sendDingTalk(event, env))).then(() => undefined));
  }
  return jsonResponse(
    alerts.length > 0 ? { ...envelope, alertDeliveries: alerts.map(() => "queued") } : envelope,
    response.status,
  );
}

/** 处理节点或管理员 API。 */
async function handleWorkerRequest(request: Request, env: Env, ctx: ExecutionContext): Promise<Response> {
  const url = new URL(request.url);
  const { raw, value } = await readJson<Record<string, unknown>>(request);
  let internalPath = url.pathname;
  const headers = new Headers({ "content-type": "application/json" });

  if (url.pathname.startsWith("/v1/control/") || url.pathname === "/v1/bootstrap") {
    authenticateAdmin(request, env);
    headers.set("x-ha-admin", "authenticated");
  } else {
    const identity = await authenticateNode(request, env, raw);
    headers.set("x-ha-node", identity.node);
    headers.set("x-ha-nonce", identity.nonce);
    headers.set("x-ha-timestamp", identity.timestamp);
  }

  if (url.pathname === "/v1/entry/switch") {
    const body = value as unknown as EntrySwitchRequest;
    if (body.target !== "A" && body.target !== "B") {
      throw new HaError("INVALID_ENTRY_TARGET", "入口目标必须是 A 或 B", 400);
    }
    const authorizeRequest = new Request("https://coordinator/internal/entry/authorize", {
      method: "POST",
      headers: derivedInternalHeaders(headers, "entry-authorize"),
      body: JSON.stringify(body),
    });
    const authorizeResponse = await forwardAndAlert(authorizeRequest, env, ctx);
    if (!authorizeResponse.ok) {
      return authorizeResponse;
    }
    await updateEntryDns(body.target, env);
    const commitRequest = new Request("https://coordinator/internal/entry/commit", {
      method: "POST",
      headers: derivedInternalHeaders(headers, "entry-commit"),
      body: JSON.stringify(body),
    });
    return forwardAndAlert(commitRequest, env, ctx);
  }

  const mapping: Readonly<Record<string, string>> = {
    "/v1/status": "/internal/status",
    "/v1/bootstrap": "/internal/bootstrap",
    "/v1/node/report": "/internal/report",
    "/v1/node/alert": "/internal/alert",
    "/v1/lease/renew": "/internal/renew",
    "/v1/lease/acquire": "/internal/acquire",
    "/v1/transition/advance": "/internal/advance",
    "/v1/transition/checkpoint": "/internal/checkpoint",
    "/v1/handoff/ready": "/internal/handoff/ready",
    "/v1/handoff/commit": "/internal/handoff/commit",
    "/v1/control/mode": "/internal/control/mode",
    "/v1/control/pause": "/internal/control/pause",
    "/v1/control/resume": "/internal/control/resume",
    "/v1/control/emergency-freeze": "/internal/control/emergency-freeze",
  };
  internalPath = mapping[url.pathname] ?? "";
  if (!internalPath) {
    throw new HaError("NOT_FOUND", "控制面路径不存在", 404);
  }
  return forwardAndAlert(
    new Request(`https://coordinator${internalPath}`, {
      method: request.method,
      headers,
      body: request.method === "GET" ? undefined : raw,
    }),
    env,
    ctx,
  );
}

/** Worker HTTP 入口。 */
export default {
  /**
   * 处理 Worker 外部 HTTP 请求。
   * @param request 外部请求。
   * @param env Worker 运行环境。
   * @param ctx Worker 执行上下文。
   * @return Worker HTTP 响应。
   */
  async fetch(request: Request, env: Env, ctx: ExecutionContext): Promise<Response> {
    try {
      return await handleWorkerRequest(request, env, ctx);
    } catch (error) {
      return errorResponse(error);
    }
  },
} satisfies ExportedHandler<Env>;

/** 强一致 HA 协调 Durable Object。 */
export class Coordinator {
  /**
   * 创建协调对象。
   * @param state Durable Object 运行状态。
   * @param env Worker 运行环境。
   * @return 无。
   */
  public constructor(
    private readonly state: DurableObjectState,
    private readonly env: Env,
  ) {}

  /**
   * 处理内部控制请求。
   * @param request Worker 转发的内部请求。
   * @return Durable Object HTTP 响应。
   */
  public async fetch(request: Request): Promise<Response> {
    try {
      const url = new URL(request.url);
      const now = new Date();
      const current = await this.loadState(now);
      const usageResult = await this.incrementUsage(now, current);
      const effective = usageResult.state;
      const body = await readJson<Record<string, unknown>>(request);
      const node = request.headers.get("x-ha-node") as NodeId | null;
      const isAdmin = request.headers.get("x-ha-admin") === "authenticated";
      if (node) {
        await this.rememberNonce(node, request.headers.get("x-ha-nonce") ?? "", now);
      }

      switch (url.pathname) {
        case "/internal/status":
          return jsonResponse({
            ok: true,
            data: effective,
            alerts: this.alerts(usageResult.event),
          } satisfies JsonEnvelope<CoordinatorState>);
        case "/internal/bootstrap": {
          this.requireAdmin(isAdmin);
          const result = bootstrapA(
            effective,
            now,
            this.leaseTtlSeconds(),
            this.requiredString(body.value.transitionId, "transitionId"),
          );
          return this.persistResult(result, usageResult.event);
        }
        case "/internal/report": {
          const report = body.value as unknown as NodeReport;
          this.requireNode(node);
          if (report.node !== node) {
            throw new HaError("REPORT_NODE_MISMATCH", "上报节点与认证节点不一致", 403);
          }
          await this.state.storage.put(`${REPORT_PREFIX}${node}`, report);
          let reportedState = updateFailbackStability(effective, report, now);
          if (
            report.epoch === reportedState.epoch &&
            !isLeaseExpired(reportedState, now) &&
            ownerReportHealthy(reportedState, report)
          ) {
            reportedState = renewLease(
              reportedState,
              node,
              reportedState.epoch,
              now,
              this.leaseTtlSeconds(),
            ).state;
          }
          if (reportedState !== effective) {
            await this.state.storage.put(STATE_KEY, reportedState);
          }
          return jsonResponse({
            ok: true,
            data: reportedState,
            alerts: this.alerts(usageResult.event),
          } satisfies JsonEnvelope<CoordinatorState>);
        }
        case "/internal/alert": {
          this.requireNode(node);
          const eventId = this.requiredString(body.value.eventId, "eventId");
          const existing = await this.state.storage.get<StateEvent>(`event:${eventId}`);
          if (existing) {
            return jsonResponse({
              ok: true,
              data: { state: effective },
              alerts: this.alerts(usageResult.event),
            } satisfies JsonEnvelope<StateResult>);
          }
          const level = body.value.level;
          if (level !== "INFO" && level !== "WARNING" && level !== "CRITICAL") {
            throw new HaError("INVALID_ALERT_LEVEL", "节点告警级别无效", 400);
          }
          const event: StateEvent = {
            eventId,
            eventType: "node-alert",
            level,
            authorityOwner: effective.owner,
            epoch: effective.epoch,
            transitionId: effective.transitionId,
            from: effective.state,
            to: effective.state,
            node,
            reason: this.requiredString(body.value.reason, "reason"),
            result: this.requiredString(body.value.result, "result"),
            operatorAction: this.requiredString(body.value.operatorAction, "operatorAction"),
            startedAt: this.requiredString(body.value.occurredAt, "occurredAt"),
            completedAt: now.toISOString(),
            errorCode: null,
            occurredAt: this.requiredString(body.value.occurredAt, "occurredAt"),
          };
          return this.persistResult({ state: effective, event }, usageResult.event);
        }
        case "/internal/renew": {
          this.requireNode(node);
          const result = renewLease(
            effective,
            node,
            this.requiredInteger(body.value.epoch, "epoch"),
            now,
            this.leaseTtlSeconds(),
          );
          return this.persistResult(result, usageResult.event);
        }
        case "/internal/acquire": {
          this.requireNode(node);
          if (node !== "B") {
            throw new HaError("ONLY_B_CAN_ACQUIRE", "只有 B 可以执行故障接管", 403);
          }
          const result = acquireForB(
            effective,
            this.requiredInteger(body.value.expectedEpoch, "expectedEpoch"),
            now,
            this.leaseTtlSeconds(),
            this.requiredString(body.value.transitionId, "transitionId"),
            body.value.eligible === true,
          );
          return this.persistResult(result, usageResult.event);
        }
        case "/internal/advance": {
          this.requireNode(node);
          const result = advanceTransition(
            effective,
            node,
            this.requiredInteger(body.value.epoch, "epoch"),
            this.requiredState(body.value.expectedState),
            this.requiredState(body.value.nextState),
            this.requiredString(body.value.transitionStep, "transitionStep"),
            this.requiredString(body.value.reason, "reason"),
            now,
          );
          return this.persistResult(result, usageResult.event);
        }
        case "/internal/checkpoint": {
          this.requireNode(node);
          const epoch = this.requiredInteger(body.value.epoch, "epoch");
          const expectedState = this.requiredState(body.value.expectedState);
          if (effective.owner !== node || effective.epoch !== epoch || effective.state !== expectedState) {
            throw new HaError("CHECKPOINT_FORBIDDEN", "节点无权写入当前迁移 checkpoint", 403);
          }
          const next: CoordinatorState = {
            ...effective,
            transitionStep: this.requiredString(body.value.transitionStep, "transitionStep"),
            transitionStepAt: now.toISOString(),
            updatedAt: now.toISOString(),
          };
          return this.persistResult({ state: next }, usageResult.event);
        }
        case "/internal/handoff/ready": {
          this.requireNode(node);
          if (
            node !== "A" ||
            effective.owner !== "B" ||
            effective.state !== "B_FREEZING" ||
            effective.transitionStep !== "b-frozen"
          ) {
            throw new HaError("HANDOFF_READY_FORBIDDEN", "当前状态不允许 A 标记追平", 403);
          }
          const next: CoordinatorState = {
            ...effective,
            transitionStep: "a-ready",
            transitionStepAt: now.toISOString(),
            updatedAt: now.toISOString(),
          };
          return this.persistResult({ state: next }, usageResult.event);
        }
        case "/internal/handoff/commit": {
          this.requireNode(node);
          if (node !== "B") {
            throw new HaError("ONLY_B_CAN_HANDOFF", "只有 B 可以提交租约转交", 403);
          }
          if (effective.transitionStep !== "a-ready") {
            throw new HaError("A_NOT_READY", "A 尚未报告达到 B 冻结点");
          }
          const result = commitHandoffToA(
            effective,
            this.requiredInteger(body.value.epoch, "epoch"),
            now,
            this.leaseTtlSeconds(),
            this.requiredString(body.value.transitionId, "transitionId"),
            body.value.aReady === true,
          );
          return this.persistResult(result, usageResult.event);
        }
        case "/internal/control/mode": {
          this.requireAdmin(isAdmin);
          const mode = body.value.mode;
          if (mode !== "observe" && mode !== "automatic" && mode !== "paused") {
            throw new HaError("INVALID_MODE", "控制模式无效", 400);
          }
          return this.persistResult(setControlMode(effective, mode as ControlMode, now), usageResult.event);
        }
        case "/internal/control/pause": {
          this.requireAdmin(isAdmin);
          const reason = this.requiredString(body.value.reason, "reason");
          const paused = setControlMode(effective, "paused", now).state;
          return this.persistResult(
            {
              state: paused,
              event: createEvent(
                paused,
                effective.state,
                effective.state,
                "SYSTEM",
                "control-pause",
                reason,
                "WARNING",
                "确认状态后使用 resume 恢复，默认先回到 observe",
              ),
            },
            usageResult.event,
          );
        }
        case "/internal/control/resume": {
          this.requireAdmin(isAdmin);
          const owner = body.value.owner;
          const targetState = body.value.state;
          const mode = body.value.mode;
          if (owner !== "A" && owner !== "B") {
            throw new HaError("INVALID_OWNER", "恢复 owner 必须是 A 或 B", 400);
          }
          if (targetState !== "A_ACTIVE" && targetState !== "B_ACTIVE" && targetState !== "FAILBACK_WAIT") {
            throw new HaError("INVALID_RESUME_STATE", "恢复状态必须是 A_ACTIVE、B_ACTIVE 或 FAILBACK_WAIT", 400);
          }
          if (mode !== "observe" && mode !== "automatic") {
            throw new HaError("INVALID_RESUME_MODE", "恢复模式必须是 observe 或 automatic", 400);
          }
          return this.persistResult(
            resumeFromPause(
              effective,
              this.requiredInteger(body.value.expectedEpoch, "expectedEpoch"),
              owner,
              targetState,
              mode,
              this.requiredString(body.value.transitionId, "transitionId"),
              this.requiredString(body.value.reason, "reason"),
              now,
              this.leaseTtlSeconds(),
            ),
            usageResult.event,
          );
        }
        case "/internal/control/emergency-freeze": {
          this.requireAdmin(isAdmin);
          return this.persistResult(
            emergencyFreeze(effective, this.requiredString(body.value.reason, "reason"), now),
            usageResult.event,
          );
        }
        case "/internal/entry/authorize": {
          this.requireNode(node);
          const target = body.value.target;
          const epoch = this.requiredInteger(body.value.epoch, "epoch");
          this.requiredString(body.value.requestId, "requestId");
          if (target !== node || effective.owner !== node || effective.epoch !== epoch) {
            throw new HaError("ENTRY_SWITCH_FORBIDDEN", "节点没有公共入口切换资格", 403);
          }
          if (
            (node === "A" && effective.state !== "A_PROMOTING" && effective.state !== "A_ACTIVE") ||
            (node === "B" && effective.state !== "B_PROMOTING" && effective.state !== "B_ACTIVE")
          ) {
            throw new HaError("ENTRY_STATE_INVALID", `状态 ${effective.state} 不允许切换入口`);
          }
          return jsonResponse({
            ok: true,
            data: effective,
            alerts: this.alerts(usageResult.event),
          } satisfies JsonEnvelope<CoordinatorState>);
        }
        case "/internal/entry/commit": {
          this.requireNode(node);
          const target = body.value.target as EntryTunnel;
          const epoch = this.requiredInteger(body.value.epoch, "epoch");
          const requestId = this.requiredString(body.value.requestId, "requestId");
          if (target !== node || effective.owner !== node || effective.epoch !== epoch) {
            throw new HaError("ENTRY_COMMIT_FORBIDDEN", "公共入口提交资格已经变化", 403);
          }
          const next: CoordinatorState = {
            ...effective,
            entryTunnel: target,
            transitionStep: "entry-switched",
            transitionStepAt: now.toISOString(),
            updatedAt: now.toISOString(),
          };
          const result: StateResult = {
            state: next,
            event: createEvent(
              next,
              effective.state,
              effective.state,
              node,
              `entry-switch-${requestId}`,
              `公共入口已切换到 ${target}`,
              "INFO",
            ),
          };
          return this.persistResult(result, usageResult.event);
        }
        default:
          throw new HaError("NOT_FOUND", "Durable Object 路径不存在", 404);
      }
    } catch (error) {
      return errorResponse(error);
    }
  }

  /** 读取状态，不存在时创建安全初始状态。 */
  private async loadState(now: Date): Promise<CoordinatorState> {
    const stored = await this.state.storage.get<CoordinatorState>(STATE_KEY);
    if (stored) {
      if (!stored.transitionStepAt) {
        const normalized = { ...stored, transitionStepAt: stored.updatedAt };
        await this.state.storage.put(STATE_KEY, normalized);
        return normalized;
      }
      return stored;
    }
    const initial = createInitialState(now);
    await this.state.storage.put(STATE_KEY, initial);
    return initial;
  }

  /** 持久化状态和事件。 */
  private async persistResult(result: StateResult, usageEvent?: StateEvent): Promise<Response> {
    await this.state.storage.put(STATE_KEY, result.state);
    let eventToAlert = result.event;
    if (result.event) {
      const eventKey = `event:${result.event.eventId}`;
      const existing = await this.state.storage.get<StateEvent>(eventKey);
      if (existing) {
        eventToAlert = undefined;
      } else {
        await this.state.storage.put(eventKey, result.event);
      }
    }
    return jsonResponse({
      ok: true,
      data: result,
      alerts: this.alerts(eventToAlert, usageEvent),
    } satisfies JsonEnvelope<StateResult>);
  }

  /** 过滤空告警并保持事件顺序。 */
  private alerts(...events: Array<StateEvent | undefined>): StateEvent[] | undefined {
    const values = events.filter((event): event is StateEvent => event !== undefined);
    return values.length > 0 ? values : undefined;
  }

  /** 记录控制面日请求估算并在高水位暂停新编排。 */
  private async incrementUsage(now: Date, current: CoordinatorState): Promise<StateResult> {
    const date = now.toISOString().slice(0, 10);
    const limit = Number.parseInt(this.env.CONTROL_REQUEST_DAILY_LIMIT || "100000", 10);
    const previous = await this.state.storage.get<DailyUsage>(USAGE_KEY);
    const usage: DailyUsage = previous?.date === date
      ? { ...previous, count: previous.count + 1 }
      : { date, count: 1, warningLevel: 0 };
    const ratio = usage.count / limit;
    const level = ratio >= 0.95 ? 95 : ratio >= 0.85 ? 85 : ratio >= 0.7 ? 70 : 0;
    let state = current;
    let event: StateEvent | undefined;
    if (level > usage.warningLevel) {
      usage.warningLevel = level;
      if (level >= 95 && current.mode === "automatic") {
        state = { ...current, mode: "paused", updatedAt: now.toISOString() };
      }
      event = createEvent(
        state,
        current.state,
        current.state,
        "SYSTEM",
        `usage-${level}`,
        `控制面免费额度估算达到 ${level}%`,
        level >= 95 ? "CRITICAL" : "WARNING",
        level >= 95 ? "检查额度并决定是否升级或降低请求频率" : "关注控制面请求量",
      );
      await this.state.storage.put(STATE_KEY, state);
      await this.state.storage.put(`event:${event.eventId}:usage-${level}`, event);
    }
    await this.state.storage.put(USAGE_KEY, usage);
    return { state, event };
  }

  /** 保存最近 nonce，拒绝一分钟窗口内重放。 */
  private async rememberNonce(node: NodeId, nonce: string, now: Date): Promise<void> {
    if (!nonce) {
      throw new HaError("NONCE_MISSING", "内部请求缺少 nonce", 401);
    }
    const key = `${NONCE_PREFIX}${node}`;
    const cache = (await this.state.storage.get<Record<string, number>>(key)) ?? {};
    const cutoff = now.getTime() - 60_000;
    const fresh = Object.fromEntries(Object.entries(cache).filter(([, timestamp]) => timestamp >= cutoff));
    if (fresh[nonce]) {
      throw new HaError("NONCE_REPLAY", "检测到重复 nonce", 401);
    }
    fresh[nonce] = now.getTime();
    await this.state.storage.put(key, fresh);
  }

  /** 返回租约 TTL。 */
  private leaseTtlSeconds(): number {
    const value = Number.parseInt(this.env.LEASE_TTL_SECONDS || "45", 10);
    if (value !== 45) {
      throw new HaError("INVALID_TTL", "LEASE_TTL_SECONDS 必须固定为 45", 500);
    }
    return value;
  }

  /** 要求请求来自节点。 */
  private requireNode(node: NodeId | null): asserts node is NodeId {
    if (node !== "A" && node !== "B") {
      throw new HaError("NODE_UNAUTHORIZED", "内部请求缺少节点身份", 401);
    }
  }

  /** 要求请求具有管理员身份。 */
  private requireAdmin(isAdmin: boolean): void {
    if (!isAdmin) {
      throw new HaError("ADMIN_UNAUTHORIZED", "内部请求缺少管理员身份", 401);
    }
  }

  /** 读取必填字符串。 */
  private requiredString(value: unknown, field: string): string {
    if (typeof value !== "string" || value.trim().length === 0 || value.length > 256) {
      throw new HaError("INVALID_FIELD", `字段 ${field} 必须是非空字符串`, 400);
    }
    return value;
  }

  /** 读取必填整数。 */
  private requiredInteger(value: unknown, field: string): number {
    if (typeof value !== "number" || !Number.isSafeInteger(value) || value < 0) {
      throw new HaError("INVALID_FIELD", `字段 ${field} 必须是非负整数`, 400);
    }
    return value;
  }

  /** 读取有效状态机状态。 */
  private requiredState(value: unknown): HaState {
    const values: readonly HaState[] = [
      "A_ACTIVE",
      "FAILOVER_WAIT",
      "B_PROMOTING",
      "B_ACTIVE",
      "A_REBUILDING",
      "FAILBACK_WAIT",
      "B_FREEZING",
      "A_PROMOTING",
      "A_ACTIVE",
      "B_REBUILDING",
      "PAUSED_NEEDS_OPERATOR",
    ];
    if (typeof value !== "string" || !values.includes(value as HaState)) {
      throw new HaError("INVALID_STATE", "状态机状态无效", 400);
    }
    return value as HaState;
  }
}
