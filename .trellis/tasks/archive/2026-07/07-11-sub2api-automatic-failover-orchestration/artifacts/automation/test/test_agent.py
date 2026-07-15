"""HA agent 单元测试。"""

from __future__ import annotations

import subprocess
import tempfile
import time
import unittest
from dataclasses import replace
from datetime import datetime, timedelta, timezone
from pathlib import Path
from typing import Callable
from unittest.mock import call, patch

from sub2api_ha.agent import HaAgent
from sub2api_ha.alerts import AlertQueue
from sub2api_ha.checkpoint import write_checkpoint
from sub2api_ha.client import (
    USER_AGENT,
    AdminControlClient,
    ControlPlaneClient,
    ControlPlaneError,
)
from sub2api_ha.cli import _agent_process_lock, verify_action_state
from sub2api_ha.config import ActionCommand, AgentConfig, ConfigError
from sub2api_ha.executor import SystemExecutor
from sub2api_ha.model import Checkpoint, ControlState, LocalState
from sub2api_ha.probe import SystemProbe, parse_machine_state, restart_policy_is_safe


NOW = datetime(2026, 7, 11, 10, 0, tzinfo=timezone.utc)


class FakeClient:
    """可编程控制面客户端。"""

    def __init__(self, state: ControlState, fail_alert: bool = False) -> None:
        self.state = state
        self.renewed: list[int] = []
        self.reported: list[dict[str, object]] = []
        self.acquired: list[tuple[int, bool, str]] = []
        self.alerts: list[dict[str, object]] = []
        self.fail_alert = fail_alert

    def status(self) -> ControlState:
        """返回预设状态。"""
        return self.state

    def report(self, payload: dict[str, object]) -> ControlState:
        """记录上报并返回预设状态。"""
        self.reported.append(payload)
        return self.state

    def alert(self, payload: dict[str, object]) -> ControlState:
        """记录节点告警。"""
        if self.fail_alert:
            raise ControlPlaneError("测试控制面不可达")
        self.alerts.append(payload)
        return self.state

    def renew(self, epoch: int) -> ControlState:
        """记录续租。"""
        self.renewed.append(epoch)
        self.state = replace(self.state, lease_until=NOW + timedelta(days=36_500))
        return self.state

    def acquire_for_b(
        self, expected_epoch: int, eligible: bool, transition_id: str
    ) -> tuple[ControlState, bool]:
        """记录 B 接管请求。"""
        self.acquired.append((expected_epoch, eligible, transition_id))
        simulated = self.state.mode == "observe"
        return self.state, simulated

    def advance(
        self,
        epoch: int,
        expected_state: str,
        next_state: str,
        transition_step: str,
        reason: str,
    ) -> ControlState:
        """推进测试状态。"""
        self.state = replace(
            self.state,
            state=next_state,
            transition_step=transition_step,
            transition_step_at=NOW,
        )
        return self.state

    def checkpoint(
        self, epoch: int, expected_state: str, transition_step: str
    ) -> ControlState:
        """更新测试 checkpoint。"""
        self.state = replace(
            self.state, transition_step=transition_step, transition_step_at=NOW
        )
        return self.state

    def handoff_ready(self, epoch: int) -> ControlState:
        """标记 A 已追平。"""
        self.state = replace(
            self.state, transition_step="a-ready", transition_step_at=NOW
        )
        return self.state

    def commit_handoff(self, epoch: int, transition_id: str) -> ControlState:
        """提交测试 handoff。"""
        self.state = replace(
            self.state,
            owner="A",
            epoch=epoch + 1,
            state="A_PROMOTING",
            transition_id=transition_id,
            transition_step="lease-transferred",
            transition_step_at=NOW,
        )
        return self.state

    def switch_entry(self, target: str, epoch: int, request_id: str) -> ControlState:
        """切换测试入口。"""
        self.state = replace(
            self.state,
            entry_tunnel=target,
            transition_step="entry-switched",
            transition_step_at=NOW,
        )
        return self.state


class UnreachableClient(FakeClient):
    """模拟 Worker 完全不可达的控制面客户端。"""

    def status(self) -> ControlState:
        """模拟状态请求失败。

        @return: 不返回，始终抛出 ControlPlaneError。
        """
        raise ControlPlaneError("测试控制面不可达")


class FakeHttpResponse:
    """为 urllib 客户端测试提供上下文管理响应。"""

    def __init__(self, payload: bytes) -> None:
        """创建固定响应。

        @param payload: read 返回的响应体。
        @return: 无。
        """
        self._payload = payload

    def __enter__(self) -> "FakeHttpResponse":
        """进入响应上下文。

        @return: 当前响应对象。
        """
        return self

    def __exit__(self, *_args: object) -> None:
        """退出响应上下文。

        @param _args: 上下文管理器异常参数。
        @return: 无。
        """

    def read(self) -> bytes:
        """返回固定响应体。

        @return: JSON 响应字节。
        """
        return self._payload


