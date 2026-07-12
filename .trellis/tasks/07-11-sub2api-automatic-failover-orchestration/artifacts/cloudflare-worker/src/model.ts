/** HA 节点标识。 */
export type NodeId = "A" | "B";

/** 当前租约所有者。 */
export type Owner = NodeId | "NONE";

/** HA 控制模式。 */
export type ControlMode = "observe" | "automatic" | "paused";

/** 持久化状态机状态。 */
export type HaState =
  | "A_ACTIVE"
  | "FAILOVER_WAIT"
  | "B_PROMOTING"
  | "B_ACTIVE"
  | "A_REBUILDING"
  | "FAILBACK_WAIT"
  | "B_FREEZING"
  | "A_PROMOTING"
  | "A_ACTIVE"
  | "B_REBUILDING"
  | "PAUSED_NEEDS_OPERATOR";

/** 当前公共入口所指向的 HA Tunnel。 */
export type EntryTunnel = NodeId | "NONE";

/** Durable Object 持久化的权威状态。 */
export interface CoordinatorState {
  owner: Owner;
  epoch: number;
  leaseUntil: string;
  state: HaState;
  mode: ControlMode;
  transitionId: string;
  transitionStep: string;
  transitionStepAt: string;
  stableSince: string | null;
  entryTunnel: EntryTunnel;
  lastErrorCode: string | null;
  lastErrorMessage: string | null;
  updatedAt: string;
}

/** 节点上报的本地状态摘要。 */
export interface NodeReport {
  node: NodeId;
  epoch: number;
  mode: string;
  appHealthy: boolean;
  appRunning: boolean;
  databaseRole: "primary" | "standby" | "unknown";
  redisRole: "master" | "replica" | "unknown";
  replicationHealthy: boolean;
  imageSyncHealthy: boolean;
  tunnelHealthy: boolean;
  restartPolicySafe: boolean;
  observedAt: string;
}

/** 可投递到钉钉的状态事件。 */
export interface StateEvent {
  eventId: string;
  eventType: string;
  level: "INFO" | "WARNING" | "CRITICAL";
  authorityOwner: Owner;
  epoch: number;
  transitionId: string;
  from: HaState;
  to: HaState;
  node: NodeId | "SYSTEM";
  reason: string;
  result: string;
  operatorAction: string;
  startedAt: string;
  completedAt: string;
  errorCode: string | null;
  occurredAt: string;
}

/** 状态机操作结果。 */
export interface StateResult {
  state: CoordinatorState;
  event?: StateEvent;
  simulated?: boolean;
}

/** HA 业务错误，包含稳定机器错误码。 */
export class HaError extends Error {
  /**
   * 创建 HA 业务错误。
   * @param code 稳定机器错误码。
   * @param message 中文错误说明。
   * @param status HTTP 状态码。
   * @return 无。
   */
  public constructor(
    public readonly code: string,
    message: string,
    public readonly status = 409,
  ) {
    super(message);
    this.name = "HaError";
  }
}

/**
 * 返回指定时间是否已经达到租约截止时间。
 * @param state 当前权威状态。
 * @param now 当前时间。
 * @return 租约已经到期时返回 true。
 */
export function isLeaseExpired(state: CoordinatorState, now: Date): boolean {
  return Date.parse(state.leaseUntil) <= now.getTime();
}

/**
 * 创建初始的安全暂停状态。
 * @param now 当前时间。
 * @return 尚未初始化 owner 的安全状态。
 */
export function createInitialState(now: Date): CoordinatorState {
  const timestamp = now.toISOString();
  return {
    owner: "NONE",
    epoch: 0,
    leaseUntil: timestamp,
    state: "PAUSED_NEEDS_OPERATOR",
    mode: "observe",
    transitionId: "bootstrap-required",
    transitionStep: "uninitialized",
    transitionStepAt: timestamp,
    stableSince: null,
    entryTunnel: "NONE",
    lastErrorCode: "BOOTSTRAP_REQUIRED",
    lastErrorMessage: "需要先确认 A 当前为唯一主节点",
    updatedAt: timestamp,
  };
}
