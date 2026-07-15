"""节点本地状态探测。"""

from __future__ import annotations

import subprocess
import time
import urllib.error
import urllib.request
from typing import Protocol

from .config import AgentConfig
from .model import LocalState


class Probe(Protocol):
    """节点状态探测接口。"""

    def collect_lease_state(self) -> LocalState:
        """采集租约续期所需的本地关键状态。

        @return: 不依赖跨节点调用的本地状态。
        """
        ...

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
        return self._collect_state(
            self._config.state_command,
            self._config.detailed_probe_timeout_seconds,
        )

    def collect_lease_state(self) -> LocalState:
        """在严格时间预算内采集租约关键状态。

        @return: 仅用于本轮节点报告和续租门禁的本地状态。
        """
        return self._collect_state(
            self._config.lease_state_command,
            self._config.lease_probe_timeout_seconds,
        )

    def _collect_state(
        self, command: tuple[str, ...], timeout_seconds: int
    ) -> LocalState:
        """在单一总预算内执行状态脚本及本地门禁检查。

        @param command: 机器状态命令。
        @param timeout_seconds: 本次完整探测的总时间预算。
        @return: 节点本地状态。
        """
        deadline = time.monotonic() + timeout_seconds
        status = subprocess.run(
            command,
            check=False,
            capture_output=True,
            text=True,
            timeout=self._remaining_timeout(deadline),
        )
        if status.returncode not in {0, 2}:
            raise RuntimeError(f"状态命令失败，退出码={status.returncode}")
        fields = parse_machine_state(status.stdout)
        app_running = fields.get("app_container") == "running"
        tunnel_healthy = self._command_ok(
            ("systemctl", "is-active", "--quiet", self._config.tunnel_service),
            self._remaining_timeout(deadline),
        )
        restart_policy = self._command_output(
            (
                "docker",
                "inspect",
                "--format",
                "{{.HostConfig.RestartPolicy.Name}}",
                self._config.app_container,
            ),
            self._remaining_timeout(deadline),
        )
        app_healthy = app_running and self._http_healthy(
            self._config.app_health_url,
            min(
                self._config.request_timeout_seconds,
                self._remaining_timeout(deadline),
            ),
        )
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
        return self._http_healthy(
            self._config.public_health_url, self._config.request_timeout_seconds
        )

    def _command_ok(self, command: tuple[str, ...], timeout_seconds: float) -> bool:
        result = subprocess.run(
            command,
            check=False,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            timeout=timeout_seconds,
        )
        return result.returncode == 0

    def _command_output(self, command: tuple[str, ...], timeout_seconds: float) -> str:
        result = subprocess.run(
            command,
            check=False,
            capture_output=True,
            text=True,
            timeout=timeout_seconds,
        )
        return result.stdout.strip() if result.returncode == 0 else "unknown"

    def _http_healthy(self, url: str, timeout_seconds: float) -> bool:
        try:
            with urllib.request.urlopen(url, timeout=timeout_seconds) as response:
                return 200 <= response.status < 300
        except (urllib.error.URLError, TimeoutError):
            return False

    @staticmethod
    def _remaining_timeout(deadline: float) -> float:
        """返回当前探测总预算中的剩余秒数。

        @param deadline: `time.monotonic()` 时间轴上的截止点。
        @return: 可传给子命令或 HTTP 客户端的正数超时。
        """
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            raise TimeoutError("节点状态探测超过总时间预算")
        return remaining