class FakeProbe:
    """返回固定本地状态。"""

    def __init__(
        self,
        state: LocalState,
        public_healthy: bool = True,
        detailed_error: Exception | None = None,
    ) -> None:
        """创建测试探测器。

        @param state: 本地节点状态。
        @param public_healthy: 公共入口是否健康。
        @param detailed_error: 详细探测时抛出的可选异常。
        """
        self.state = state
        self.public_healthy = public_healthy
        self.detailed_error = detailed_error
        self.lease_collect_count = 0
        self.detailed_collect_count = 0

    def collect_lease_state(self) -> LocalState:
        """返回租约关键状态。"""
        self.lease_collect_count += 1
        return self.state

    def collect(self) -> LocalState:
        """返回固定状态。"""
        self.detailed_collect_count += 1
        if self.detailed_error is not None:
            raise self.detailed_error
        return self.state

    def public_entry_healthy(self) -> bool:
        """返回预设公共入口健康状态。"""
        return self.public_healthy


class FakeExecutor:
    """记录应用门禁动作。"""

    def __init__(self, fail_action: bool = False) -> None:
        """创建测试执行器。

        @param fail_action: 是否让受控动作主动失败。
        """
        self.stop_count = 0
        self.start_count = 0
        self.action_count = 0
        self.restart_policy_count = 0
        self.fail_action = fail_action

    def stop_app(self) -> None:
        """记录停止动作。"""
        self.stop_count += 1

    def start_app(self) -> None:
        """记录启动动作。"""
        self.start_count += 1

    def run_action(
        self,
        action: ActionCommand,
        control: ControlState,
        authorization_heartbeat: Callable[[], None],
    ) -> None:
        """记录受控动作。"""
        self.action_count += 1
        if self.fail_action:
            raise RuntimeError("测试动作失败")

    def ensure_restart_policy(self) -> None:
        """记录 restart policy 收敛。"""
        self.restart_policy_count += 1


def config(
    node: str,
    checkpoint: Path,
    action_states: tuple[str, ...] = (),
    restart_policy_states: tuple[str, ...] = (),
) -> AgentConfig:
    """创建测试配置。"""
    actions = {
        state: {
            "command": ["true"],
            "stdin": "",
            "timeout_seconds": 30,
            "enforce_restart_policy": state in restart_policy_states,
        }
        for state in action_states
    }
    return AgentConfig.from_mapping(
        {
            "node": node,
            "control_url": "https://ha.example.com",
            "secret_file": "/tmp/not-used",
            "state_command": ["true"],
            "app_container": "app",
            "stop_app_command": ["true"],
            "start_app_command": ["true"],
            "ensure_restart_policy_command": ["true"],
            "app_health_url": "http://127.0.0.1/health",
            "public_health_url": "https://api.example.com/health",
            "tunnel_service": "cloudflared-ha.service",
            "expected_restart_policy": "no",
            "actions": actions,
            "checkpoint_file": str(checkpoint),
            "pending_alert_file": str(checkpoint.with_name("pending-alerts.json")),
        }
    )


def control(
    owner: str,
    mode: str,
    expired: bool = False,
    state: str | None = None,
    transition_step: str = "topology-restored",
    transition_step_at: datetime = NOW,
) -> ControlState:
    """创建测试权威状态。"""
    return ControlState(
        owner=owner,
        epoch=7,
        lease_until=NOW + timedelta(seconds=-1)
        if expired
        else NOW + timedelta(days=36_500),
        state=state or ("A_ACTIVE" if owner == "A" else "B_ACTIVE"),
        mode=mode,
        transition_id="transition-1",
        transition_step=transition_step,
        transition_step_at=transition_step_at,
        stable_since=None,
        entry_tunnel=owner,
    )


def active_local(node: str, mode: str, restart_policy_safe: bool = True) -> LocalState:
    """创建满足镜像门禁的活动节点状态。

    @param node: A 或 B。
    @param mode: 活动容灾模式。
    @param restart_policy_safe: restart policy 是否满足门禁。
    @return: 测试用活动节点状态。
    """
    fields = {"mode": mode}
    if node == "A":
        fields["image_sync"] = "ok"
    else:
        fields.update(
            {
                "app_image_digest": "repo@sha256:abc",
                "app_image_cached": "yes",
                "release_image_digest": "repo@sha256:abc",
            }
        )
    return LocalState(node, fields, True, True, True, restart_policy_safe)


