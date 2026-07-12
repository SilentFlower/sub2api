"""节点本地状态探测。"""

from __future__ import annotations

import subprocess
import urllib.error
import urllib.request
from typing import Protocol

from .config import AgentConfig
from .model import LocalState


class Probe(Protocol):
    """节点状态探测接口。"""

    def collect(self) -> LocalState:
        """采集一次节点状态。

        @return: 节点本地状态。
        """
        ...

    def public_entry_healthy(self) -> bool:
        """检查 HA 公共入口健康状态。

        @return: 公共入口健康时返回 True。
        """
        ...


def parse_machine_state(output: str) -> dict[str, str]:
    """解析 `key=value` 机器状态。

    @param output: 状态命令标准输出。
    @return: 字段字典。
    """
    fields: dict[str, str] = {}
    for raw_line in output.splitlines():
        line = raw_line.strip()
        if not line:
            continue
        key, separator, value = line.partition("=")
        if not separator or not key:
            raise ValueError(f"机器状态包含非法行：{raw_line}")
        if key in fields:
            raise ValueError(f"机器状态字段重复：{key}")
        fields[key] = value
    if "mode" not in fields:
        raise ValueError("机器状态缺少 mode")
    return fields


def restart_policy_is_safe(
    app_container_state: str, actual: str, expected: str
) -> bool:
    """判断应用容器是否满足启动门禁。

    @param app_container_state: `status --machine` 返回的应用容器状态。
    @param actual: Docker 返回的实际 restart policy。
    @param expected: 配置要求的 restart policy。
    @return: 容器不存在或策略匹配时返回 True。
    """
    return app_container_state == "absent" or actual == expected


class SystemProbe:
    """通过现有脚本、Docker 和 systemd 采集状态。"""

    def __init__(self, config: AgentConfig) -> None:
        """创建系统探测器。

        @param config: 节点配置。
        @return: 无。
        """
        self._config = config

    def collect(self) -> LocalState:
        """采集一次节点状态。

        @return: 节点本地状态。
        """
        status = subprocess.run(
            self._config.state_command,
            check=False,
            capture_output=True,
            text=True,
            timeout=self._config.command_timeout_seconds,
        )
        if status.returncode not in {0, 2}:
            raise RuntimeError(f"状态命令失败，退出码={status.returncode}")
        fields = parse_machine_state(status.stdout)
        app_running = fields.get("app_container") == "running"
        tunnel_healthy = self._command_ok(
            ("systemctl", "is-active", "--quiet", self._config.tunnel_service)
        )
        restart_policy = self._command_output(
            (
                "docker",
                "inspect",
                "--format",
                "{{.HostConfig.RestartPolicy.Name}}",
                self._config.app_container,
            )
        )
        app_healthy = app_running and self._http_healthy(self._config.app_health_url)
        return LocalState(
            node=self._config.node,
            fields=fields,
            app_running=app_running,
            app_healthy=app_healthy,
            tunnel_healthy=tunnel_healthy,
            restart_policy_safe=restart_policy_is_safe(
                fields.get("app_container", "unknown"),
                restart_policy,
                self._config.expected_restart_policy,
            ),
        )

    def public_entry_healthy(self) -> bool:
        """从公共域名检查 HA 入口健康状态。

        @return: 公共入口健康时返回 True。
        """
        return self._http_healthy(self._config.public_health_url)

    def _command_ok(self, command: tuple[str, ...]) -> bool:
        result = subprocess.run(
            command,
            check=False,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            timeout=self._config.command_timeout_seconds,
        )
        return result.returncode == 0

    def _command_output(self, command: tuple[str, ...]) -> str:
        result = subprocess.run(
            command,
            check=False,
            capture_output=True,
            text=True,
            timeout=self._config.command_timeout_seconds,
        )
        return result.stdout.strip() if result.returncode == 0 else "unknown"

    def _http_healthy(self, url: str) -> bool:
        try:
            with urllib.request.urlopen(
                url, timeout=self._config.request_timeout_seconds
            ) as response:
                return 200 <= response.status < 300
        except (urllib.error.URLError, TimeoutError):
            return False
