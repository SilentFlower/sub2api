"""节点应用门禁执行器。"""

from __future__ import annotations

import subprocess
import os
import signal
import time
from typing import Callable, Protocol

from .config import ActionCommand, AgentConfig
from .model import ControlState


class Executor(Protocol):
    """节点可变更动作接口。"""

    def stop_app(self) -> None:
        """停止本节点容灾应用。

        @return: 无。
        """
        ...

    def start_app(self) -> None:
        """启动本节点容灾应用。

        @return: 无。
        """
        ...

    def run_action(
        self,
        action: ActionCommand,
        control: ControlState,
        authorization_heartbeat: Callable[[], None],
    ) -> None:
        """执行状态机动作并持续验证租约授权。

        @param action: 受控动作配置。
        @param control: 动作开始时的权威状态。
        @param authorization_heartbeat: 周期性授权复核函数。
        @return: 无。
        """
        ...

    def ensure_restart_policy(self) -> None:
        """在线收敛应用 restart policy。

        @return: 无。
        """
        ...


class SystemExecutor:
    """通过显式命令执行应用启动和停止。"""

    def __init__(self, config: AgentConfig) -> None:
        """创建系统执行器。

        @param config: 节点配置。
        @return: 无。
        """
        self._config = config

    def stop_app(self) -> None:
        """停止本节点容灾应用。

        @return: 无。
        """
        self._run(self._config.stop_app_command, "停止应用失败")

    def start_app(self) -> None:
        """启动本节点容灾应用。

        @return: 无。
        """
        self._run(self._config.start_app_command, "启动应用失败")

    def run_action(
        self,
        action: ActionCommand,
        control: ControlState,
        authorization_heartbeat: Callable[[], None],
    ) -> None:
        """执行状态机动作并持续验证租约授权。

        @param action: 受控动作配置。
        @param control: 动作开始时的权威状态。
        @param authorization_heartbeat: 周期性授权复核函数。
        @return: 无。
        """
        environment = os.environ.copy()
        environment.update(
            {
                "HA_AUTOMATION": "true",
                "HA_EPOCH": str(control.epoch),
                "HA_TRANSITION_ID": control.transition_id,
                "HA_EXPECTED_STATE": control.state,
                "HA_EXPECTED_OWNER": control.owner,
            }
        )
        process = subprocess.Popen(
            action.command,
            stdin=subprocess.PIPE,
            text=True,
            env=environment,
            start_new_session=True,
        )
        if process.stdin is not None:
            process.stdin.write(action.stdin)
            process.stdin.close()
        deadline = time.monotonic() + action.timeout_seconds
        try:
            while process.poll() is None:
                remaining = deadline - time.monotonic()
                if remaining <= 0:
                    raise TimeoutError(f"受控动作超过 {action.timeout_seconds} 秒")
                try:
                    process.wait(timeout=min(self._config.interval_seconds, remaining))
                except subprocess.TimeoutExpired:
                    authorization_heartbeat()
        except Exception:
            self._terminate_process_group(process)
            raise
        if process.returncode != 0:
            raise RuntimeError(f"受控动作失败，退出码={process.returncode}")

    def ensure_restart_policy(self) -> None:
        """在线收敛应用 restart policy。

        @return: 无。
        """
        self._run(
            self._config.ensure_restart_policy_command, "收敛应用 restart policy 失败"
        )

    def _run(self, command: tuple[str, ...], message: str) -> None:
        result = subprocess.run(
            command,
            check=False,
            timeout=self._config.command_timeout_seconds,
        )
        if result.returncode != 0:
            raise RuntimeError(f"{message}，退出码={result.returncode}")

    @staticmethod
    def _terminate_process_group(process: subprocess.Popen[str]) -> None:
        if process.poll() is not None:
            return
        try:
            os.killpg(process.pid, signal.SIGTERM)
            process.wait(timeout=5)
        except (ProcessLookupError, subprocess.TimeoutExpired):
            if process.poll() is None:
                os.killpg(process.pid, signal.SIGKILL)
                process.wait(timeout=5)
