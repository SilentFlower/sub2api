import {
  CoordinatorState,
  ControlMode,
  HaError,
  HaState,
  NodeId,
  NodeReport,
  StateEvent,
  StateResult,
  isLeaseExpired,
} from "./model";

const ACTIVE_STATES = new Set<HaState>(["A_ACTIVE", "B_ACTIVE"]);

const ALLOWED_TRANSITIONS: Readonly<Record<HaState, readonly HaState[]>> = {
  A_ACTIVE: ["FAILOVER_WAIT", "B_REBUILDING", "PAUSED_NEEDS_OPERATOR"],
  FAILOVER_WAIT: ["B_PROMOTING", "A_ACTIVE", "PAUSED_NEEDS_OPERATOR"],
  B_PROMOTING: ["B_ACTIVE", "PAUSED_NEEDS_OPERATOR"],
  B_ACTIVE: ["A_REBUILDING", "PAUSED_NEEDS_OPERATOR"],
  A_REBUILDING: ["FAILBACK_WAIT", "PAUSED_NEEDS_OPERATOR"],
  FAILBACK_WAIT: ["B_FREEZING", "PAUSED_NEEDS_OPERATOR"],
  B_FREEZING: ["FAILBACK_WAIT", "A_PROMOTING", "PAUSED_NEEDS_OPERATOR"],
  A_PROMOTING: ["A_ACTIVE", "PAUSED_NEEDS_OPERATOR"],
  B_REBUILDING: ["A_ACTIVE", "PAUSED_NEEDS_OPERATOR"],
  PAUSED_NEEDS_OPERATOR: ["A_ACTIVE", "B_ACTIVE", "FAILBACK_WAIT"],
};

/**
 * 返回状态对应的预期租约所有者。
 * @param state 状态机状态。
 * @return 固定节点或允许受托节点推进的 ANY。
 */
export function expectedOwner(state: HaState): NodeId | "ANY" {
  switch (state) {
    case "A_ACTIVE":
    case "A_PROMOTING":
    case "B_REBUILDING":
      return "A";
    case "B_PROMOTING":
    case "B_ACTIVE":
    case "A_REBUILDING":
    case "FAILBACK_WAIT":
    case "B_FREEZING":
      return "B";
    case "FAILOVER_WAIT":
    case "PAUSED_NEEDS_OPERATOR":
      return "ANY";
  }
}

/**
 * 生成稳定状态事件。
 * @param state 事件完成后的权威状态。
 * @param from 原状态。
 * @param to 目标状态。
 * @param node 执行动作的节点。
 * @param eventType 稳定事件类型。
 * @param reason 触发原因。
 * @param level 告警级别。
 * @param operatorAction 建议人工动作。
 * @return 可持久化和去重的状态事件。
 */
export function createEvent(
  state: CoordinatorState,
  from: HaState,
  to: HaState,
  node: NodeId | "SYSTEM",
  eventType: string,
  reason: string,
  level: StateEvent["level"],
  operatorAction = "无需人工操作",
): StateEvent {
  return {
    eventId: `${state.epoch}:${state.transitionId}:${eventType}`,
    eventType,
    level,
    authorityOwner: state.owner,
    epoch: state.epoch,
    transitionId: state.transitionId,
    from,
    to,
    node,
    reason,
    result: `状态已更新为 ${to}`,
    operatorAction,
    startedAt: state.transitionStepAt,
    completedAt: state.updatedAt,
    errorCode: state.lastErrorCode,
    occurredAt: state.updatedAt,
  };
}

/**
 * 初始化 A 为唯一主节点，默认只进入观察模式。
 * @param current 当前权威状态。
 * @param now 当前时间。
 * @param ttlSeconds 租约 TTL 秒数。
 * @param transitionId 初始化迁移 ID。
 * @return 初始化结果。
 */
export function bootstrapA(
  current: CoordinatorState,
  now: Date,
  ttlSeconds: number,
  transitionId: string,
): StateResult {
  if (current.epoch !== 0 || current.owner !== "NONE") {
    throw new HaError("ALREADY_BOOTSTRAPPED", "控制面已经完成初始化");
  }
  const timestamp = now.toISOString();
  const next: CoordinatorState = {
    ...current,
    owner: "A",
    epoch: 1,
    leaseUntil: new Date(now.getTime() + ttlSeconds * 1000).toISOString(),
    state: "A_ACTIVE",
    mode: "observe",
    transitionId,
    transitionStep: "bootstrap-complete",
    transitionStepAt: timestamp,
    entryTunnel: "A",
    lastErrorCode: null,
    lastErrorMessage: null,
    updatedAt: timestamp,
  };
  return {
    state: next,
    event: createEvent(next, current.state, next.state, "SYSTEM", "bootstrap-a", "确认 A 当前为唯一主节点", "INFO"),
  };
}

