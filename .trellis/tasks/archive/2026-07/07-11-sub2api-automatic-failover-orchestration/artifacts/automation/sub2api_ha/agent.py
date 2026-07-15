"""HA agent 主循环。"""

from __future__ import annotations

import logging
import time
import uuid
from datetime import datetime, timezone
from typing import Protocol
from zoneinfo import ZoneInfo

from .alerts import AlertQueue, PendingAlert, create_pending_alert
from .checkpoint import read_checkpoint, write_checkpoint
from .client import ControlPlaneClient, ControlPlaneError
from .config import AgentConfig
from .executor import Executor
from .model import Checkpoint, ControlState, LocalState
from .probe import Probe


class Client(Protocol):
    """agent 使用的控制面接口。"""

    def status(self) -> ControlState:
        """读取权威状态。

        @return: Worker 当前权威状态。
        """
        ...

    def report(self, payload: dict[str, object]) -> ControlState:
        """上报节点状态。

        @param payload: 节点本地状态负载。
        @return: 合并心跳后的权威状态。
        """
        ...

    def alert(self, payload: dict[str, object]) -> ControlState:
        """发送节点关键告警。

        @param payload: 关键告警负载。
        @return: Worker 当前权威状态。
        """
        ...

    def renew(self, epoch: int) -> ControlState:
        """续租当前节点。

        @param epoch: 当前已知 epoch。
        @return: 续租后的权威状态。
        """
        ...

    def acquire_for_b(
        self, expected_epoch: int, eligible: bool, transition_id: str
    ) -> tuple[ControlState, bool]:
        """请求 B 故障接管。

        @param expected_epoch: B 已观察到的 epoch。
        @param eligible: B 本地接管门禁是否通过。
        @param transition_id: 本次迁移 ID。
        @return: 权威状态与是否为观察模式模拟结果。
        """
        ...

    def advance(
        self,
        epoch: int,
        expected_state: str,
        next_state: str,
        transition_step: str,
        reason: str,
    ) -> ControlState:
        """推进状态机。

        @param epoch: 当前 epoch。
        @param expected_state: 预期当前状态。
        @param next_state: 目标状态。
        @param transition_step: 新阶段标记。
        @param reason: 迁移原因。
        @return: 更新后的权威状态。
        """
        ...

    def checkpoint(
        self, epoch: int, expected_state: str, transition_step: str
    ) -> ControlState:
        """写入迁移 checkpoint。

        @param epoch: 当前 epoch。
        @param expected_state: 预期当前状态。
        @param transition_step: 新阶段标记。
        @return: 更新后的权威状态。
        """
        ...

    def handoff_ready(self, epoch: int) -> ControlState:
        """标记 A 已达到冻结点。

        @param epoch: 当前 epoch。
        @return: 更新后的权威状态。
        """
        ...

    def commit_handoff(self, epoch: int, transition_id: str) -> ControlState:
        """提交 B 到 A 的租约转交。

        @param epoch: B 当前持有的 epoch。
        @param transition_id: 新 handoff 迁移 ID。
        @return: A 取得新 epoch 后的权威状态。
        """
        ...

    def switch_entry(self, target: str, epoch: int, request_id: str) -> ControlState:
        """切换公共 HA Tunnel 入口。

        @param target: 目标节点。
        @param epoch: 当前 epoch。
        @param request_id: 幂等请求 ID。
        @return: 入口提交后的权威状态。
        """
        ...


