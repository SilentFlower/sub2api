"""本地 checkpoint 读写。"""

from __future__ import annotations

import json
import os
import tempfile
from pathlib import Path

from .config import ensure_private_directory
from .model import Checkpoint


def read_checkpoint(path: Path) -> Checkpoint | None:
    """读取本地 checkpoint。

    @param path: checkpoint 文件路径。
    @return: 文件不存在时返回 None，否则返回已校验的 checkpoint。
    """
    if not path.exists():
        return None
    try:
        raw = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise RuntimeError(f"无法读取 checkpoint：{path}") from exc
    if not isinstance(raw, dict):
        raise RuntimeError("checkpoint 根节点必须是对象")
    try:
        return Checkpoint.from_mapping(raw)
    except ValueError as exc:
        raise RuntimeError(f"checkpoint 内容无效：{path}") from exc


def write_checkpoint(path: Path, checkpoint: Checkpoint) -> None:
    """原子写入 checkpoint。

    @param path: checkpoint 文件路径。
    @param checkpoint: 待写入数据。
    @return: 无。
    """
    ensure_private_directory(path.parent)
    descriptor, temporary = tempfile.mkstemp(prefix=f".{path.name}.", dir=path.parent)
    try:
        with os.fdopen(descriptor, "w", encoding="utf-8") as handle:
            json.dump(checkpoint.as_dict(), handle, ensure_ascii=False, sort_keys=True)
            handle.write("\n")
            handle.flush()
            os.fsync(handle.fileno())
        os.chmod(temporary, 0o600)
        os.replace(temporary, path)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)