/**
 * 为当前所有者续租。
 * @param current 当前权威状态。
 * @param node 请求节点。
 * @param epoch 请求 epoch。
 * @param now 当前时间。
 * @param ttlSeconds 租约 TTL 秒数。
 * @return 续租结果。
 */
export function renewLease(
  current: CoordinatorState,
  node: NodeId,
  epoch: number,
  now: Date,
  ttlSeconds: number,
): StateResult {
  if (current.owner !== node) {
    throw new HaError("NOT_LEASE_OWNER", `节点 ${node} 不是当前租约所有者`);
  }
  if (current.epoch !== epoch) {
    throw new HaError("STALE_EPOCH", `请求 epoch=${epoch}，当前 epoch=${current.epoch}`);
  }
  if (isLeaseExpired(current, now)) {
    throw new HaError("LEASE_EXPIRED", "租约已经过期，不能通过普通续租恢复");
  }
  const expected = expectedOwner(current.state);
  if (expected !== "ANY" && expected !== node) {
    throw new HaError("OWNER_STATE_MISMATCH", "租约所有者与状态机不一致");
  }
  return {
    state: {
      ...current,
      leaseUntil: new Date(now.getTime() + ttlSeconds * 1000).toISOString(),
      updatedAt: now.toISOString(),
    },
  };
}

/**
 * B 在 A 租约到期后申请接管。
 * @param current 当前权威状态。
 * @param expectedEpoch B 已观察到的 epoch。
 * @param now 当前时间。
 * @param ttlSeconds 租约 TTL 秒数。
 * @param transitionId 故障接管迁移 ID。
 * @param eligible B 本地门禁是否通过。
 * @return 接管或观察模式模拟结果。
 */
export function acquireForB(
  current: CoordinatorState,
  expectedEpoch: number,
  now: Date,
  ttlSeconds: number,
  transitionId: string,
  eligible: boolean,
): StateResult {
  if (current.epoch !== expectedEpoch) {
    throw new HaError("STALE_EPOCH", `请求 epoch=${expectedEpoch}，当前 epoch=${current.epoch}`);
  }
  if (!isLeaseExpired(current, now)) {
    throw new HaError("LEASE_STILL_VALID", "A 租约仍有效，B 不能接管");
  }
  if (!eligible) {
    throw new HaError("B_NOT_ELIGIBLE", "B 本地复制、镜像或角色门禁未通过");
  }
  if (current.mode === "paused") {
    throw new HaError("CONTROL_PAUSED", "控制面已暂停，不能申请接管");
  }
  if (current.mode === "observe") {
    return { state: current, simulated: true };
  }
  if (!ACTIVE_STATES.has(current.state) && current.state !== "FAILOVER_WAIT") {
    throw new HaError("INVALID_FAILOVER_STATE", `状态 ${current.state} 不允许 B 接管`);
  }
  const from = current.state;
  const next: CoordinatorState = {
    ...current,
    owner: "B",
    epoch: current.epoch + 1,
    leaseUntil: new Date(now.getTime() + ttlSeconds * 1000).toISOString(),
    state: "B_PROMOTING",
    transitionId,
    transitionStep: "lease-acquired",
    transitionStepAt: now.toISOString(),
    stableSince: null,
    lastErrorCode: null,
    lastErrorMessage: null,
    updatedAt: now.toISOString(),
  };
  return {
    state: next,
    event: createEvent(next, from, next.state, "B", "b-acquire", "A 租约已过期且 B 门禁通过", "CRITICAL"),
  };
}

/**
 * 按白名单推进普通状态迁移。
 * @param current 当前权威状态。
 * @param node 请求节点。
 * @param epoch 当前 epoch。
 * @param expectedState 预期当前状态。
 * @param nextState 目标状态。
 * @param transitionStep 新阶段标记。
 * @param reason 迁移原因。
 * @param now 当前时间。
 * @return 状态迁移结果。
 */
