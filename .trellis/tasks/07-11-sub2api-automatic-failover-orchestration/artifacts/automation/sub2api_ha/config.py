"""HA agent 配置读取与校验。"""

from __future__ import annotations

import json
import os
from dataclasses import dataclass
from pathlib import Path
from typing import Any


class ConfigError(ValueError):
    """配置不完整或不安全。"""


@dataclass(frozen=True)
class ActionCommand:
    """某个状态对应的受控本地命令。"""

    command: tuple[str, ...]
    stdin: str
    timeout_seconds: int
    enforce_restart_policy: bool


@dataclass(frozen=True)
class AgentConfig:
    """HA agent 的静态配置。"""

    node: str
    control_url: str
    secret_file: Path
    state_command: tuple[str, ...]
    app_container: str
    stop_app_command: tuple[str, ...]
    start_app_command: tuple[str, ...]
    ensure_restart_policy_command: tuple[str, ...]
    app_health_url: str
    public_health_url: str
    tunnel_service: str
    expected_restart_policy: str
    actions: dict[str, ActionCommand]
    interval_seconds: int = 5
    request_timeout_seconds: int = 4
    command_timeout_seconds: int = 30
    public_health_timeout_seconds: int = 90
    lock_file: Path = Path("/run/sub2api-ha-agent.lock")
    checkpoint_file: Path = Path("/var/lib/sub2api-ha-agent/checkpoint.json")
    pending_alert_file: Path = Path("/var/lib/sub2api-ha-agent/pending-alerts.json")
    failback_stable_seconds: int = 1800
    failback_window_start: str = "04:00"
    failback_window_end: str = "05:00"

    @classmethod
    def from_file(cls, path: str | Path) -> "AgentConfig":
        """从 JSON 文件读取配置。

        @param path: 配置文件路径。
        @return: 已校验的配置对象。
        """
        config_path = Path(path)
        try:
            raw = json.loads(config_path.read_text(encoding="utf-8"))
        except (OSError, json.JSONDecodeError) as exc:
            raise ConfigError(f"无法读取配置文件：{config_path}") from exc
        if not isinstance(raw, dict):
            raise ConfigError("配置根节点必须是对象")
        return cls.from_mapping(raw)

    @classmethod
    def from_mapping(cls, raw: dict[str, Any]) -> "AgentConfig":
        """从字典构造配置。

        @param raw: 配置字典。
        @return: 已校验的配置对象。
        """
        node = cls._required_string(raw, "node")
        if node not in {"A", "B"}:
            raise ConfigError("node 必须是 A 或 B")
        control_url = cls._required_string(raw, "control_url").rstrip("/")
        if not control_url.startswith("https://"):
            raise ConfigError("control_url 必须使用 https://")
        interval = cls._positive_int(raw, "interval_seconds", 5)
        request_timeout = cls._positive_int(raw, "request_timeout_seconds", 4)
        if request_timeout >= interval:
            raise ConfigError("request_timeout_seconds 必须小于 interval_seconds")
        public_health_url = cls._required_string(raw, "public_health_url")
        if not public_health_url.startswith("https://"):
            raise ConfigError("public_health_url 必须使用 https://")
        return cls(
            node=node,
            control_url=control_url,
            secret_file=Path(cls._required_string(raw, "secret_file")),
            state_command=cls._command(raw, "state_command"),
            app_container=cls._required_string(raw, "app_container"),
            stop_app_command=cls._command(raw, "stop_app_command"),
            start_app_command=cls._command(raw, "start_app_command"),
            ensure_restart_policy_command=cls._command(
                raw, "ensure_restart_policy_command"
            ),
            app_health_url=cls._required_string(raw, "app_health_url"),
            public_health_url=public_health_url,
            tunnel_service=cls._required_string(raw, "tunnel_service"),
            expected_restart_policy=cls._required_string(
                raw, "expected_restart_policy"
            ),
            actions=cls._actions(raw.get("actions", {})),
            interval_seconds=interval,
            request_timeout_seconds=request_timeout,
            command_timeout_seconds=cls._positive_int(
                raw, "command_timeout_seconds", 30
            ),
            public_health_timeout_seconds=cls._positive_int(
                raw, "public_health_timeout_seconds", 90
            ),
            lock_file=Path(raw.get("lock_file", "/run/sub2api-ha-agent.lock")),
            checkpoint_file=Path(
                raw.get("checkpoint_file", "/var/lib/sub2api-ha-agent/checkpoint.json")
            ),
            pending_alert_file=Path(
                raw.get(
                    "pending_alert_file",
                    "/var/lib/sub2api-ha-agent/pending-alerts.json",
                )
            ),
            failback_stable_seconds=cls._positive_int(
                raw, "failback_stable_seconds", 1800
            ),
            failback_window_start=cls._clock(
                raw.get("failback_window_start", "04:00"), "failback_window_start"
            ),
            failback_window_end=cls._clock(
                raw.get("failback_window_end", "05:00"), "failback_window_end"
            ),
        )

    def read_secret(self) -> str:
        """读取节点 HMAC 密钥并检查权限。

        @return: 去除首尾空白后的密钥。
        """
        try:
            mode = self.secret_file.stat().st_mode & 0o777
            value = self.secret_file.read_text(encoding="utf-8").strip()
        except OSError as exc:
            raise ConfigError(f"无法读取节点密钥：{self.secret_file}") from exc
        if mode & 0o077:
            raise ConfigError(f"节点密钥权限必须为 0600：{self.secret_file}")
        if len(value) < 32:
            raise ConfigError("节点密钥长度至少为 32 个字符")
        return value

    @staticmethod
    def _required_string(raw: dict[str, Any], key: str) -> str:
        value = raw.get(key)
        if not isinstance(value, str) or not value.strip():
            raise ConfigError(f"缺少非空字符串配置：{key}")
        return value.strip()

    @staticmethod
    def _command(raw: dict[str, Any], key: str) -> tuple[str, ...]:
        value = raw.get(key)
        if (
            not isinstance(value, list)
            or not value
            or not all(isinstance(item, str) and item for item in value)
        ):
            raise ConfigError(f"配置 {key} 必须是非空字符串数组")
        return tuple(value)

    @staticmethod
    def _positive_int(raw: dict[str, Any], key: str, default: int) -> int:
        value = raw.get(key, default)
        if not isinstance(value, int) or isinstance(value, bool) or value <= 0:
            raise ConfigError(f"配置 {key} 必须是正整数")
        return value

    @classmethod
    def _actions(cls, value: Any) -> dict[str, ActionCommand]:
        if not isinstance(value, dict):
            raise ConfigError("actions 必须是对象")
        actions: dict[str, ActionCommand] = {}
        for state, raw_action in value.items():
            if not isinstance(state, str) or not isinstance(raw_action, dict):
                raise ConfigError("actions 的状态和配置格式无效")
            actions[state] = ActionCommand(
                command=cls._command(raw_action, "command"),
                stdin=str(raw_action.get("stdin", "")),
                timeout_seconds=cls._positive_int(raw_action, "timeout_seconds", 3600),
                enforce_restart_policy=cls._boolean(
                    raw_action, "enforce_restart_policy", False
                ),
            )
        return actions

    @staticmethod
    def _boolean(raw: dict[str, Any], key: str, default: bool) -> bool:
        value = raw.get(key, default)
        if not isinstance(value, bool):
            raise ConfigError(f"配置 {key} 必须是布尔值")
        return value

    @staticmethod
    def _clock(value: Any, key: str) -> str:
        if not isinstance(value, str):
            raise ConfigError(f"配置 {key} 必须是 HH:MM")
        parts = value.split(":")
        if len(parts) != 2 or not all(part.isdigit() for part in parts):
            raise ConfigError(f"配置 {key} 必须是 HH:MM")
        hour, minute = (int(part) for part in parts)
        if not 0 <= hour <= 23 or not 0 <= minute <= 59:
            raise ConfigError(f"配置 {key} 必须是有效时间")
        return f"{hour:02d}:{minute:02d}"


def ensure_private_directory(path: Path) -> None:
    """创建仅 root 可访问的状态目录。

    @param path: 目标目录。
    @return: 无。
    """
    path.mkdir(parents=True, exist_ok=True, mode=0o700)
    os.chmod(path, 0o700)
