"""Cloudflare Worker 控制面客户端。"""

from __future__ import annotations

import hashlib
import hmac
import json
import secrets
import urllib.error
import urllib.request
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

from .model import ControlState


USER_AGENT = "sub2api-ha-agent/0.1"


class ControlPlaneError(RuntimeError):
    """控制面请求失败。"""


class ControlPlaneClient:
    """使用节点 HMAC 身份调用 Worker。"""

    def __init__(
        self, base_url: str, node: str, secret: str, timeout_seconds: int
    ) -> None:
        """创建控制面客户端。

        @param base_url: Worker 基础地址。
        @param node: A 或 B。
        @param secret: 节点 HMAC 密钥。
        @param timeout_seconds: 单次请求超时。
        @return: 无。
        """
        self._base_url = base_url.rstrip("/")
        self._node = node
        self._secret = secret.encode("utf-8")
        self._timeout_seconds = timeout_seconds

    def status(self) -> ControlState:
        """读取权威状态。

        @return: Worker 当前权威状态。
        """
        return ControlState.from_payload(self._request("GET", "/v1/status", {}))

    def report(self, payload: dict[str, Any]) -> ControlState:
        """上报节点本地状态。

        @param payload: 节点状态负载。
        @return: 当前权威状态。
        """
        return ControlState.from_payload(
            self._request("POST", "/v1/node/report", payload)
        )

    def alert(self, payload: dict[str, Any]) -> ControlState:
        """发送节点本地积压告警。

        @param payload: 稳定事件 ID 和告警内容。
        @return: 当前权威状态。
        """
        result = self._request("POST", "/v1/node/alert", payload)
        return ControlState.from_payload(self._unwrap_state_result(result))

    def renew(self, epoch: int) -> ControlState:
        """续租当前节点。

        @param epoch: 当前 epoch。
        @return: 续租后的权威状态。
        """
        result = self._request("POST", "/v1/lease/renew", {"epoch": epoch})
        return ControlState.from_payload(self._unwrap_state_result(result))

    def acquire_for_b(
        self, expected_epoch: int, eligible: bool, transition_id: str
    ) -> tuple[ControlState, bool]:
        """由 B 请求故障接管。

        @param expected_epoch: B 已观察到的 epoch。
        @param eligible: 本地门禁是否通过。
        @param transition_id: 本次迁移 ID。
        @return: 权威状态与是否为模拟执行。
        """
        result = self._request(
            "POST",
            "/v1/lease/acquire",
            {
                "expectedEpoch": expected_epoch,
                "eligible": eligible,
                "transitionId": transition_id,
            },
        )
        return ControlState.from_payload(self._unwrap_state_result(result)), bool(
            result.get("simulated")
        )

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
        result = self._request(
            "POST",
            "/v1/transition/advance",
            {
                "epoch": epoch,
                "expectedState": expected_state,
                "nextState": next_state,
                "transitionStep": transition_step,
                "reason": reason,
            },
        )
        return ControlState.from_payload(self._unwrap_state_result(result))

    def checkpoint(
        self, epoch: int, expected_state: str, transition_step: str
    ) -> ControlState:
        """写入 owner 节点阶段 checkpoint。

        @param epoch: 当前 epoch。
        @param expected_state: 预期当前状态。
        @param transition_step: 新阶段标记。
        @return: 更新后的权威状态。
        """
        result = self._request(
            "POST",
            "/v1/transition/checkpoint",
            {
                "epoch": epoch,
                "expectedState": expected_state,
                "transitionStep": transition_step,
            },
        )
        return ControlState.from_payload(self._unwrap_state_result(result))

    def handoff_ready(self, epoch: int) -> ControlState:
        """由 A 标记已经达到 B 冻结点。

        @param epoch: 当前 epoch。
        @return: 更新后的权威状态。
        """
        result = self._request("POST", "/v1/handoff/ready", {"epoch": epoch})
        return ControlState.from_payload(self._unwrap_state_result(result))

    def commit_handoff(self, epoch: int, transition_id: str) -> ControlState:
        """由 B 原子提交租约到 A。

        @param epoch: B 当前持有的 epoch。
        @param transition_id: 新 handoff 迁移 ID。
        @return: A 取得新 epoch 后的权威状态。
        """
        result = self._request(
            "POST",
            "/v1/handoff/commit",
            {"epoch": epoch, "transitionId": transition_id, "aReady": True},
        )
        return ControlState.from_payload(self._unwrap_state_result(result))

    def switch_entry(self, target: str, epoch: int, request_id: str) -> ControlState:
        """把公共入口切换到目标 HA Tunnel。

        @param target: 目标节点。
        @param epoch: 当前 epoch。
        @param request_id: 幂等请求 ID。
        @return: 入口提交后的权威状态。
        """
        result = self._request(
            "POST",
            "/v1/entry/switch",
            {"target": target, "epoch": epoch, "requestId": request_id},
        )
        return ControlState.from_payload(self._unwrap_state_result(result))

    def _request(
        self, method: str, path: str, payload: dict[str, Any]
    ) -> dict[str, Any]:
        raw = (
            ""
            if method == "GET"
            else json.dumps(payload, separators=(",", ":"), ensure_ascii=False)
        )
        timestamp = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
        nonce = secrets.token_urlsafe(24)
        signature_payload = "\n".join((method, path, timestamp, nonce, raw)).encode(
            "utf-8"
        )
        signature = hmac.new(
            self._secret, signature_payload, hashlib.sha256
        ).hexdigest()
        request = urllib.request.Request(
            f"{self._base_url}{path}",
            data=None if method == "GET" else raw.encode("utf-8"),
            method=method,
            headers={
                "content-type": "application/json",
                "user-agent": USER_AGENT,
                "x-ha-node": self._node,
                "x-ha-timestamp": timestamp,
                "x-ha-nonce": nonce,
                "x-ha-signature": signature,
            },
        )
        try:
            with urllib.request.urlopen(
                request, timeout=self._timeout_seconds
            ) as response:
                body = json.loads(response.read().decode("utf-8"))
        except (urllib.error.URLError, TimeoutError, json.JSONDecodeError) as exc:
            raise ControlPlaneError(f"控制面请求失败：{method} {path}") from exc
        if (
            not isinstance(body, dict)
            or body.get("ok") is not True
            or not isinstance(body.get("data"), dict)
        ):
            error = body.get("error", {}) if isinstance(body, dict) else {}
            code = (
                error.get("code", "INVALID_RESPONSE")
                if isinstance(error, dict)
                else "INVALID_RESPONSE"
            )
            message = (
                error.get("message", "控制面返回无效")
                if isinstance(error, dict)
                else "控制面返回无效"
            )
            raise ControlPlaneError(f"{code}: {message}")
        return body["data"]

    @staticmethod
    def _unwrap_state_result(payload: dict[str, Any]) -> dict[str, Any]:
        state = payload.get("state")
        if not isinstance(state, dict):
            raise ControlPlaneError("控制面状态结果缺少 state")
        return state


