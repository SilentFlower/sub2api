import { afterEach, describe, expect, it, vi } from "vitest";
import { Coordinator, sendDingTalk } from "../src/index";
import { CoordinatorState, HaError, NodeReport, StateEvent, createInitialState } from "../src/model";
import {
  acquireForB,
  advanceTransition,
  bootstrapA,
  commitHandoffToA,
  emergencyFreeze,
  ownerReportHealthy,
  renewLease,
  resumeFromPause,
  setControlMode,
  updateFailbackStability,
} from "../src/state-machine";

const NOW = new Date("2026-07-11T10:00:00.000Z");

afterEach(() => {
  vi.unstubAllGlobals();
});

/** 创建已经完成 A 初始化的测试状态。 */
function activeA(mode: CoordinatorState["mode"] = "automatic"): CoordinatorState {
  const initial = createInitialState(NOW);
  const bootstrapped = bootstrapA(initial, NOW, 30, "bootstrap-1").state;
  return setControlMode(bootstrapped, mode, NOW).state;
}

/** 创建 A 已从 B 重建完成的健康报告。 */
function healthyFailbackReport(): NodeReport {
  return {
    node: "A",
    epoch: 4,
    mode: "standby-from-b",
    appHealthy: false,
    appRunning: false,
    databaseRole: "standby",
    redisRole: "replica",
    replicationHealthy: true,
    imageSyncHealthy: true,
    tunnelHealthy: true,
    restartPolicySafe: true,
    observedAt: NOW.toISOString(),
  };
}