export function advanceTransition(
  current: CoordinatorState,
  node: NodeId,
  epoch: number,
  expectedState: HaState,
  nextState: HaState,
  transitionStep: string,
  reason: string,
  now: Date,
): StateResult {
  const delegatedActor = node === "A" && (
    (current.state === "B_ACTIVE" && nextState === "A_REBUILDING") ||
    (current.state === "A_REBUILDING" && nextState === "FAILBACK_WAIT") ||
    (current.state === "FAILBACK_WAIT" && nextState === "B_FREEZING") ||
    (nextState === "PAUSED_NEEDS_OPERATOR" &&
      (current.state === "A_REBUILDING" || current.state === "B_FREEZING"))
  );
  if (current.owner !== node && !delegatedActor) {
    throw new HaError("ACTOR_NOT_ALLOWED", `节点 ${node} 不能推进当前状态`);
  }
  if (current.epoch !== epoch) {
    throw new HaError("STALE_EPOCH", `请求 epoch=${epoch}，当前 epoch=${current.epoch}`);
  }
  if (current.state !== expectedState) {
    throw new HaError("STATE_CHANGED", `请求状态 ${expectedState}，当前状态 ${current.state}`);
  }
  if (!ALLOWED_TRANSITIONS[current.state].includes(nextState)) {
    throw new HaError("INVALID_TRANSITION", `不允许从 ${current.state} 迁移到 ${nextState}`);
  }
  const owner = expectedOwner(nextState);
  if (owner !== "ANY" && owner !== current.owner) {
    throw new HaError("NEXT_OWNER_MISMATCH", `状态 ${nextState} 与当前租约所有者不一致`);
  }
  const next: CoordinatorState = {
    ...current,
    state: nextState,
    transitionStep,
    transitionStepAt: now.toISOString(),
    stableSince: nextState === "FAILBACK_WAIT" ? null : current.stableSince,
    lastErrorCode: nextState === "PAUSED_NEEDS_OPERATOR" ? "TRANSITION_PAUSED" : null,
    lastErrorMessage: nextState === "PAUSED_NEEDS_OPERATOR" ? reason : null,
    updatedAt: now.toISOString(),
  };
  const level = nextState === "PAUSED_NEEDS_OPERATOR" ? "CRITICAL" : "INFO";
  return {
    state: next,
    event: createEvent(
      next,
      current.state,
      nextState,
      node,
      `transition-${transitionStep}`,
      reason,
      level,
      nextState === "PAUSED_NEEDS_OPERATOR" ? "检查状态后执行 resume" : undefined,
    ),
  };
}

/**
 * 根据 A 的连续健康报告维护自动回切稳定窗口。
 * @param current 当前权威状态。
 * @param report 节点健康报告。
 * @param now 当前时间。
 * @return 更新稳定时间后的权威状态。
 */
export function updateFailbackStability(
  current: CoordinatorState,
  report: NodeReport,
  now: Date,
): CoordinatorState {
  if (current.owner !== "B" || current.state !== "FAILBACK_WAIT" || report.node !== "A") {
    return current;
  }
  const healthy = report.mode === "standby-from-b" &&
    !report.appRunning &&
    report.databaseRole === "standby" &&
    report.redisRole === "replica" &&
    report.replicationHealthy &&
    report.imageSyncHealthy &&
    report.tunnelHealthy &&
    report.restartPolicySafe;
  if (healthy && current.stableSince === null) {
    return { ...current, stableSince: now.toISOString(), updatedAt: now.toISOString() };
  }
  if (!healthy && current.stableSince !== null) {
    return { ...current, stableSince: null, updatedAt: now.toISOString() };
  }
  return current;
}

/**
 * 判断节点报告是否满足 owner 续租门禁。
 * @param current 当前权威状态。
 * @param report 节点健康报告。
 * @return 全部写入门禁一致时返回 true。
 */
export function ownerReportHealthy(current: CoordinatorState, report: NodeReport): boolean {
  if (current.owner !== report.node || !report.tunnelHealthy || report.mode === "inconsistent") {
    return false;
  }
  if (current.state === "A_ACTIVE" || current.state === "B_ACTIVE") {
    const expectedMode = report.node === "A" ? ["legacy-active", "active-recovered"] : ["active"];
    // A 的运行镜像是发布权威；镜像漂移只关闭 B 接管资格，不撤销健康 A 的租约。
    const imageReady = report.node === "A" || report.imageSyncHealthy;
    return expectedMode.includes(report.mode) &&
      report.appRunning &&
      report.appHealthy &&
      report.databaseRole === "primary" &&
      report.redisRole === "master" &&
      imageReady &&
      report.restartPolicySafe;
  }
  return true;
}

/**
 * 原子提交 B 到 A 的租约转交。
 * @param current 当前权威状态。
 * @param epoch B 当前 epoch。
 * @param now 当前时间。
 * @param ttlSeconds 新租约 TTL 秒数。
 * @param transitionId handoff 迁移 ID。
 * @param aReady A 是否已经达到冻结点。
 * @return A 取得新 epoch 后的状态。
 */
