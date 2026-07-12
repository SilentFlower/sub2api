"""节点本地待发送告警队列。"""

from __future__ import annotations

import json
import os
import tempfile
from dataclasses import asdict, dataclass
from datetime import datetime
from pathlib import Path
from typing import Any

from .config import ensure_private_directory


@dataclass(frozen=True)
class PendingAlert:
    """等待 Worker 恢复后补发的节点告警。"""

    event_id: str
    level: str
    reason: str
    result: str
    operator_action: str
    occurred_at: str

    @classmethod
    def from_mapping(cls, value: dict[str, Any]) -> "PendingAlert":
        """从持久化字典恢复告警。

        @param value: 本地 JSON 队列中的单条记录。
        @return: 已校验的待发送告警。
        """
        fields = (
            "event_id",
            "level",
            "reason",
            "result",
            "operator_action",
            "occurred_at",
        )
        if any(
            not isinstance(value.get(field), str) or not value[field]
            for field in fields
        ):
            raise ValueError("本地告警队列包含无效记录")
        return cls(**{field: value[field] for field in fields})

    def as_payload(self) -> dict[str, str]:
        """转换为 Worker 节点告警 API 负载。

        @return: 使用外部 JSON 字段名的告警负载。
        """
        return {
            "eventId": self.event_id,
            "level": self.level,
            "reason": self.reason,
            "result": self.result,
            "operatorAction": self.operator_action,
            "occurredAt": self.occurred_at,
        }


class AlertQueue:
    """使用权限受限 JSON 文件保存未发送告警。"""

    def __init__(self, path: Path) -> None:
        """创建本地告警队列。

        @param path: 队列文件路径。
        @return: 无。
        """
        self._path = path

    def pending(self) -> list[PendingAlert]:
        """读取当前待发送告警。

        @return: 按写入顺序排列的告警列表。
        """
        if not self._path.exists():
            return []
        try:
            raw = json.loads(self._path.read_text(encoding="utf-8"))
        except (OSError, json.JSONDecodeError) as exc:
            raise RuntimeError(f"无法读取本地告警队列：{self._path}") from exc
        if not isinstance(raw, list):
            raise RuntimeError("本地告警队列根节点必须是数组")
        alerts: list[PendingAlert] = []
        for item in raw:
            if not isinstance(item, dict):
                raise RuntimeError("本地告警队列包含非对象记录")
            alerts.append(PendingAlert.from_mapping(item))
        return alerts

    def enqueue(self, alert: PendingAlert) -> None:
        """按 event ID 幂等加入告警。

        @param alert: 待发送告警。
        @return: 无。
        """
        alerts = self.pending()
        if any(item.event_id == alert.event_id for item in alerts):
            return
        alerts.append(alert)
        self._write(alerts)

    def remove(self, event_id: str) -> None:
        """删除已经由 Worker 接收的告警。

        @param event_id: 已接收事件 ID。
        @return: 无。
        """
        alerts = [item for item in self.pending() if item.event_id != event_id]
        self._write(alerts)

    def _write(self, alerts: list[PendingAlert]) -> None:
        ensure_private_directory(self._path.parent)
        descriptor, temporary = tempfile.mkstemp(
            prefix=f".{self._path.name}.", dir=self._path.parent
        )
        try:
            with os.fdopen(descriptor, "w", encoding="utf-8") as handle:
                json.dump(
                    [asdict(alert) for alert in alerts],
                    handle,
                    ensure_ascii=False,
                    sort_keys=True,
                )
                handle.write("\n")
                handle.flush()
                os.fsync(handle.fileno())
            os.chmod(temporary, 0o600)
            os.replace(temporary, self._path)
        finally:
            if os.path.exists(temporary):
                os.unlink(temporary)


def create_pending_alert(
    node: str,
    epoch: int,
    transition_id: str,
    event_type: str,
    reason: str,
    result: str,
    operator_action: str,
    occurred_at: datetime,
) -> PendingAlert:
    """创建具有稳定去重 ID 的关键节点告警。

    @param node: 产生告警的节点。
    @param epoch: 最近已知权威 epoch。
    @param transition_id: 最近已知迁移 ID。
    @param event_type: 稳定事件类型。
    @param reason: 告警原因。
    @param result: 节点处置结果。
    @param operator_action: 建议人工动作。
    @param occurred_at: 事件发生时间。
    @return: 待发送告警。
    """
    return PendingAlert(
        event_id=f"node:{node}:{epoch}:{transition_id}:{event_type}",
        level="CRITICAL",
        reason=reason,
        result=result,
        operator_action=operator_action,
        occurred_at=occurred_at.isoformat(),
    )