describe("HA 状态机", () => {
  it("钉钉发送失败只做有限次数重试", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(Response.json({ errcode: 1 }, { status: 500 }))
      .mockResolvedValueOnce(Response.json({ errcode: 1 }, { status: 500 }))
      .mockResolvedValueOnce(Response.json({ errcode: 0 }));
    vi.stubGlobal("fetch", fetchMock);
    const event: StateEvent = {
      eventId: "node:A:1:test",
      eventType: "fail-closed",
      level: "CRITICAL",
      authorityOwner: "A",
      epoch: 1,
      transitionId: "transition-1",
      from: "A_ACTIVE",
      to: "A_ACTIVE",
      node: "A",
      reason: "测试告警",
      result: "已隔离",
      operatorAction: "检查节点",
      startedAt: NOW.toISOString(),
      completedAt: NOW.toISOString(),
      errorCode: null,
      occurredAt: NOW.toISOString(),
    };

    const result = await sendDingTalk(event, { DINGTALK_WEBHOOK_TOKEN: "test-token" } as never);

    expect(result).toBe("sent");
    expect(fetchMock).toHaveBeenCalledTimes(3);
    const firstRequest = fetchMock.mock.calls[0]![1] as RequestInit;
    const firstBody = JSON.parse(String(firstRequest.body));
    expect(firstBody.at.isAtAll).toBe(true);
    expect(firstBody.markdown.text).toContain("权威节点：A");
    expect(firstBody.markdown.text).toContain("2026-07-11 18:00:00 Asia/Shanghai");
  });

  it("初始化时只允许确认一次 A 主节点", () => {
    const initial = createInitialState(NOW);
    const result = bootstrapA(initial, NOW, 30, "bootstrap-1");

    expect(result.state.owner).toBe("A");
    expect(result.state.epoch).toBe(1);
    expect(result.state.mode).toBe("observe");
    expect(result.state.state).toBe("A_ACTIVE");
    expect(() => bootstrapA(result.state, NOW, 30, "bootstrap-2")).toThrowError(HaError);
  });

  it("相同状态事件重试只返回一次待发送告警", async () => {
    const values = new Map<string, unknown>();
    const state: CoordinatorState = {
      ...activeA(),
      owner: "B",
      epoch: 8,
      state: "B_PROMOTING",
      leaseUntil: new Date(NOW.getTime() + 30_000).toISOString(),
      transitionId: "failover-1",
      transitionStep: "service-ready",
      entryTunnel: "A",
    };
    values.set("coordinator-state", state);
    const durableState = {
      storage: {
        get: async (key: string) => values.get(key),
        put: async (key: string, value: unknown) => {
          values.set(key, value);
        },
      },
    };
    const coordinator = new Coordinator(
      durableState as never,
      { LEASE_TTL_SECONDS: "30", CONTROL_REQUEST_DAILY_LIMIT: "100000" } as never,
    );
    const request = (nonce: string): Request => new Request("https://coordinator/internal/entry/commit", {
      method: "POST",
      headers: {
        "content-type": "application/json",
        "x-ha-node": "B",
        "x-ha-nonce": nonce,
      },
      body: JSON.stringify({ target: "B", epoch: 8, requestId: "entry-failover-1" }),
    });

    const first = await coordinator.fetch(request("nonce-entry-commit-0001"));
    const second = await coordinator.fetch(request("nonce-entry-commit-0002"));
    const firstBody = await first.json() as { alerts?: StateEvent[] };
    const secondBody = await second.json() as { alerts?: StateEvent[] };

    expect(firstBody.alerts).toHaveLength(1);
    expect(secondBody.alerts).toBeUndefined();
  });

  it("只有当前 owner 和 epoch 可以续租", () => {
    const state = activeA();
    const renewed = renewLease(state, "A", 1, new Date(NOW.getTime() + 5_000), 30);

    expect(Date.parse(renewed.state.leaseUntil)).toBe(NOW.getTime() + 35_000);
    expect(renewed.state.transitionStepAt).toBe(state.transitionStepAt);
    expect(() => renewLease(state, "B", 1, NOW, 30)).toThrowError(/不是当前租约所有者/);
    expect(() => renewLease(state, "A", 0, NOW, 30)).toThrowError(/STALE_EPOCH|请求 epoch/);
  });

  it("节点报告只有满足门禁时才能合并续租", () => {
    const state = activeA();
    const report = healthyFailbackReport();
    const activeReport: NodeReport = {
      ...report,
      mode: "legacy-active",
      appHealthy: true,
      appRunning: true,
      databaseRole: "primary",
      redisRole: "master",
    };

    expect(ownerReportHealthy(state, activeReport)).toBe(true);
    expect(ownerReportHealthy(state, { ...activeReport, restartPolicySafe: false })).toBe(false);
    expect(ownerReportHealthy(state, { ...activeReport, imageSyncHealthy: false })).toBe(true);
    expect(ownerReportHealthy({ ...state, mode: "observe" }, { ...activeReport, restartPolicySafe: false })).toBe(false);

    const activeBState: CoordinatorState = { ...state, owner: "B", state: "B_ACTIVE" };
    const activeBReport: NodeReport = {
      ...activeReport,
      node: "B",
      mode: "active",
      imageSyncHealthy: false,
    };
    expect(ownerReportHealthy(activeBState, activeBReport)).toBe(false);
  });

  it("observe 模式只返回拟接管结果，不改变 owner", () => {
    const state = activeA("observe");
    const expired = { ...state, leaseUntil: new Date(NOW.getTime() - 1).toISOString() };
    const result = acquireForB(expired, 1, NOW, 30, "failover-1", true);

    expect(result.simulated).toBe(true);
    expect(result.state.owner).toBe("A");
    expect(result.state.epoch).toBe(1);
  });

  it("automatic 模式下 B 原子取得新 epoch", () => {
    const state = activeA();
    const expired = { ...state, leaseUntil: new Date(NOW.getTime() - 1).toISOString() };
    const result = acquireForB(expired, 1, NOW, 30, "failover-1", true);

    expect(result.state.owner).toBe("B");
    expect(result.state.epoch).toBe(2);
    expect(result.state.state).toBe("B_PROMOTING");
    expect(result.event?.level).toBe("CRITICAL");
    expect(() => acquireForB(result.state, 1, NOW, 30, "failover-2", true)).toThrowError(/epoch/);
  });

  it("复制或镜像门禁失败时 B 不能取得租约", () => {
    const state = activeA();
    const expired = { ...state, leaseUntil: new Date(NOW.getTime() - 1).toISOString() };

    expect(() => acquireForB(expired, 1, NOW, 30, "failover-1", false)).toThrowError(/门禁未通过/);
  });

  it("B 只能按白名单推进到 active", () => {
    const state = activeA();
    const expired = { ...state, leaseUntil: new Date(NOW.getTime() - 1).toISOString() };
    const promoting = acquireForB(expired, 1, NOW, 30, "failover-1", true).state;
    const active = advanceTransition(
      promoting,
      "B",
      2,
      "B_PROMOTING",
      "B_ACTIVE",
      "entry-verified",
      "B 应用和公共入口健康",
      NOW,
    );

    expect(active.state.state).toBe("B_ACTIVE");
    expect(() =>
      advanceTransition(
        promoting,
        "B",
        2,
        "B_PROMOTING",
        "A_ACTIVE",
        "invalid",
        "越级迁移",
        NOW,
      ),
    ).toThrowError(/不允许/);
  });

  it("只有 B_FREEZING 且 A 已追平时才能 handoff", () => {
    const base = activeA();
    const bFreezing: CoordinatorState = {
      ...base,
      owner: "B",
      epoch: 8,
      state: "B_FREEZING",
      leaseUntil: new Date(NOW.getTime() + 30_000).toISOString(),
      transitionId: "failback-1",
    };
    expect(() => commitHandoffToA(bFreezing, 8, NOW, 30, "handoff-1", false)).toThrowError(/尚未达到/);

    const result = commitHandoffToA(bFreezing, 8, NOW, 30, "handoff-1", true);
    expect(result.state.owner).toBe("A");
    expect(result.state.epoch).toBe(9);
    expect(result.state.state).toBe("A_PROMOTING");
  });

  it("B 持有租约时允许 A 只推进自身重建阶段", () => {
    const state: CoordinatorState = {
      ...activeA(),
      owner: "B",
      epoch: 4,
      state: "B_ACTIVE",
      transitionId: "recovery-1",
      leaseUntil: new Date(NOW.getTime() + 30_000).toISOString(),
    };
    const rebuilding = advanceTransition(
      state,
      "A",
      4,
      "B_ACTIVE",
      "A_REBUILDING",
      "a-rebuild-started",
      "A 已恢复",
      NOW,
    );
    expect(rebuilding.state.owner).toBe("B");
    expect(rebuilding.state.state).toBe("A_REBUILDING");

    expect(() =>
      advanceTransition(
        state,
        "A",
        4,
        "B_ACTIVE",
        "PAUSED_NEEDS_OPERATOR",
        "invalid",
        "A 越权暂停",
        NOW,
      ),
    ).toThrowError(/不能推进/);
  });

  it("A 受托动作失败时可以把编排安全暂停", () => {
    const state: CoordinatorState = {
      ...activeA(),
      owner: "B",
      epoch: 4,
      state: "A_REBUILDING",
      transitionId: "recovery-1",
      leaseUntil: new Date(NOW.getTime() + 30_000).toISOString(),
    };

    const paused = advanceTransition(
      state,
      "A",
      4,
      "A_REBUILDING",
      "PAUSED_NEEDS_OPERATOR",
      "a-rebuilding-failed",
      "A 重建失败",
      NOW,
    );

    expect(paused.state.state).toBe("PAUSED_NEEDS_OPERATOR");
    expect(paused.event?.level).toBe("CRITICAL");
  });

  it("回切稳定窗口只累计 A 的连续健康时间", () => {
    const state: CoordinatorState = {
      ...activeA(),
      owner: "B",
      epoch: 4,
      state: "A_REBUILDING",
      transitionId: "recovery-1",
      leaseUntil: new Date(NOW.getTime() + 30_000).toISOString(),
    };
    const waiting = advanceTransition(
      state,
      "A",
      4,
      "A_REBUILDING",
      "FAILBACK_WAIT",
      "a-rebuild-complete",
      "A 重建完成",
      NOW,
    ).state;
    expect(waiting.stableSince).toBeNull();

    const healthy = updateFailbackStability(waiting, healthyFailbackReport(), NOW);
    expect(healthy.stableSince).toBe(NOW.toISOString());

    const unhealthyReport = { ...healthyFailbackReport(), replicationHealthy: false };
    const reset = updateFailbackStability(
      healthy,
      unhealthyReport,
      new Date(NOW.getTime() + 10 * 60_000),
    );
    expect(reset.stableSince).toBeNull();

    const restarted = updateFailbackStability(
      reset,
      healthyFailbackReport(),
      new Date(NOW.getTime() + 11 * 60_000),
    );
    expect(restarted.stableSince).toBe(new Date(NOW.getTime() + 11 * 60_000).toISOString());
  });

  it("紧急冻结清除 owner 并进入 paused", () => {
    const result = emergencyFreeze(activeA(), "人工紧急冻结", NOW);

    expect(result.state.owner).toBe("NONE");
    expect(result.state.mode).toBe("paused");
    expect(result.state.state).toBe("PAUSED_NEEDS_OPERATOR");
    expect(result.event?.level).toBe("CRITICAL");
  });

  it("管理员恢复暂停状态时必须声明 owner 并取得新 epoch", () => {
    const frozen = emergencyFreeze(activeA(), "人工紧急冻结", NOW).state;
    const resumed = resumeFromPause(
      frozen,
      frozen.epoch,
      "A",
      "A_ACTIVE",
      "observe",
      "resume-1",
      "已确认 A 是唯一主节点",
      new Date(NOW.getTime() + 5_000),
      30,
    );

    expect(resumed.state.owner).toBe("A");
    expect(resumed.state.epoch).toBe(frozen.epoch + 1);
    expect(resumed.state.mode).toBe("observe");
    expect(resumed.state.transitionStep).toBe("operator-resumed");
    expect(() =>
      resumeFromPause(
        frozen,
        frozen.epoch,
        "A",
        "B_ACTIVE",
        "observe",
        "resume-invalid",
        "错误 owner",
        NOW,
        30,
      ),
    ).toThrowError(/必须由节点 B/);
  });
});