class AgentTest(unittest.TestCase):
    """HA agent 行为测试。"""

    def test_parse_machine_state_rejects_duplicate_key(self) -> None:
        """重复机器字段必须失败。"""
        with self.assertRaisesRegex(ValueError, "字段重复"):
            parse_machine_state("mode=standby\nmode=active\n")

    def test_absent_app_container_is_restart_policy_safe(self) -> None:
        """重建期应用容器不存在时不应阻塞稳定窗口。"""
        self.assertTrue(restart_policy_is_safe("absent", "unknown", "no"))
        self.assertTrue(restart_policy_is_safe("running", "no", "no"))
        self.assertFalse(restart_policy_is_safe("running", "unless-stopped", "no"))

    def test_config_requires_https(self) -> None:
        """控制面必须使用 HTTPS。"""
        with self.assertRaisesRegex(ConfigError, "https"):
            AgentConfig.from_mapping(
                {
                    "node": "A",
                    "control_url": "http://example.com",
                    "secret_file": "/tmp/secret",
                    "state_command": ["true"],
                    "app_container": "app",
                    "stop_app_command": ["true"],
                    "start_app_command": ["true"],
                    "ensure_restart_policy_command": ["true"],
                    "app_health_url": "http://127.0.0.1/health",
                    "public_health_url": "https://api.example.com/health",
                    "tunnel_service": "tunnel.service",
                    "expected_restart_policy": "no",
                }
            )

    def test_config_defaults_to_ten_second_interval(self) -> None:
        """未显式配置时应使用十秒心跳间隔。"""
        agent_config = config("A", Path("/tmp/sub2api-ha-test-checkpoint.json"))

        self.assertEqual(agent_config.interval_seconds, 10)
        self.assertEqual(agent_config.request_timeout_seconds, 4)
        self.assertEqual(agent_config.lease_probe_timeout_seconds, 5)
        self.assertEqual(agent_config.detailed_probe_timeout_seconds, 20)
        self.assertEqual(agent_config.lease_state_command, agent_config.state_command)

    def test_config_rejects_lease_probe_timeout_not_less_than_interval(self) -> None:
        """租约关键探测预算不得耗尽整个心跳周期。"""
        with self.assertRaisesRegex(ConfigError, "lease_probe_timeout_seconds"):
            AgentConfig.from_mapping(
                {
                    "node": "A",
                    "control_url": "https://ha.example.com",
                    "secret_file": "/tmp/not-used",
                    "state_command": ["true"],
                    "app_container": "app",
                    "stop_app_command": ["true"],
                    "start_app_command": ["true"],
                    "ensure_restart_policy_command": ["true"],
                    "app_health_url": "http://127.0.0.1/health",
                    "public_health_url": "https://api.example.com/health",
                    "tunnel_service": "cloudflared-ha.service",
                    "expected_restart_policy": "no",
                    "interval_seconds": 10,
                    "lease_probe_timeout_seconds": 10,
                }
            )

    def test_system_probe_uses_independent_lease_command_and_budget(self) -> None:
        """租约关键探测必须使用独立命令和更短的总预算。"""
        agent_config = replace(
            config("A", Path("/tmp/sub2api-ha-test-checkpoint.json")),
            lease_state_command=("lease-status",),
            state_command=("detailed-status",),
            lease_probe_timeout_seconds=5,
            detailed_probe_timeout_seconds=20,
        )
        local = active_local("A", "legacy-active")
        probe = SystemProbe(agent_config)

        with patch.object(probe, "_collect_state", return_value=local) as collect_state:
            self.assertIs(probe.collect_lease_state(), local)
            self.assertIs(probe.collect(), local)

        self.assertEqual(
            collect_state.call_args_list,
            [call(("lease-status",), 5), call(("detailed-status",), 20)],
        )

    def test_system_probe_shares_one_total_timeout_budget(self) -> None:
        """状态脚本、systemd、Docker 和 HTTP 必须共享同一探测预算。"""
        agent_config = replace(
            config("A", Path("/tmp/sub2api-ha-test-checkpoint.json")),
            lease_state_command=("lease-status",),
            lease_probe_timeout_seconds=5,
        )
        probe = SystemProbe(agent_config)
        status = subprocess.CompletedProcess(
            ("lease-status",),
            0,
            stdout="mode=legacy-active\napp_container=running\n",
            stderr="",
        )
        active = subprocess.CompletedProcess(("systemctl",), 0)
        restart_policy = subprocess.CompletedProcess(
            ("docker",), 0, stdout="no\n", stderr=""
        )

        with (
            patch(
                "sub2api_ha.probe.time.monotonic",
                side_effect=[100.0, 101.0, 102.0, 103.0, 104.0],
            ),
            patch(
                "sub2api_ha.probe.subprocess.run",
                side_effect=[status, active, restart_policy],
            ) as run,
            patch.object(probe, "_http_healthy", return_value=True) as http_healthy,
        ):
            local = probe.collect_lease_state()

        self.assertTrue(local.owner_healthy(control("A", "automatic")))
        self.assertEqual(
            [item.kwargs["timeout"] for item in run.call_args_list], [4, 3, 2]
        )
        http_healthy.assert_called_once_with("http://127.0.0.1/health", 1)

    def test_process_lock_rejects_second_agent(self) -> None:
        """同一节点不得并发运行两个 HA agent 循环。"""
        with tempfile.TemporaryDirectory() as directory:
            lock_file = Path(directory) / "agent.lock"
            with _agent_process_lock(lock_file):
                with self.assertRaisesRegex(ConfigError, "已有 HA agent"):
                    with _agent_process_lock(lock_file):
                        self.fail("第二个进程锁不应成功")

    def test_action_restart_policy_flag_requires_boolean(self) -> None:
        """动作策略开关必须是明确布尔值。"""
        with tempfile.TemporaryDirectory() as directory:
            raw = {
                "node": "A",
                "control_url": "https://ha.example.com",
                "secret_file": "/tmp/not-used",
                "state_command": ["true"],
                "app_container": "app",
                "stop_app_command": ["true"],
                "start_app_command": ["true"],
                "ensure_restart_policy_command": ["true"],
                "app_health_url": "http://127.0.0.1/health",
                "public_health_url": "https://api.example.com/health",
                "tunnel_service": "cloudflared-ha.service",
                "expected_restart_policy": "no",
                "actions": {
                    "A_REBUILDING": {
                        "command": ["true"],
                        "timeout_seconds": 30,
                        "enforce_restart_policy": "yes",
                    }
                },
                "checkpoint_file": str(Path(directory) / "checkpoint.json"),
            }

            with self.assertRaisesRegex(ConfigError, "布尔值"):
                AgentConfig.from_mapping(raw)

    def test_admin_token_file_must_be_private(self) -> None:
        """管理员 Token 不得允许组或其它用户读取。"""
        with tempfile.TemporaryDirectory() as directory:
            token_file = Path(directory) / "admin-token"
            token_file.write_text("a" * 32, encoding="utf-8")
            token_file.chmod(0o644)

            with self.assertRaisesRegex(ControlPlaneError, "0600"):
                AdminControlClient.from_token_file(
                    "https://ha.example.com", token_file, 4
                )

            token_file.chmod(0o600)
            client = AdminControlClient.from_token_file(
                "https://ha.example.com", token_file, 4
            )
            self.assertIsInstance(client, AdminControlClient)

    def test_cloudflare_clients_send_stable_user_agent(self) -> None:
        """节点和管理员请求必须避开 Python urllib 默认指纹。"""
        response = FakeHttpResponse(b'{"ok":true,"data":{}}')
        with patch(
            "sub2api_ha.client.urllib.request.urlopen", return_value=response
        ) as urlopen:
            node_client = ControlPlaneClient("https://ha.example.com", "A", "a" * 32, 4)
            node_client._request("GET", "/v1/status", {})
            node_request = urlopen.call_args.args[0]
            self.assertEqual(node_request.get_header("User-agent"), USER_AGENT)

            admin_client = AdminControlClient("https://ha.example.com", "b" * 32, 4)
            admin_client._request("/v1/control/mode", {"mode": "observe"})
            admin_request = urlopen.call_args.args[0]
            self.assertEqual(admin_request.get_header("User-agent"), USER_AGENT)

    def test_verify_action_state_rejects_stale_epoch(self) -> None:
        """shell 自动化动作必须再次验证实时 epoch。"""
        current = control(
            "B", "automatic", state="B_PROMOTING", transition_step="lease-acquired"
        )
        verify_action_state(current, 7, "B", "B_PROMOTING", "transition-1", NOW)

        with self.assertRaisesRegex(ConfigError, "epoch"):
            verify_action_state(current, 6, "B", "B_PROMOTING", "transition-1", NOW)

    def test_system_executor_stops_action_when_lease_heartbeat_fails(self) -> None:
        """长动作授权失效时必须终止整个子进程组。"""
        with tempfile.TemporaryDirectory() as directory:
            agent_config = replace(
                config("B", Path(directory) / "checkpoint.json"),
                interval_seconds=1,
            )
            executor = SystemExecutor(agent_config)
            action = ActionCommand(
                command=("sh", "-c", "sleep 10"),
                stdin="",
                timeout_seconds=30,
                enforce_restart_policy=False,
            )
            started = time.monotonic()

            with self.assertRaisesRegex(RuntimeError, "租约失效"):
                executor.run_action(
                    action,
                    control("B", "automatic", state="B_PROMOTING"),
                    lambda: (_ for _ in ()).throw(RuntimeError("租约失效")),
                )

            self.assertLess(time.monotonic() - started, 5)

    def test_owner_uses_single_report_heartbeat_when_healthy(self) -> None:
        """健康 owner 的稳态循环不应额外调用 renew。"""
        with tempfile.TemporaryDirectory() as directory:
            local = LocalState(
                node="A",
                fields={"mode": "legacy-active", "image_sync": "ok"},
                app_running=True,
                app_healthy=True,
                tunnel_healthy=True,
                restart_policy_safe=True,
            )
            client = FakeClient(control("A", "automatic"))
            agent = HaAgent(
                config("A", Path(directory) / "checkpoint.json"),
                client,
                FakeProbe(local),
                FakeExecutor(),
            )

            agent.run_once(NOW)

            self.assertEqual(len(client.reported), 1)
            self.assertEqual(client.renewed, [])

    def test_a_active_heartbeat_skips_cross_node_detailed_probe(self) -> None:
        """A 稳态续租不得执行可能阻塞的跨节点详细探测。"""
        with tempfile.TemporaryDirectory() as directory:
            local = active_local("A", "legacy-active")
            probe = FakeProbe(local, detailed_error=TimeoutError("B SSH 超时"))
            client = FakeClient(control("A", "automatic"))
            executor = FakeExecutor()
            agent_config = replace(
                config("A", Path(directory) / "checkpoint.json"),
                lease_state_command=("lease-status",),
                state_command=("detailed-status",),
            )
            agent = HaAgent(
                agent_config,
                client,
                probe,
                executor,
            )

            agent.run_once(NOW)

            self.assertEqual(probe.lease_collect_count, 1)
            self.assertEqual(probe.detailed_collect_count, 0)
            self.assertEqual(len(client.reported), 1)
            self.assertEqual(executor.stop_count, 0)

    def test_matching_state_commands_reuse_lease_probe_result(self) -> None:
        """B 的租约命令已是完整状态时不得重复执行详细探测。"""
        with tempfile.TemporaryDirectory() as directory:
            local = LocalState("B", {"mode": "standby"}, False, False, True, True)
            probe = FakeProbe(local)
            agent = HaAgent(
                config("B", Path(directory) / "checkpoint.json"),
                FakeClient(control("A", "observe")),
                probe,
                FakeExecutor(),
            )

            agent.run_once(NOW)

            self.assertEqual(probe.lease_collect_count, 1)
            self.assertEqual(probe.detailed_collect_count, 0)

    def test_detailed_probe_timeout_after_heartbeat_skips_orchestration(self) -> None:
        """非稳态详细探测超时后应保留心跳并跳过破坏性编排。"""
        with tempfile.TemporaryDirectory() as directory:
            checkpoint = Path(directory) / "checkpoint.json"
            local = LocalState("A", {"mode": "offline"}, False, False, True, True)
            probe = FakeProbe(local, detailed_error=TimeoutError("B SSH 超时"))
            client = FakeClient(control("B", "automatic", state="B_ACTIVE"))
            executor = FakeExecutor()
            agent_config = replace(
                config("A", checkpoint, action_states=("A_REBUILDING",)),
                lease_state_command=("lease-status",),
                state_command=("detailed-status",),
            )
            agent = HaAgent(
                agent_config,
                client,
                probe,
                executor,
            )

            with self.assertLogs("sub2api-ha-agent", level="WARNING") as logs:
                result = agent.run_once(NOW)

            self.assertIsNotNone(result)
            self.assertEqual(len(client.reported), 1)
            self.assertEqual(probe.detailed_collect_count, 1)
            self.assertEqual(executor.action_count, 0)
            self.assertIn("本轮租约心跳已完成", "\n".join(logs.output))

    def test_a_owner_stays_healthy_during_release_image_drift(self) -> None:
        """B 发布镜像未同步时，健康 A 仍应保持 owner 资格。"""
        with tempfile.TemporaryDirectory() as directory:
            local = active_local("A", "legacy-active")
            local = replace(
                local, fields={"mode": "legacy-active", "image_sync": "drift"}
            )
            client = FakeClient(control("A", "automatic"))
            executor = FakeExecutor()
            agent = HaAgent(
                config("A", Path(directory) / "checkpoint.json"),
                client,
                FakeProbe(local),
                executor,
            )

            agent.run_once(NOW)

            self.assertTrue(local.owner_healthy(client.state))
            self.assertEqual(executor.stop_count, 0)
            self.assertEqual(len(client.reported), 1)

    def test_b_owner_requires_release_image_readiness(self) -> None:
        """B 活动节点仍必须满足固定镜像与发布记录一致。"""
        local = LocalState(
            "B",
            {
                "mode": "active",
                "app_image_digest": "repo@sha256:new",
                "app_image_cached": "yes",
                "release_image_digest": "repo@sha256:old",
            },
            True,
            True,
            True,
            True,
        )

        self.assertFalse(local.owner_healthy(control("B", "automatic")))

    def test_restart_policy_drift_fences_before_alerting(self) -> None:
        """活动应用重启策略漂移时必须停止应用且不再续租。"""
        with tempfile.TemporaryDirectory() as directory:
            local = active_local("A", "legacy-active", restart_policy_safe=False)
            client = FakeClient(control("A", "automatic"))
            executor = FakeExecutor()
            agent = HaAgent(
                config("A", Path(directory) / "checkpoint.json"),
                client,
                FakeProbe(local),
                executor,
            )

            agent.run_once(NOW)

            self.assertEqual(executor.restart_policy_count, 1)
            self.assertEqual(executor.stop_count, 1)
            self.assertEqual(client.renewed, [])
            self.assertEqual(len(client.alerts), 1)

    def test_expired_owner_report_cannot_continue_orchestration(self) -> None:
        """合并心跳未续租时 owner 必须立即 fail-closed。"""
        with tempfile.TemporaryDirectory() as directory:
            checkpoint = Path(directory) / "checkpoint.json"
            agent_config = config("A", checkpoint)
            local = active_local("A", "legacy-active")
            executor = FakeExecutor()
            agent = HaAgent(
                agent_config,
                FakeClient(control("A", "automatic", expired=True)),
                FakeProbe(local),
                executor,
            )

            agent.run_once(NOW)

            self.assertEqual(executor.stop_count, 1)
            self.assertEqual(
                len(AlertQueue(agent_config.pending_alert_file).pending()), 1
            )

    def test_restart_uses_expired_checkpoint_when_worker_is_unreachable(self) -> None:
        """Agent 重启后必须用过期 automatic checkpoint 执行 fail-closed。"""
        with tempfile.TemporaryDirectory() as directory:
            checkpoint_path = Path(directory) / "checkpoint.json"
            agent_config = config("A", checkpoint_path)
            expired = control("A", "automatic", expired=True)
            write_checkpoint(checkpoint_path, Checkpoint.from_control(expired, NOW))
            executor = FakeExecutor()
            agent = HaAgent(
                agent_config,
                UnreachableClient(expired),
                FakeProbe(active_local("A", "legacy-active")),
                executor,
            )

            result = agent.run_once(NOW)

            self.assertIsNone(result)
            self.assertEqual(executor.stop_count, 1)
            self.assertEqual(
                len(AlertQueue(agent_config.pending_alert_file).pending()), 1
            )

    def test_expired_cached_lease_fences_without_detailed_probe(self) -> None:
        """租约过期后的 fail-closed 不得再等待跨节点详细探测。"""
        with tempfile.TemporaryDirectory() as directory:
            checkpoint_path = Path(directory) / "checkpoint.json"
            agent_config = config("A", checkpoint_path)
            expired = control("A", "automatic", expired=True)
            write_checkpoint(checkpoint_path, Checkpoint.from_control(expired, NOW))
            probe = FakeProbe(
                active_local("A", "legacy-active"),
                detailed_error=TimeoutError("B SSH 超时"),
            )
            executor = FakeExecutor()
            agent = HaAgent(agent_config, FakeClient(expired), probe, executor)

            agent._fence_if_cached_lease_expired(NOW)

            self.assertEqual(executor.stop_count, 1)
            self.assertEqual(probe.detailed_collect_count, 0)

    def test_observe_mode_does_not_stop_non_owner_app(self) -> None:
        """观察模式只记录风险，不停止应用。"""
        with tempfile.TemporaryDirectory() as directory:
            local = active_local("A", "legacy-active")
            executor = FakeExecutor()
            client = FakeClient(control("B", "observe"))
            agent = HaAgent(
                config("A", Path(directory) / "checkpoint.json"),
                client,
                FakeProbe(local),
                executor,
            )

            agent.run_once(NOW)

            self.assertEqual(executor.stop_count, 0)
            self.assertEqual(client.alerts, [])

    def test_automatic_mode_stops_non_owner_app(self) -> None:
        """自动模式下非 owner 应用必须停止。"""
        with tempfile.TemporaryDirectory() as directory:
            local = active_local("A", "legacy-active")
            executor = FakeExecutor()
            agent = HaAgent(
                config("A", Path(directory) / "checkpoint.json"),
                FakeClient(control("B", "automatic", state="PAUSED_NEEDS_OPERATOR")),
                FakeProbe(local),
                executor,
            )

            agent.run_once(NOW)

            self.assertEqual(executor.stop_count, 1)

    def test_worker_outage_alert_is_queued_and_flushed(self) -> None:
        """Worker 不可达期间的关键告警应在恢复后补发。"""
        with tempfile.TemporaryDirectory() as directory:
            checkpoint = Path(directory) / "checkpoint.json"
            agent_config = config("A", checkpoint)
            local_running = active_local("A", "legacy-active")
            failed_client = FakeClient(
                control("B", "automatic", state="PAUSED_NEEDS_OPERATOR"),
                fail_alert=True,
            )
            executor = FakeExecutor()
            failed_agent = HaAgent(
                agent_config, failed_client, FakeProbe(local_running), executor
            )

            failed_agent.run_once(NOW)

            queue = AlertQueue(agent_config.pending_alert_file)
            self.assertEqual(len(queue.pending()), 1)
            recovered_client = FakeClient(
                control("B", "automatic", state="PAUSED_NEEDS_OPERATOR")
            )
            recovered_agent = HaAgent(
                agent_config,
                recovered_client,
                FakeProbe(
                    LocalState("A", {"mode": "offline"}, False, False, True, True)
                ),
                FakeExecutor(),
            )

            recovered_agent.run_once(NOW)

            self.assertEqual(queue.pending(), [])
            self.assertEqual(len(recovered_client.alerts), 1)

    def test_b_requests_acquire_only_when_eligible(self) -> None:
        """B 只有完整 standby 门禁通过时才申请接管。"""
        with tempfile.TemporaryDirectory() as directory:
            fields = {
                "mode": "standby",
                "postgres_recovery": "t",
                "postgres_streaming": "streaming",
                "ntp_synchronized": "yes",
                "redis_role": "slave",
                "redis_link": "up",
                "redis_sync": "0",
                "app_container": "absent",
                "app_image_cached": "yes",
                "app_image_digest": "repo@sha256:abc",
                "release_image_digest": "repo@sha256:abc",
            }
            local = LocalState("B", fields, False, False, True, True)
            client = FakeClient(control("A", "automatic", expired=True))
            agent = HaAgent(
                config("B", Path(directory) / "checkpoint.json"),
                client,
                FakeProbe(local),
                FakeExecutor(),
            )

            agent.run_once(NOW)

            self.assertEqual(len(client.acquired), 1)
            self.assertTrue(client.acquired[0][1])

    def test_b_does_not_acquire_when_wal_receiver_is_not_streaming(self) -> None:
        """PostgreSQL 断流时 B 不得先取得租约再失败。"""
        with tempfile.TemporaryDirectory() as directory:
            fields = {
                "mode": "standby",
                "postgres_recovery": "t",
                "postgres_streaming": "stopped",
                "ntp_synchronized": "yes",
                "redis_role": "slave",
                "redis_link": "up",
                "redis_sync": "0",
                "app_container": "absent",
                "app_image_cached": "yes",
                "app_image_digest": "repo@sha256:abc",
                "release_image_digest": "repo@sha256:abc",
            }
            local = LocalState("B", fields, False, False, True, True)
            client = FakeClient(control("A", "automatic", expired=True))
            agent = HaAgent(
                config("B", Path(directory) / "checkpoint.json"),
                client,
                FakeProbe(local),
                FakeExecutor(),
            )

            agent.run_once(NOW)

            self.assertEqual(client.acquired, [])
            self.assertEqual(len(client.alerts), 1)

    def test_b_promoting_runs_action_switches_entry_and_becomes_active(self) -> None:
        """B_PROMOTING 完成后必须先切入口再进入 B_ACTIVE。"""
        with tempfile.TemporaryDirectory() as directory:
            standby = LocalState(
                "B",
                {"mode": "standby", "app_container": "absent"},
                False,
                False,
                True,
                True,
            )
            initial = control(
                "B", "automatic", state="B_PROMOTING", transition_step="lease-acquired"
            )
            client = FakeClient(initial)
            executor = FakeExecutor()
            probe = FakeProbe(standby)
            agent = HaAgent(
                config(
                    "B",
                    Path(directory) / "checkpoint.json",
                    ("B_PROMOTING",),
                    ("B_PROMOTING",),
                ),
                client,
                probe,
                executor,
            )

            first = agent.run_once(NOW)
            self.assertEqual(first.transition_step, "service-ready")

            probe.state = active_local("B", "active")
            second = agent.run_once(NOW)
            self.assertEqual(second.transition_step, "entry-switched")

            result = agent.run_once(NOW)

            self.assertEqual(executor.action_count, 1)
            self.assertEqual(executor.restart_policy_count, 1)
            self.assertEqual(result.state, "B_ACTIVE")
            self.assertEqual(result.entry_tunnel, "B")

    def test_active_owner_reconciles_entry_tunnel_drift(self) -> None:
        """活动 owner 发现入口漂移时应按权威租约纠正。"""
        with tempfile.TemporaryDirectory() as directory:
            local = active_local("B", "active")
            initial = replace(
                control("B", "automatic", state="B_ACTIVE"), entry_tunnel="A"
            )
            client = FakeClient(initial)
            agent = HaAgent(
                config("B", Path(directory) / "checkpoint.json"),
                client,
                FakeProbe(local),
                FakeExecutor(),
            )

            switched = agent.run_once(NOW)
            self.assertEqual(switched.transition_step, "entry-switched")

            result = agent.run_once(NOW)

            self.assertEqual(result.entry_tunnel, "B")
            self.assertEqual(result.transition_step, "entry-reconciled")

    def test_entry_switched_restart_resumes_public_health_verification(self) -> None:
        """入口已切换后的 agent 重启不得重复执行数据库提升。"""
        with tempfile.TemporaryDirectory() as directory:
            local = active_local("B", "active")
            initial = replace(
                control(
                    "B",
                    "automatic",
                    state="B_PROMOTING",
                    transition_step="entry-switched",
                ),
                entry_tunnel="B",
            )
            client = FakeClient(initial)
            executor = FakeExecutor()
            agent = HaAgent(
                config("B", Path(directory) / "checkpoint.json", ("B_PROMOTING",)),
                client,
                FakeProbe(local),
                executor,
            )

            result = agent.run_once(NOW)

            self.assertEqual(result.state, "B_ACTIVE")
            self.assertEqual(result.transition_step, "entry-verified")
            self.assertEqual(executor.action_count, 0)

    def test_public_health_timeout_pauses_promotion(self) -> None:
        """公共入口持续不健康时不得把提升状态标记为 ACTIVE。"""
        with tempfile.TemporaryDirectory() as directory:
            local = active_local("B", "active")
            initial = replace(
                control(
                    "B",
                    "automatic",
                    state="B_PROMOTING",
                    transition_step="entry-switched",
                    transition_step_at=NOW - timedelta(seconds=91),
                ),
                entry_tunnel="B",
            )
            client = FakeClient(initial)
            agent = HaAgent(
                config("B", Path(directory) / "checkpoint.json", ("B_PROMOTING",)),
                client,
                FakeProbe(local, public_healthy=False),
                FakeExecutor(),
            )

            result = agent.run_once(NOW)

            self.assertEqual(result.state, "PAUSED_NEEDS_OPERATOR")
            self.assertEqual(result.transition_step, "public-health-timeout")

    def test_a_recovery_advances_to_failback_wait(self) -> None:
        """B_ACTIVE 时 A 应执行重建并进入稳定观察。"""
        with tempfile.TemporaryDirectory() as directory:
            local = LocalState("A", {"mode": "offline"}, False, False, True, True)
            client = FakeClient(control("B", "automatic", state="B_ACTIVE"))
            executor = FakeExecutor()
            agent = HaAgent(
                config("A", Path(directory) / "checkpoint.json", ("A_REBUILDING",)),
                client,
                FakeProbe(local),
                executor,
            )

            result = agent.run_once(NOW)

            self.assertEqual(executor.action_count, 1)
            self.assertEqual(executor.restart_policy_count, 0)
            self.assertEqual(result.state, "FAILBACK_WAIT")

    def test_action_failure_advances_to_operator_pause(self) -> None:
        """受控动作失败后必须阻止下一轮自动重试。"""
        with tempfile.TemporaryDirectory() as directory:
            local = LocalState("A", {"mode": "offline"}, False, False, True, True)
            client = FakeClient(control("B", "automatic", state="A_REBUILDING"))
            executor = FakeExecutor(fail_action=True)
            agent = HaAgent(
                config("A", Path(directory) / "checkpoint.json", ("A_REBUILDING",)),
                client,
                FakeProbe(local),
                executor,
            )

            with self.assertRaisesRegex(RuntimeError, "测试动作失败"):
                agent.run_once(NOW)

            self.assertEqual(client.state.state, "PAUSED_NEEDS_OPERATOR")
            self.assertEqual(client.state.transition_step, "a_rebuilding-failed")

    def test_failback_wait_requires_stable_window_and_maintenance_window(self) -> None:
        """稳定时间和维护窗口同时满足才请求冻结 B。"""
        with tempfile.TemporaryDirectory() as directory:
            local = LocalState(
                "A", {"mode": "standby-from-b"}, False, False, True, True
            )
            initial = replace(
                control("B", "automatic", state="FAILBACK_WAIT"),
                stable_since=NOW - timedelta(minutes=31),
            )
            client = FakeClient(initial)
            test_now = datetime(2026, 7, 11, 20, 30, tzinfo=timezone.utc)
            agent = HaAgent(
                config("A", Path(directory) / "checkpoint.json"),
                client,
                FakeProbe(local),
                FakeExecutor(),
            )

            result = agent.run_once(test_now)

            self.assertEqual(result.state, "B_FREEZING")

    def test_full_handoff_sequence_restores_a_primary_b_standby(self) -> None:
        """完整回切必须经过 B 冻结、A 就绪、租约转交和 B 重建。"""
        with tempfile.TemporaryDirectory() as directory:
            checkpoint = Path(directory) / "checkpoint.json"
            shared = FakeClient(
                control(
                    "B",
                    "automatic",
                    state="B_FREEZING",
                    transition_step="freeze-requested",
                )
            )
            b_executor = FakeExecutor()
            b_probe = FakeProbe(active_local("B", "active"))
            b_agent = HaAgent(
                config("B", checkpoint, ("B_FREEZING",)),
                shared,
                b_probe,
                b_executor,
            )
            a_executor = FakeExecutor()
            a_probe = FakeProbe(
                LocalState("A", {"mode": "standby-from-b"}, False, False, True, True)
            )
            a_agent = HaAgent(
                config(
                    "A",
                    checkpoint,
                    ("B_FREEZING", "A_PROMOTING", "B_REBUILDING"),
                    ("A_PROMOTING",),
                ),
                shared,
                a_probe,
                a_executor,
            )

            b_agent.run_once(NOW)
            self.assertEqual(shared.state.transition_step, "b-frozen")

            a_agent.run_once(NOW)
            self.assertEqual(shared.state.transition_step, "a-ready")

            b_agent.run_once(NOW)
            self.assertEqual(shared.state.owner, "A")
            self.assertEqual(shared.state.state, "A_PROMOTING")

            a_agent.run_once(NOW)
            self.assertEqual(shared.state.transition_step, "service-ready")

            a_probe.state = active_local("A", "active-recovered")
            a_agent.run_once(NOW)
            self.assertEqual(shared.state.transition_step, "entry-switched")

            a_agent.run_once(NOW)
            self.assertEqual(shared.state.state, "B_REBUILDING")
            self.assertEqual(shared.state.entry_tunnel, "A")

            result = a_agent.run_once(NOW)
            self.assertEqual(result.state, "A_ACTIVE")
            self.assertEqual(result.transition_step, "topology-restored")
            self.assertEqual(b_executor.action_count, 1)
            self.assertEqual(a_executor.action_count, 3)
            self.assertEqual(a_executor.restart_policy_count, 1)


if __name__ == "__main__":
    unittest.main()