class HaAgent:
    """协调本地门禁和外部租约。"""

    def __init__(
        self,
        config: AgentConfig,
        client: Client,
        probe: Probe,
        executor: Executor,
        logger: logging.Logger | None = None,
    ) -> None:
        """创建 HA agent。

        @param config: 节点配置。
        @param client: 控制面客户端。
        @param probe: 本地状态探测器。
        @param executor: 可变更动作执行器。
        @param logger: 可选日志器。
        @return: 无。
        """
        self._config = config
        self._client = client
        self._probe = probe
        self._executor = executor
        self._logger = logger or logging.getLogger("sub2api-ha-agent")
        self._last_control: ControlState | None = None
        self._checkpoint_invalid = False
        try:
            self._checkpoint = read_checkpoint(config.checkpoint_file)
        except RuntimeError:
            self._logger.exception(
                "本地 checkpoint 无法读取，将在控制面不可达时 fail-closed"
            )
            self._checkpoint = None
            self._checkpoint_invalid = True
        self._alerts = AlertQueue(config.pending_alert_file)

    @classmethod
    def create_control_client(cls, config: AgentConfig) -> ControlPlaneClient:
        """创建使用节点 HMAC 身份的 Worker 客户端。

        @param config: 节点配置。
        @return: Worker 控制面客户端。
        """
        return ControlPlaneClient(
            config.control_url,
            config.node,
            config.read_secret(),
            config.request_timeout_seconds,
        )

    @classmethod
    def create_system(
        cls, config: AgentConfig, probe: Probe, executor: Executor
    ) -> "HaAgent":
        """创建使用真实 Worker 客户端的 agent。

        @param config: 节点配置。
        @param probe: 本地状态探测器。
        @param executor: 可变更动作执行器。
        @return: HA agent。
        """
        return cls(config, cls.create_control_client(config), probe, executor)

    def run_forever(self) -> None:
        """持续运行 HA 循环。

        @return: 无；进程被停止前持续运行。
        """
        while True:
            started = time.monotonic()
            try:
                self.run_once()
            except Exception:
                self._logger.exception("HA 循环发生未处理错误")
                self._fence_if_cached_lease_expired(datetime.now(timezone.utc))
            elapsed = time.monotonic() - started
            time.sleep(max(0.1, self._config.interval_seconds - elapsed))

    def run_once(self, now: datetime | None = None) -> ControlState | None:
        """执行一次状态采集和决策。

        @param now: 测试可覆盖的当前时间。
        @return: 本轮权威状态，控制面不可达时返回 None。
        """
        lease_local = self._probe.collect_lease_state()
        try:
            control = self._last_control or self._client.status()
            control = self._client.report(lease_local.report_payload(control.epoch))
            self._last_control = control
            self._flush_pending_alerts()
        except ControlPlaneError as exc:
            current_time = now or datetime.now(timezone.utc)
            self._logger.warning("控制面不可达：%s", exc)
            self._fence_if_cached_lease_expired(
                current_time, lease_local, control_unreachable=True
            )
            return None

        local = lease_local
        if self._requires_detailed_state(control):
            try:
                local = self._probe.collect()
            except Exception as exc:
                current_time = now or datetime.now(timezone.utc)
                self._logger.warning(
                    "详细状态探测失败；本轮租约心跳已完成，跳过状态编排：%s",
                    exc,
                )
                self._reconcile(
                    lease_local,
                    control,
                    current_time,
                    allow_orchestration=False,
                )
                return self._persist_checkpoint(control, current_time)

        current_time = now or datetime.now(timezone.utc)
        self._reconcile(local, control, current_time)
        effective = self._last_control or control
        return self._persist_checkpoint(effective, current_time)

    def _persist_checkpoint(
        self, control: ControlState, current_time: datetime
    ) -> ControlState:
        """持久化最近一次成功取得的权威状态。

        @param control: 本轮最终权威状态。
        @param current_time: checkpoint 记录时间。
        @return: 原样返回权威状态。
        """
        checkpoint = Checkpoint.from_control(control, current_time)
        write_checkpoint(self._config.checkpoint_file, checkpoint)
        self._checkpoint = checkpoint
        self._checkpoint_invalid = False
        return control

    def _requires_detailed_state(self, control: ControlState) -> bool:
        """判断本轮是否需要包含跨节点信息的详细状态。

        @param control: 合并心跳后的权威状态。
        @return: 需要执行迁移或非稳态判断时返回 True。
        """
        if self._config.lease_state_command == self._config.state_command:
            return False
        return not (
            self._config.node == "A"
            and control.owner == "A"
            and control.state == "A_ACTIVE"
        )

    def _reconcile(
        self,
        local: LocalState,
        control: ControlState,
        now: datetime,
        allow_orchestration: bool = True,
    ) -> None:
        if control.owner == self._config.node:
            if control.expired(now):
                self._logger.critical("权威租约已经到期，拒绝继续 owner 编排")
                self._fence_if_cached_lease_expired(now, local)
                return
            if not local.owner_healthy(control):
                if (
                    local.owner_healthy_ignoring_restart_policy(control)
                    and not local.restart_policy_safe
                ):
                    if control.mode == "observe":
                        self._log_observe_fence(
                            local, control, "应用 restart policy 不符合门禁"
                        )
                        return
                    try:
                        self._executor.ensure_restart_policy()
                    finally:
                        try:
                            self._fence(local, control.mode)
                        finally:
                            self._emit_alert(
                                "restart-policy-drift",
                                control,
                                "活动应用 restart policy 绕过启动门禁",
                                "已在线收敛策略并执行 self-fencing",
                                "检查 A Compose 持久配置后再恢复租约",
                                now,
                            )
                    return
                if control.mode == "observe":
                    self._log_observe_fence(
                        local, control, "活动 owner 本地状态不满足续租门禁"
                    )
                    return
                self._logger.error("本地状态不满足续租门禁，执行 self-fencing")
                try:
                    self._fence(local, control.mode)
                finally:
                    self._emit_alert(
                        "owner-gate-failed",
                        control,
                        "活动 owner 本地状态不满足续租门禁",
                        "已执行 self-fencing 且不再续租",
                        "检查应用、数据库角色、Tunnel 和镜像状态",
                        now,
                    )
                return
            self._last_control = control
            if allow_orchestration:
                self._orchestrate(local, control, now)
            return

        if local.app_running:
            self._logger.error("本节点不是租约 owner，但应用仍在运行")
            if control.mode == "observe":
                self._log_observe_fence(local, control, "非租约 owner 的应用仍在运行")
            else:
                try:
                    self._fence(local, control.mode)
                finally:
                    self._emit_alert(
                        "non-owner-app-running",
                        control,
                        "非租约 owner 的应用仍在运行",
                        "已执行 self-fencing",
                        "立即检查双主风险和应用停止结果",
                        now,
                    )

        if not allow_orchestration:
            return

        self._orchestrate(local, control, now)

        if self._config.node == "B" and control.expired(now):
            eligible = local.b_failover_eligible()
            if not eligible:
                self._logger.critical("A 租约已过期，但 B 接管门禁未通过")
                self._emit_alert(
                    "b-failover-gate-failed",
                    control,
                    "A 租约已过期但 B 复制、镜像或角色门禁未通过",
                    "B 未申请租约且未执行提升",
                    "检查 PostgreSQL WAL receiver、Redis、镜像和 Tunnel",
                    now,
                )
                return
            transition_id = f"failover-{uuid.uuid4()}"
            acquired, simulated = self._client.acquire_for_b(
                control.epoch, eligible, transition_id
            )
            self._last_control = acquired
            if simulated:
                self._logger.info(
                    "observe：拟执行 B 接管但未改变租约，owner=%s epoch=%s state=%s "
                    "mode=%s postgres_recovery=%s postgres_streaming=%s ntp_synchronized=%s "
                    "redis_role=%s redis_link=%s redis_sync=%s image_sync_healthy=%s tunnel_healthy=%s",
                    control.owner,
                    control.epoch,
                    control.state,
                    local.mode,
                    local.fields.get("postgres_recovery", "unknown"),
                    local.fields.get("postgres_streaming", "unknown"),
                    local.fields.get("ntp_synchronized", "unknown"),
                    local.fields.get("redis_role", "unknown"),
                    local.fields.get("redis_link", "unknown"),
                    local.fields.get("redis_sync", "unknown"),
                    local.image_sync_healthy(),
                    local.tunnel_healthy,
                )

    def _orchestrate(
        self, local: LocalState, control: ControlState, now: datetime
    ) -> None:
        if control.mode != "automatic":
            return
        if self._config.node == "B":
            self._orchestrate_b(local, control, now)
        else:
            self._orchestrate_a(local, control, now)

    def _orchestrate_b(
        self, local: LocalState, control: ControlState, now: datetime
    ) -> None:
        if control.owner != "B":
            return
        if control.state == "B_ACTIVE" and self._reconcile_active_entry(
            local, control, now, "B"
        ):
            return
        if control.state == "B_PROMOTING":
            self._orchestrate_promotion(
                local,
                control,
                now,
                node="B",
                initial_step="lease-acquired",
                action_state="B_PROMOTING",
                active_state="B_ACTIVE",
            )
            return
        if control.state != "B_FREEZING":
            return
        if control.transition_step not in {"b-frozen", "a-ready"}:
            self._run_action("B_FREEZING", control)
            self._last_control = self._client.checkpoint(
                control.epoch, "B_FREEZING", "b-frozen"
            )
            return
        if control.transition_step == "a-ready":
            self._last_control = self._client.commit_handoff(
                control.epoch,
                f"handoff-{uuid.uuid4()}",
            )

    def _orchestrate_a(
        self, local: LocalState, control: ControlState, now: datetime
    ) -> None:
        if control.owner == "B" and control.state == "B_ACTIVE":
            rebuilding = self._client.advance(
                control.epoch,
                "B_ACTIVE",
                "A_REBUILDING",
                "a-rebuild-started",
                "A 已恢复并开始从 B 重建",
            )
            self._run_action("A_REBUILDING", rebuilding)
            self._last_control = self._client.advance(
                rebuilding.epoch,
                "A_REBUILDING",
                "FAILBACK_WAIT",
                "a-rebuild-complete",
                "A 已从 B 重建并进入稳定观察",
            )
            return
        if control.owner == "B" and control.state == "A_REBUILDING":
            self._run_action("A_REBUILDING", control)
            self._last_control = self._client.advance(
                control.epoch,
                "A_REBUILDING",
                "FAILBACK_WAIT",
                "a-rebuild-complete",
                "A 已从 B 重建并进入稳定观察",
            )
            return
        if control.owner == "B" and control.state == "FAILBACK_WAIT":
            if self._ready_for_failback(local, control, now):
                self._last_control = self._client.advance(
                    control.epoch,
                    "FAILBACK_WAIT",
                    "B_FREEZING",
                    "freeze-requested",
                    "A 稳定窗口和维护窗口均已满足",
                )
            return
        if (
            control.owner == "B"
            and control.state == "B_FREEZING"
            and control.transition_step == "b-frozen"
        ):
            self._run_action("B_FREEZING", control)
            self._last_control = self._client.handoff_ready(control.epoch)
            return
        if control.owner == "A" and control.state == "A_PROMOTING":
            active = self._orchestrate_promotion(
                local,
                control,
                now,
                node="A",
                initial_step="lease-transferred",
                action_state="A_PROMOTING",
                active_state="A_ACTIVE",
            )
            if active is None:
                return
            self._last_control = self._client.advance(
                active.epoch,
                "A_ACTIVE",
                "B_REBUILDING",
                "b-rebuild-started",
                "开始从新 A 全量重建 B",
            )
            return
        if (
            control.owner == "A"
            and control.state == "A_ACTIVE"
            and self._reconcile_active_entry(local, control, now, "A")
        ):
            return
        if (
            control.owner == "A"
            and control.state == "A_ACTIVE"
            and control.transition_step == "entry-verified"
        ):
            self._last_control = self._client.advance(
                control.epoch,
                "A_ACTIVE",
                "B_REBUILDING",
                "b-rebuild-started",
                "开始从新 A 全量重建 B",
            )
            return
        if control.owner == "A" and control.state == "B_REBUILDING":
            self._run_action("B_REBUILDING", control)
            self._last_control = self._client.advance(
                control.epoch,
                "B_REBUILDING",
                "A_ACTIVE",
                "topology-restored",
                "A 主 B 备拓扑已经恢复",
            )

    def _orchestrate_promotion(
        self,
        local: LocalState,
        control: ControlState,
        now: datetime,
        node: str,
        initial_step: str,
        action_state: str,
        active_state: str,
    ) -> ControlState | None:
        """按可恢复 checkpoint 完成应用提升和公共入口验证。

        @param local: 当前节点本地状态。
        @param control: 当前权威状态。
        @param now: 当前 UTC 时间。
        @param node: 正在提升的节点。
        @param initial_step: 获得租约后的初始 checkpoint。
        @param action_state: 本地受控动作配置键。
        @param active_state: 公共入口验证后的 ACTIVE 状态。
        @return: 成功进入 ACTIVE 时返回新状态，否则返回 None。
        """
        if control.transition_step == initial_step:
            self._run_action(action_state, control)
            self._last_control = self._client.checkpoint(
                control.epoch, control.state, "service-ready"
            )
            return None
        if control.transition_step == "service-ready":
            if not self._promoted_service_healthy(local, node):
                self._pause_transition(
                    control,
                    "service-gate-failed",
                    f"节点 {node} 提升后本地应用门禁未通过",
                )
                return None
            self._last_control = self._client.switch_entry(
                node,
                control.epoch,
                f"entry-{control.transition_id}",
            )
            return None
        if control.transition_step == "entry-switched":
            if not self._promoted_service_healthy(local, node):
                self._pause_transition(
                    control,
                    "service-regressed",
                    f"节点 {node} 公共验证前本地应用状态退化",
                )
                return None
            if self._probe.public_entry_healthy():
                active = self._client.advance(
                    control.epoch,
                    control.state,
                    active_state,
                    "entry-verified",
                    f"节点 {node} 数据库、应用和公共入口已健康",
                )
                self._last_control = active
                return active
            if self._entry_verification_expired(control, now):
                self._pause_transition(
                    control,
                    "public-health-timeout",
                    f"节点 {node} 公共入口健康验证超时",
                )
            else:
                self._logger.warning(
                    "节点 %s 公共入口尚未健康，保持当前租约并等待下一轮验证", node
                )
            return None
        self._pause_transition(
            control,
            "promotion-checkpoint-invalid",
            f"状态 {control.state} 包含未知 checkpoint：{control.transition_step}",
        )
        return None

    def _reconcile_active_entry(
        self,
        local: LocalState,
        control: ControlState,
        now: datetime,
        node: str,
    ) -> bool:
        """纠正活动节点入口漂移并验证公共健康。

        @param local: 当前节点本地状态。
        @param control: 当前权威状态。
        @param now: 当前 UTC 时间。
        @param node: 当前活动节点。
        @return: 本轮已处理入口收敛时返回 True。
        """
        if control.entry_tunnel != node:
            self._last_control = self._client.switch_entry(
                node,
                control.epoch,
                f"entry-reconcile-{control.transition_id}",
            )
            return True
        if control.transition_step != "entry-switched":
            return False
        if not self._promoted_service_healthy(local, node):
            self._pause_transition(
                control,
                "entry-reconcile-service-failed",
                f"节点 {node} 入口纠正时本地门禁失败",
            )
            return True
        if self._probe.public_entry_healthy():
            self._last_control = self._client.checkpoint(
                control.epoch, control.state, "entry-reconciled"
            )
            return True
        if self._entry_verification_expired(control, now):
            self._pause_transition(
                control,
                "entry-reconcile-timeout",
                f"节点 {node} 入口纠正后的公共健康验证超时",
            )
        else:
            self._logger.warning("节点 %s 入口已纠正，等待公共健康验证", node)
        return True

    @staticmethod
    def _promoted_service_healthy(local: LocalState, node: str) -> bool:
        expected_mode = "active-recovered" if node == "A" else "active"
        return (
            local.mode == expected_mode
            and local.app_running
            and local.app_healthy
            and local.tunnel_healthy
            and local.restart_policy_safe
        )

    def _entry_verification_expired(self, control: ControlState, now: datetime) -> bool:
        return (
            now - control.transition_step_at
        ).total_seconds() >= self._config.public_health_timeout_seconds

    def _pause_transition(
        self, control: ControlState, transition_step: str, reason: str
    ) -> None:
        self._last_control = self._client.advance(
            control.epoch,
            control.state,
            "PAUSED_NEEDS_OPERATOR",
            transition_step,
            reason,
        )

    def _ready_for_failback(
        self, local: LocalState, control: ControlState, now: datetime
    ) -> bool:
        if (
            local.mode != "standby-from-b"
            or not local.tunnel_healthy
            or not local.restart_policy_safe
        ):
            return False
        if not control.stable_for(now, self._config.failback_stable_seconds):
            return False
        local_time = now.astimezone(ZoneInfo("Asia/Shanghai")).strftime("%H:%M")
        return (
            self._config.failback_window_start
            <= local_time
            < self._config.failback_window_end
        )

    def _run_action(self, state: str, control: ControlState) -> None:
        action = self._config.actions.get(state)
        if action is None:
            raise RuntimeError(f"缺少状态 {state} 的受控动作配置")
        try:
            self._keep_action_authorized(control)
            self._executor.run_action(
                action, control, lambda: self._keep_action_authorized(control)
            )
            # 重建和冻结动作结束后应用可能不存在，只有明确启动应用的动作才收敛策略。
            if action.enforce_restart_policy:
                self._executor.ensure_restart_policy()
        except Exception:
            self._logger.exception("受控动作 %s 失败，正在暂停自动编排", state)
            try:
                self._last_control = self._client.advance(
                    control.epoch,
                    control.state,
                    "PAUSED_NEEDS_OPERATOR",
                    f"{state.lower()}-failed",
                    f"受控动作 {state} 执行失败，需要人工检查",
                )
            except Exception:
                # 暂停请求失败时仍抛出原始动作异常，主循环会继续执行缓存租约 fencing。
                self._logger.exception("受控动作失败后无法更新权威暂停状态")
                self._queue_alert(
                    create_pending_alert(
                        self._config.node,
                        control.epoch,
                        control.transition_id,
                        f"{state.lower()}-failed",
                        f"受控动作 {state} 失败且无法更新控制面暂停状态",
                        "动作已中止，等待人工检查",
                        "恢复 Worker 后核对权威状态和本地数据库角色",
                        datetime.now(timezone.utc),
                    )
                )
            raise

    def _keep_action_authorized(self, expected: ControlState) -> None:
        if expected.owner == self._config.node:
            current = self._client.renew(expected.epoch)
        else:
            current = self._client.status()
        if (
            current.mode != "automatic"
            or current.owner != expected.owner
            or current.epoch != expected.epoch
            or current.state != expected.state
            or current.transition_id != expected.transition_id
            or current.expired(datetime.now(timezone.utc))
        ):
            raise RuntimeError("受控动作执行期间的租约或状态授权已经变化")
        self._last_control = current

    def _fence_if_cached_lease_expired(
        self,
        now: datetime,
        local: LocalState | None = None,
        control_unreachable: bool = False,
    ) -> None:
        control = self._last_control
        if control is not None:
            if control.owner != self._config.node or not control.expired(now):
                return
            epoch = control.epoch
            transition_id = control.transition_id
            mode = control.mode
            reason = (
                "控制面不可达且缓存租约已经到期"
                if control_unreachable
                else "权威租约已经到期"
            )
        else:
            checkpoint = self._checkpoint
            if checkpoint is None:
                if not self._checkpoint_invalid:
                    return
                epoch = 0
                transition_id = "checkpoint-invalid"
                mode = "automatic"
                reason = "控制面不可达且本地 checkpoint 无法验证"
            else:
                if checkpoint.mode == "observe":
                    return
                if checkpoint.owner == self._config.node and not checkpoint.expired(
                    now
                ):
                    return
                epoch = checkpoint.epoch
                transition_id = checkpoint.transition_id
                mode = checkpoint.mode
                reason = (
                    "控制面不可达且缓存状态表明本节点不是 owner"
                    if checkpoint.owner != self._config.node
                    else "控制面不可达且缓存租约已经到期"
                )
        self._logger.critical("%s，执行 fail-closed", reason)
        try:
            self._fence(local, mode)
        finally:
            self._queue_alert(
                create_pending_alert(
                    self._config.node,
                    epoch,
                    transition_id,
                    "lease-expired",
                    reason,
                    "已执行 fail-closed self-fencing",
                    "检查 Worker、网络和应用停止结果",
                    now,
                )
            )

    def _log_observe_fence(
        self, local: LocalState, control: ControlState, reason: str
    ) -> None:
        self._logger.warning(
            "observe：拟执行 self-fencing 但未修改资源，原因=%s node=%s owner=%s epoch=%s "
            "state=%s local_mode=%s app_running=%s app_healthy=%s tunnel_healthy=%s "
            "restart_policy_safe=%s image_sync_healthy=%s",
            reason,
            self._config.node,
            control.owner,
            control.epoch,
            control.state,
            local.mode,
            local.app_running,
            local.app_healthy,
            local.tunnel_healthy,
            local.restart_policy_safe,
            local.image_sync_healthy(),
        )

    def _fence(self, local: LocalState | None, mode: str) -> None:
        # observe 模式必须完整计算风险，但不能执行任何可变更动作。
        if mode == "observe":
            self._logger.warning("observe：若为 automatic 将停止应用")
            return
        # 租约已经失效时不能再用可能阻塞的状态探测确认容器是否存在。
        if local is None or local.app_running:
            self._executor.stop_app()

    def _emit_alert(
        self,
        event_type: str,
        control: ControlState,
        reason: str,
        result: str,
        operator_action: str,
        now: datetime,
    ) -> None:
        alert = create_pending_alert(
            self._config.node,
            control.epoch,
            control.transition_id,
            event_type,
            reason,
            result,
            operator_action,
            now,
        )
        try:
            self._client.alert(alert.as_payload())
        except Exception:
            self._logger.exception("节点关键告警发送失败，写入本地待发送队列")
            self._queue_alert(alert)

    def _flush_pending_alerts(self) -> None:
        try:
            pending = self._alerts.pending()
        except Exception:
            self._logger.exception("读取本地待发送告警失败")
            return
        for alert in pending:
            try:
                self._client.alert(alert.as_payload())
                self._alerts.remove(alert.event_id)
            except Exception:
                self._logger.exception("补发本地告警失败，保留剩余队列")
                return

    def _queue_alert(self, alert: PendingAlert) -> None:
        try:
            self._alerts.enqueue(alert)
        except Exception:
            # 告警持久化失败不能阻塞 self-fencing 或掩盖原始故障。
            self._logger.exception("写入本地待发送告警失败")
