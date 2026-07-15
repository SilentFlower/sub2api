"""HA agent 命令行入口。"""

from __future__ import annotations

import argparse
import fcntl
import json
import logging
import os
import sys
import uuid
from contextlib import contextmanager
from datetime import datetime, timezone
from pathlib import Path
from typing import Iterator

from .agent import HaAgent
from .client import AdminControlClient, ControlPlaneError
from .config import AgentConfig, ConfigError
from .executor import SystemExecutor
from .model import ControlState
from .probe import SystemProbe


@contextmanager
def _agent_process_lock(path: Path) -> Iterator[None]:
    path.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    descriptor = os.open(path, os.O_CREAT | os.O_RDWR, 0o600)
    try:
        os.fchmod(descriptor, 0o600)
        try:
            fcntl.flock(descriptor, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError as exc:
            raise ConfigError(f"已有 HA agent 正在运行：{path}") from exc
        yield
    finally:
        os.close(descriptor)


def build_parser() -> argparse.ArgumentParser:
    """创建命令行解析器。

    @return: argparse 解析器。
    """
    parser = argparse.ArgumentParser(description="Sub2API 自动容灾 agent")
    parser.add_argument(
        "command",
        choices=(
            "run",
            "once",
            "status",
            "verify-action",
            "observe",
            "automatic",
            "pause",
            "resume",
            "emergency-freeze",
        ),
        help="运行 agent、读取本地状态或执行管理员控制",
    )
    parser.add_argument("--config", required=True, help="agent JSON 配置文件")
    parser.add_argument(
        "--admin-token-file", help="权限为 0600 的 Worker 管理员 Token 文件"
    )
    parser.add_argument("--reason", help="人工控制原因或确认依据")
    parser.add_argument(
        "--expected-epoch", type=int, help="resume 前读取到的当前 epoch"
    )
    parser.add_argument(
        "--owner", choices=("A", "B"), help="resume 时已确认的唯一 owner"
    )
    parser.add_argument("--expected-state", help="verify-action 要求的权威状态")
    parser.add_argument("--transition-id", help="verify-action 要求的迁移 ID")
    parser.add_argument(
        "--resume-state",
        choices=("A_ACTIVE", "B_ACTIVE", "FAILBACK_WAIT"),
        help="resume 时与真实拓扑一致的权威状态",
    )
    parser.add_argument(
        "--resume-mode",
        default="observe",
        choices=("observe", "automatic"),
        help="resume 后的模式，默认 observe",
    )
    parser.add_argument(
        "--log-level", default="INFO", choices=("DEBUG", "INFO", "WARNING", "ERROR")
    )
    return parser


def print_control_state(state: ControlState) -> None:
    """输出机器可读的权威控制状态。

    @param state: Worker 返回的权威状态。
    @return: 无。
    """
    print(
        json.dumps(
            {
                "owner": state.owner,
                "epoch": state.epoch,
                "leaseUntil": state.lease_until.isoformat(),
                "state": state.state,
                "mode": state.mode,
                "transitionId": state.transition_id,
                "transitionStep": state.transition_step,
                "transitionStepAt": state.transition_step_at.isoformat(),
                "stableSince": state.stable_since.isoformat()
                if state.stable_since
                else None,
                "entryTunnel": state.entry_tunnel,
            },
            ensure_ascii=False,
            sort_keys=True,
        )
    )


def required_argument(value: object, name: str) -> object:
    """读取管理员命令必填参数。

    @param value: argparse 解析后的值。
    @param name: 参数名称。
    @return: 非空参数值。
    """
    if value is None or value == "":
        raise ConfigError(f"命令缺少必填参数：{name}")
    return value


def verify_action_state(
    control: ControlState,
    expected_epoch: int,
    expected_owner: str,
    expected_state: str,
    transition_id: str,
    now: datetime,
) -> None:
    """验证自动化 shell 动作仍匹配权威租约。

    @param control: Worker 返回的当前权威状态。
    @param expected_epoch: Agent 发起动作时的 epoch。
    @param expected_owner: Agent 发起动作时的 owner。
    @param expected_state: Agent 发起动作时的状态。
    @param transition_id: Agent 发起动作时的迁移 ID。
    @param now: 当前 UTC 时间。
    @return: 无；不匹配时抛出 ConfigError。
    """
    if control.mode != "automatic":
        raise ConfigError(f"控制面模式不是 automatic：{control.mode}")
    if control.owner != expected_owner:
        raise ConfigError(f"控制面 owner={control.owner}，预期 {expected_owner}")
    if control.epoch != expected_epoch:
        raise ConfigError(f"控制面 epoch={control.epoch}，预期 {expected_epoch}")
    if control.state != expected_state:
        raise ConfigError(f"控制面 state={control.state}，预期 {expected_state}")
    if control.transition_id != transition_id:
        raise ConfigError("控制面 transition_id 与自动化动作不一致")
    if control.expired(now):
        raise ConfigError("控制面租约已经到期")


def main(argv: list[str] | None = None) -> int:
    """执行 HA agent 命令。

    @param argv: 可选命令行参数。
    @return: 进程退出码。
    """
    args = build_parser().parse_args(argv)
    logging.basicConfig(
        level=getattr(logging, args.log_level),
        format="%(asctime)s %(levelname)s %(name)s %(message)s",
    )
    try:
        config = AgentConfig.from_file(args.config)
        if args.command == "verify-action":
            expected_epoch = int(
                required_argument(args.expected_epoch, "--expected-epoch")
            )
            if expected_epoch < 0:
                raise ConfigError("--expected-epoch 必须是非负整数")
            control = HaAgent.create_control_client(config).status()
            expected_owner = str(required_argument(args.owner, "--owner"))
            expected_state = str(
                required_argument(args.expected_state, "--expected-state")
            )
            transition_id = str(
                required_argument(args.transition_id, "--transition-id")
            )
            verify_action_state(
                control,
                expected_epoch,
                expected_owner,
                expected_state,
                transition_id,
                datetime.now(timezone.utc),
            )
            print_control_state(control)
            return 0
        probe = SystemProbe(config)
        if args.command == "status":
            state = probe.collect()
            print(
                json.dumps(state.report_payload(0), ensure_ascii=False, sort_keys=True)
            )
            return 0
        if args.command in {
            "observe",
            "automatic",
            "pause",
            "resume",
            "emergency-freeze",
        }:
            token_file = str(
                required_argument(args.admin_token_file, "--admin-token-file")
            )
            admin = AdminControlClient.from_token_file(
                config.control_url,
                token_file,
                config.request_timeout_seconds,
            )
            if args.command in {"observe", "automatic"}:
                control = admin.set_mode(args.command)
            elif args.command == "pause":
                control = admin.pause(str(required_argument(args.reason, "--reason")))
            elif args.command == "emergency-freeze":
                control = admin.emergency_freeze(
                    str(required_argument(args.reason, "--reason"))
                )
            else:
                expected_epoch = int(
                    required_argument(args.expected_epoch, "--expected-epoch")
                )
                if expected_epoch < 0:
                    raise ConfigError("--expected-epoch 必须是非负整数")
                control = admin.resume(
                    expected_epoch,
                    str(required_argument(args.owner, "--owner")),
                    str(required_argument(args.resume_state, "--resume-state")),
                    args.resume_mode,
                    f"operator-resume-{uuid.uuid4()}",
                    str(required_argument(args.reason, "--reason")),
                )
            print_control_state(control)
            return 0
        with _agent_process_lock(config.lock_file):
            agent = HaAgent.create_system(config, probe, SystemExecutor(config))
            if args.command == "once":
                return 0 if agent.run_once() is not None else 2
            agent.run_forever()
            return 0
    except (ConfigError, ControlPlaneError, RuntimeError, ValueError) as exc:
        logging.getLogger("sub2api-ha-agent").error("启动失败：%s", exc)
        return 1


if __name__ == "__main__":
    sys.exit(main())
