"""HA agent 数据模型。"""

from __future__ import annotations

from dataclasses import asdict, dataclass
from datetime import datetime, timezone
from typing import Any


@dataclass(frozen=True)
class ControlState:
    """Cloudflare 控制面返回的权威状态。"""

    owner: str
    epoch: int
    lease_until: datetime
    state: str
    mode: str
    transition_id: str
    transition_step: str
    transition_step_at: datetime
    stable_since: datetime | None
    entry_tunnel: str

    @classmethod
    def from_payload(cls, payload: dict[str, Any]) -> "ControlState":
        """从 Worker JSON 数据构造权威状态。

        @param payload: Worker data 字段。
        @return: 权威状态对象。
        """
        try:
            lease_until = datetime.fromisoformat(
                str(payload["leaseUntil"]).replace("Z", "+00:00")
            )
            if lease_until.tzinfo is None:
                lease_until = lease_until.replace(tzinfo=timezone.utc)
            stable_raw = payload.get("stableSince")
            stable_since = None
            if stable_raw:
                stable_since = datetime.fromisoformat(
                    str(stable_raw).replace("Z", "+00:00")
                )
                if stable_since.tzinfo is None:
                    stable_since = stable_since.replace(tzinfo=timezone.utc)
            transition_step_at = datetime.fromisoformat(
                str(payload.get("transitionStepAt", payload["updatedAt"])).replace(
                    "Z", "+00:00"
                )
            )
            if transition_step_at.tzinfo is None:
                transition_step_at = transition_step_at.replace(tzinfo=timezone.utc)
            return cls(
                owner=str(payload["owner"]),
                epoch=int(payload["epoch"]),
                lease_until=lease_until,
                state=str(payload["state"]),
                mode=str(payload["mode"]),
                transition_id=str(payload["transitionId"]),
                transition_step=str(payload["transitionStep"]),
                transition_step_at=transition_step_at,
                stable_since=stable_since,
                entry_tunnel=str(payload["entryTunnel"]),
            )
        except (KeyError, TypeError, ValueError) as exc:
            raise ValueError("控制面响应缺少有效状态字段") from exc

    def expired(self, now: datetime) -> bool:
        """判断租约是否已经到期。

        @param now: 当前 UTC 时间。
        @return: 到期返回 True。
        """
        return self.lease_until <= now

    def stable_for(self, now: datetime, seconds: int) -> bool:
        """判断稳定窗口是否已经满足。

        @param now: 当前 UTC 时间。
        @param seconds: 要求持续秒数。
        @return: 满足返回 True。
        """
        return (
            self.stable_since is not None
            and (now - self.stable_since).total_seconds() >= seconds
        )