class AdminControlClient:
    """使用独立管理员 Token 调用人工控制 API。"""

    def __init__(self, base_url: str, token: str, timeout_seconds: int) -> None:
        """创建管理员控制客户端。

        @param base_url: Worker 基础地址。
        @param token: 管理员 Bearer Token。
        @param timeout_seconds: 单次请求超时。
        @return: 无。
        """
        self._base_url = base_url.rstrip("/")
        self._token = token
        self._timeout_seconds = timeout_seconds

    @classmethod
    def from_token_file(
        cls,
        base_url: str,
        token_file: str | Path,
        timeout_seconds: int,
    ) -> "AdminControlClient":
        """从权限受限文件创建管理员客户端。

        @param base_url: Worker 基础地址。
        @param token_file: 管理员 Token 文件。
        @param timeout_seconds: 单次请求超时。
        @return: 管理员控制客户端。
        """
        path = Path(token_file)
        try:
            mode = path.stat().st_mode & 0o777
            token = path.read_text(encoding="utf-8").strip()
        except OSError as exc:
            raise ControlPlaneError(f"无法读取管理员 Token：{path}") from exc
        if mode & 0o077:
            raise ControlPlaneError(f"管理员 Token 权限必须为 0600：{path}")
        if len(token) < 32:
            raise ControlPlaneError("管理员 Token 长度至少为 32 个字符")
        return cls(base_url, token, timeout_seconds)

    def set_mode(self, mode: str) -> ControlState:
        """切换 observe、automatic 或 paused 模式。

        @param mode: 目标控制模式。
        @return: 更新后的权威状态。
        """
        result = self._request("/v1/control/mode", {"mode": mode})
        return ControlState.from_payload(self._unwrap_state(result))

    def pause(self, reason: str) -> ControlState:
        """暂停新的自动编排但保留当前租约。

        @param reason: 人工暂停原因。
        @return: 更新后的权威状态。
        """
        result = self._request("/v1/control/pause", {"reason": reason})
        return ControlState.from_payload(self._unwrap_state(result))

    def resume(
        self,
        expected_epoch: int,
        owner: str,
        state: str,
        mode: str,
        transition_id: str,
        reason: str,
    ) -> ControlState:
        """在确认唯一权威节点后恢复暂停状态。

        @param expected_epoch: 当前控制面 epoch。
        @param owner: 已人工确认的唯一 owner。
        @param state: 与真实拓扑一致的恢复状态。
        @param mode: 恢复后的 observe 或 automatic 模式。
        @param transition_id: 本次人工恢复 ID。
        @param reason: 人工确认依据。
        @return: 更新后的权威状态。
        """
        result = self._request(
            "/v1/control/resume",
            {
                "expectedEpoch": expected_epoch,
                "owner": owner,
                "state": state,
                "mode": mode,
                "transitionId": transition_id,
                "reason": reason,
            },
        )
        return ControlState.from_payload(self._unwrap_state(result))

    def emergency_freeze(self, reason: str) -> ControlState:
        """清除租约 owner 并要求所有节点 fail-closed。

        @param reason: 紧急冻结原因。
        @return: 更新后的权威状态。
        """
        result = self._request("/v1/control/emergency-freeze", {"reason": reason})
        return ControlState.from_payload(self._unwrap_state(result))

    def _request(self, path: str, payload: dict[str, Any]) -> dict[str, Any]:
        raw = json.dumps(payload, separators=(",", ":"), ensure_ascii=False).encode(
            "utf-8"
        )
        request = urllib.request.Request(
            f"{self._base_url}{path}",
            data=raw,
            method="POST",
            headers={
                "authorization": f"Bearer {self._token}",
                "content-type": "application/json",
                "user-agent": USER_AGENT,
            },
        )
        try:
            with urllib.request.urlopen(
                request, timeout=self._timeout_seconds
            ) as response:
                body = json.loads(response.read().decode("utf-8"))
        except (urllib.error.URLError, TimeoutError, json.JSONDecodeError) as exc:
            raise ControlPlaneError(f"管理员控制请求失败：{path}") from exc
        if (
            not isinstance(body, dict)
            or body.get("ok") is not True
            or not isinstance(body.get("data"), dict)
        ):
            raise ControlPlaneError(f"管理员控制返回无效：{path}")
        return body["data"]

    @staticmethod
    def _unwrap_state(payload: dict[str, Any]) -> dict[str, Any]:
        state = payload.get("state")
        if not isinstance(state, dict):
            raise ControlPlaneError("管理员控制结果缺少 state")
        return state