export function commitHandoffToA(
  current: CoordinatorState,
  epoch: number,
  now: Date,
  ttlSeconds: number,
  transitionId: string,
  aReady: boolean,
): StateResult {
  if (current.owner !== "B" || current.epoch !== epoch) {
    throw new HaError("HANDOFF_OWNER_MISMATCH", "只有当前 B 租约持有者可以提交回切");
  }
  if (current.state !== "B_FREEZING") {
    throw new HaError("INVALID_HANDOFF_STATE", `状态 ${current.state} 不允许提交回切`);
  }
  if (!aReady) {
    throw new HaError("A_NOT_READY", "A 尚未达到 B 的冻结点");
  }
  if (isLeaseExpired(current, now)) {
    throw new HaError("LEASE_EXPIRED", "B 租约已过期，不能提交 handoff");
  }
  const next: CoordinatorState = {
    ...current,
    owner: "A",
    epoch: current.epoch + 1,
    leaseUntil: new Date(now.getTime() + ttlSeconds * 1000).toISOString(),
    state: "A_PROMOTING",
    transitionId,
    transitionStep: "lease-transferred",
    transitionStepAt: now.toISOString(),
    stableSince: null,
    lastErrorCode: null,
    lastErrorMessage: null,
    updatedAt: now.toISOString(),
  };
  return {
    state: next,
    event: createEvent(next, current.state, next.state, "A", "handoff-commit", "A 已追平 B 冻结点", "CRITICAL"),
  };
}

/**
 * 更新控制模式，不改变当前租约所有者。
 * @param current 当前权威状态。
 * @param mode 目标控制模式。
 * @param now 当前时间。
 * @return 模式更新结果。
 */
export function setControlMode(
  current: CoordinatorState,
  mode: ControlMode,
  now: Date,
): StateResult {
  return {
    state: {
      ...current,
      mode,
      updatedAt: now.toISOString(),
    },
  };
}

/**
 * 由管理员在确认唯一权威节点后恢复暂停状态。
 * @param current 当前权威状态。
 * @param expectedEpoch 管理员已确认的当前 epoch。
 * @param owner 已确认的唯一 owner。
 * @param state 与真实拓扑一致的恢复状态。
 * @param mode 恢复后的控制模式。
 * @param transitionId 人工恢复迁移 ID。
 * @param reason 人工确认依据。
 * @param now 当前时间。
 * @param ttlSeconds 新租约 TTL 秒数。
 * @return 恢复后的权威状态。
 */
export function resumeFromPause(
  current: CoordinatorState,
  expectedEpoch: number,
  owner: NodeId,
  state: "A_ACTIVE" | "B_ACTIVE" | "FAILBACK_WAIT",
  mode: "observe" | "automatic",
  transitionId: string,
  reason: string,
  now: Date,
  ttlSeconds: number,
): StateResult {
  if (current.state !== "PAUSED_NEEDS_OPERATOR" && current.mode !== "paused") {
    throw new HaError("CONTROL_NOT_PAUSED", "控制面当前不处于暂停状态");
  }
  if (current.epoch !== expectedEpoch) {
    throw new HaError("STALE_EPOCH", `请求 epoch=${expectedEpoch}，当前 epoch=${current.epoch}`);
  }
  const requiredOwner = expectedOwner(state);
  if (requiredOwner !== owner) {
    throw new HaError("RESUME_OWNER_MISMATCH", `状态 ${state} 必须由节点 ${requiredOwner} 持有`);
  }
  const next: CoordinatorState = {
    ...current,
    owner,
    epoch: current.epoch + 1,
    leaseUntil: new Date(now.getTime() + ttlSeconds * 1000).toISOString(),
    state,
    mode,
    transitionId,
    transitionStep: "operator-resumed",
    transitionStepAt: now.toISOString(),
    stableSince: state === "FAILBACK_WAIT" ? null : current.stableSince,
    lastErrorCode: null,
    lastErrorMessage: null,
    updatedAt: now.toISOString(),
  };
  return {
    state: next,
    event: createEvent(
      next,
      current.state,
      next.state,
      "SYSTEM",
      "operator-resume",
      reason,
      "CRITICAL",
      mode === "observe" ? "核对节点状态并完成观察后再启用 automatic" : "持续观察恢复后的租约和拓扑",
    ),
  };
}

/**
 * 紧急冻结全局写入资格。
 * @param current 当前权威状态。
 * @param reason 紧急冻结原因。
 * @param now 当前时间。
 * @return 清除 owner 后的安全暂停状态。
 */
export function emergencyFreeze(
  current: CoordinatorState,
  reason: string,
  now: Date,
): StateResult {
  const next: CoordinatorState = {
    ...current,
    owner: "NONE",
    leaseUntil: now.toISOString(),
    state: "PAUSED_NEEDS_OPERATOR",
    mode: "paused",
    transitionStep: "emergency-frozen",
    transitionStepAt: now.toISOString(),
    lastErrorCode: "EMERGENCY_FREEZE",
    lastErrorMessage: reason,
    updatedAt: now.toISOString(),
  };
  return {
    state: next,
    event: createEvent(
      next,
      current.state,
      next.state,
      "SYSTEM",
      "emergency-freeze",
      reason,
      "CRITICAL",
      "人工确认唯一权威节点",
    ),
  };
}