@dataclass(frozen=True)
class LocalState:
    """节点本地机器状态。"""

    node: str
    fields: dict[str, str]
    app_running: bool
    app_healthy: bool
    tunnel_healthy: bool
    restart_policy_safe: bool

    @property
    def mode(self) -> str:
        """返回现有容灾脚本识别的本地模式。

        @return: 本地容灾模式。
        """
        return self.fields.get("mode", "unknown")

    def image_sync_healthy(self) -> bool:
        """判断当前节点的发布镜像是否满足写入门禁。

        @return: 双端镜像同步或本地固定镜像记录一致时返回 True。
        """
        if self.fields.get("image_sync") == "ok":
            return True
        digest = self.fields.get("app_image_digest")
        return (
            bool(digest)
            and digest != "unknown"
            and self.fields.get("app_image_cached") == "yes"
            and digest == self.fields.get("release_image_digest")
        )

    def owner_healthy(self, control: ControlState) -> bool:
        """判断当前节点是否满足续租门禁。

        @param control: 权威控制状态。
        @return: 满足续租门禁返回 True。
        """
        if not self.tunnel_healthy or self.mode == "inconsistent":
            return False
        if control.state in {"A_ACTIVE", "B_ACTIVE"}:
            expected_modes = {
                "A": {"legacy-active", "active-recovered"},
                "B": {"active"},
            }
            # A 当前运行镜像是发布权威；B 尚未同步只降低接管就绪度，不能停止健康主节点。
            image_ready = self.node == "A" or self.image_sync_healthy()
            return (
                self.mode in expected_modes[self.node]
                and self.app_running
                and self.app_healthy
                and image_ready
                and self.restart_policy_safe
            )
        return True

    def owner_healthy_ignoring_restart_policy(self, control: ControlState) -> bool:
        """判断除 restart policy 外的 owner 门禁。

        @param control: 权威控制状态。
        @return: 其它门禁通过返回 True。
        """
        if not self.tunnel_healthy or self.mode == "inconsistent":
            return False
        if control.state in {"A_ACTIVE", "B_ACTIVE"}:
            expected_modes = {
                "A": {"legacy-active", "active-recovered"},
                "B": {"active"},
            }
            image_ready = self.node == "A" or self.image_sync_healthy()
            return (
                self.mode in expected_modes[self.node]
                and self.app_running
                and self.app_healthy
                and image_ready
            )
        return True

    def b_failover_eligible(self) -> bool:
        """判断 B 是否满足故障接管前置条件。

        @return: 全部接管门禁通过时返回 True。
        """
        return (
            self.node == "B"
            and self.mode == "standby"
            and self.fields.get("postgres_recovery") == "t"
            and self.fields.get("postgres_streaming") == "streaming"
            and self.fields.get("ntp_synchronized") == "yes"
            and self.fields.get("redis_role") == "slave"
            and self.fields.get("redis_link") == "up"
            and self.fields.get("redis_sync") == "0"
            and self.fields.get("app_container") != "running"
            and self.image_sync_healthy()
            and self.tunnel_healthy
        )

    def report_payload(self, epoch: int) -> dict[str, Any]:
        """生成节点上报负载。

        @param epoch: 节点当前已知 epoch。
        @return: 可序列化字典。
        """
        database_role = "unknown"
        if self.fields.get("postgres_recovery") == "t":
            database_role = "standby"
        elif self.fields.get("postgres_recovery") == "f":
            database_role = "primary"
        redis_role = self.fields.get("redis_role", "unknown")
        if redis_role == "slave":
            redis_role = "replica"
        return {
            "node": self.node,
            "epoch": epoch,
            "mode": self.mode,
            "appHealthy": self.app_healthy,
            "appRunning": self.app_running,
            "databaseRole": database_role,
            "redisRole": redis_role
            if redis_role in {"master", "replica"}
            else "unknown",
            "replicationHealthy": self.fields.get("redis_link") == "up"
            and self.fields.get("redis_sync") == "0",
            "imageSyncHealthy": self.image_sync_healthy(),
            "tunnelHealthy": self.tunnel_healthy,
            "restartPolicySafe": self.restart_policy_safe,
            "observedAt": datetime.now(timezone.utc).isoformat(),
        }


@dataclass(frozen=True)
class Checkpoint:
    """本地持久化的最后权威观察结果。"""

    owner: str
    epoch: int
    lease_until: str
    state: str
    mode: str
    transition_id: str
    transition_step: str
    updated_at: str

    @classmethod
    def from_mapping(cls, payload: dict[str, Any]) -> "Checkpoint":
        """从本地 JSON 字典恢复 checkpoint。

        @param payload: checkpoint 文件内容。
        @return: 已校验的 checkpoint。
        """
        try:
            return cls(
                owner=str(payload["owner"]),
                epoch=int(payload["epoch"]),
                lease_until=str(payload["lease_until"]),
                state=str(payload["state"]),
                mode=str(payload["mode"]),
                transition_id=str(payload.get("transition_id", "checkpoint-legacy")),
                transition_step=str(
                    payload.get("transition_step", "checkpoint-legacy")
                ),
                updated_at=str(payload["updated_at"]),
            )
        except (KeyError, TypeError, ValueError) as exc:
            raise ValueError("checkpoint 缺少有效字段") from exc

    @classmethod
    def from_control(cls, control: ControlState, now: datetime) -> "Checkpoint":
        """从权威状态生成 checkpoint。

        @param control: Worker 返回的权威状态。
        @param now: checkpoint 写入时间。
        @return: 可持久化的 checkpoint。
        """
        return cls(
            owner=control.owner,
            epoch=control.epoch,
            lease_until=control.lease_until.isoformat(),
            state=control.state,
            mode=control.mode,
            transition_id=control.transition_id,
            transition_step=control.transition_step,
            updated_at=now.isoformat(),
        )

    def expired(self, now: datetime) -> bool:
        """判断 checkpoint 中的租约是否已经到期。

        @param now: 当前 UTC 时间。
        @return: 租约到期或时间字段无效时返回 True。
        """
        try:
            lease_until = datetime.fromisoformat(
                self.lease_until.replace("Z", "+00:00")
            )
        except ValueError:
            return True
        if lease_until.tzinfo is None:
            lease_until = lease_until.replace(tzinfo=timezone.utc)
        return lease_until <= now

    def as_dict(self) -> dict[str, Any]:
        """返回可序列化字典。

        @return: checkpoint 字段字典。
        """
        return asdict(self)
