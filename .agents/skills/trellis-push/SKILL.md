---
name: trellis-push
description: "按确认的精确文件范围提交并推送相关仓库，并在普通推送后同步当前任务进度。"
---

# Trellis Push

`trellis-push` 是 Phase 3.4 唯一的代码提交入口。它只负责生成最小计划、精确提交、普通推送，以及触发当前任务进度同步。

## 职责边界

- 普通模式默认 `commit + push`。
- 用户明确要求“只提交不推送”时使用 `commit-only`。
- auto-loop 可调用内部 `commit-only`，但必须传入已经校验过的 exact files 与 commit message；本 skill 只执行该提交。
- 不处理分支合并、上线核对、任务归档、会话日志或自动任务队列状态。
- 不使用 `git add .`、`git add -A`，不要求工作区整体干净，也不提交计划外文件。

## 模式

| 模式 | 确认 | Git 动作 | 进度同步 |
| --- | --- | --- | --- |
| 普通 | 展示最小计划并确认一次 | exact commit，然后 push 当前分支 | 有活动任务时立即同步 |
| 用户 `commit-only` | 展示最小计划并确认一次 | exact local commit | 跳过 |
| auto-loop 内部 `commit-only` | 复用 auto-loop 预授权 | exact local commit | 跳过 |

内部 `commit-only` 不接受临时扩大文件范围、远端推送或其他附加动作。安全条件不满足时返回失败，由调用方决定后续状态。

## Step 1：发现仓库与任务

候选仓库包括：

- 含 `.trellis/` 的父仓根目录。
- `.trellis/config.yaml` 中 package 路径对应的独立 Git root。
- 用户明确指定的候选仓库。

同一个 Git root 只保留一次。位于父仓内部但不是独立 Git root 的 package 变更归父仓处理。

为每个候选仓库生成用户可见名称：优先使用 `.trellis/config.yaml` 中匹配的 package 名；没有配置时使用 Git top-level 目录名。`root`、`parent`、`main repo` 只允许作为输入别名，禁止直接显示在计划或结果中。

活动任务是可选上下文：

```bash
python3 ./.trellis/scripts/task.py current --source || true
python3 ./.trellis/scripts/task_progress.py status --json || true
```

无活动任务时仍可提交相关代码，但不生成任务进度。存在活动任务时，结合 `brief.md`、`implement.md`、当前 diff 与本轮执行范围生成一行语义进度；不得从旧进度推断 Git 动作。

## Step 2：预检与文件归属

对每个候选仓库读取：

```bash
git status --short
git branch --show-current
git rev-parse --abbrev-ref --symbolic-full-name @{u} 2>/dev/null || true
git diff --stat
git diff --name-only
git diff --cached --stat
git diff --cached --name-only
git log --oneline -5
git log @{u}..HEAD --oneline 2>/dev/null || true
```

停止条件：

- detached HEAD、分支不可读、冲突、rebase 或其他未完成的 Git 集成状态。
- 普通推送会携带无法归属本次任务的历史 ahead commits。
- 无法确定 planned file 是否属于当前请求或活动任务。
- 内部 `commit-only` 发现 staged 区非空。

文件分为两组：

- `planned`：本轮明确归属且准备提交的 exact files。
- `retained`：当前存在、但本次明确不提交并保持原状的 dirty paths，包含计划外 untracked、unstaged、staged 文件。clean files 不进入该集合。

`retained` 只是内部集合名。用户可见输出统一写“保留未提交的变更（dirty）”，并逐项标注 `[untracked]`、`[unstaged]`、`[staged]`。unknown ahead、branch/upstream 异常、归属不确定等真正需要处理的事项单独进入“风险”区；普通 retained dirty 不默认视为阻塞。

普通模式允许 `retained` 存在。执行前记录计划外 staged set，提交后确认这些 staged 文件仍保持原状。用户明确要求新增文件时，重新生成计划并确认，不能在执行中静默扩大范围。

## Step 3：展示最小计划

确认前禁止 `git add`、`git commit` 或 `git push`。计划只展示：

```markdown
## Trellis Push 计划

[<PUSH / COMMIT-ONLY>] <N> 个仓库 · <N> 个 commit · <N> 个文件 · 保留未提交 <N> · 风险 <N>
[无活动任务时追加：无活动任务]
顺序：<repo-a> -> <repo-b> [-> task progress]

### 1. <repository-name>

`<commit message>`
分支：`<branch>` -> `<upstream>`
变更：<N> 个文件 · `+<adds> -<deletes>`

计划提交：
- <exact files 或分组摘要>

Push：<执行 / 跳过（commit-only）>

### 保留未提交的变更（dirty，仅数量大于 0 时显示）
- [untracked] <path>
- [unstaged] <path>
- [staged] <path>

### 风险（仅数量大于 0 时显示）
- <unknown ahead / branch-upstream / attribution risk>

任务进度：completed=<...> | partial=<...> | next=<...>
执行：<commit -> push -> progress commit -> progress push>

确认执行请回复 `确认`。可调整：`只提交`、`修改 message`、`展开文件`。
```

展示规则：

- 单仓 `planned` 不超过 8 个文件时完整列出。
- 超过 8 个时按目录归组，最多 12 行；用户要求展开时展示同一 exact set。
- 保留未提交的变更始终逐项标注 Git 状态；真正风险在独立“风险”区逐项展示。
- 无活动任务或 `commit-only` 时省略进度动作。
- 不重复展示检查结果、规范复核、归档或其他阶段信息。

普通多仓只确认一次。用户调整 message、文件或仓库顺序后，更新计划并重新确认。

auto-loop 内部 `commit-only` 仍生成同样的逐仓执行数据用于自检和结果记录，但不再次询问用户；它不得扩展调用方给定的 exact files/message。

## Step 4：精确提交与推送

每个仓库按计划顺序执行。执行前重新检查 planned files、当前分支、upstream、冲突状态和 ahead commits；任一关键条件变化都停止当前执行并重新规划。仅 `retained` 内容变化时保留并在结果中更新说明。

精确提交：

```bash
git add -- <exact planned files>
git commit --only -m "<confirmed message>" -- <exact planned files>
```

提交后验证：

```bash
git show --name-only --format= HEAD
git diff --cached --name-only
```

commit 只能包含 planned files，执行前的计划外 staged set 必须仍保留。

普通模式继续推送当前分支：

```bash
git push origin <current-branch>
```

已有 upstream 且远端名称不是 `origin` 时，使用实际 upstream remote。无 upstream 时只能在计划中明确将当前分支设置到选定 remote；不能猜测目标分支。

`commit-only` 到本地提交成功即结束，不推送，也不写远端任务进度。

多仓执行失败时停止后续未开始仓库，保留已经成功的提交/推送，不做回滚。

## Step 5：同步任务进度

仅普通模式且存在活动任务时执行。全部业务仓库成功后写完整进度；已有仓库成功而后续仓库失败时写 partial 进度，明确 completed、失败位置、next 和 notes。尚未发生成功 Git 动作就失败时，不记录虚假的 completed steps；只有父仓仍可安全提交并推送时才允许记录 failure notes。

新进度固定为：

```json
{
  "updatedAt": "<ISO 8601>",
  "completedSteps": ["<已完成步骤>"],
  "partialStep": "<部分完成步骤或 null>",
  "nextStep": "<下一步>",
  "notes": "<可选说明；无说明时为空字符串>"
}
```

进度不得保存本轮模式、业务 commit hash 或提交计划。

写入前确认：

- 当前任务 `task.json` 在业务提交结束后没有计划外 dirty 内容。
- 父仓分支、upstream 和冲突状态安全。
- 推送不会携带无法归属的历史 ahead commits。

通过 helper 写入：

```bash
python3 ./.trellis/scripts/task_progress.py write \
  --task <task-dir> \
  --progress-json '<progress-json>' \
  --json
```

然后只提交并推送当前任务 `task.json`：

```bash
git add -- <task-dir>/task.json
git commit --only -m "chore(task): update <task-name> progress" -- <task-dir>/task.json
git push origin <current-branch>
```

该动作属于用户已确认的普通 push 计划，不增加第二次确认。提交后必须验证 commit 只包含该 `task.json`。如果写入、提交或推送失败，不回滚已成功的业务 Git 动作，并单独报告进度同步失败。

## Step 6：结果

结果复用计划的视觉顺序，先给总览，再逐仓报告实际 commit/push，最后报告任务进度与保留 dirty：

```markdown
## Trellis Push 结果

[完成 / 部分完成 / 失败] <N> 个仓库 · <N> 个业务 commit

### 1. <repository-name>

`<short-hash> <actual commit message>`
分支：`<branch>` -> `<upstream>`
状态：<✓ 已推送 / · 仅本地提交 / ❌ 失败>

### 任务进度

状态：<✓ 已同步 · `<progress-hash>` / · 已跳过 / ❌ 同步失败>
进度：completed=<...> | partial=<...> | next=<...>
[失败时追加：原因和恢复动作]

### 保留未提交的变更（dirty，仅存在时显示）
- [untracked] <path>
- [unstaged] <path>
- [staged] <path>
```

部分完成时必须明确列出已成功仓库、失败仓库/步骤、当前分支和下一恢复动作。业务结果与 progress sync 状态不得合并成一个模糊结论。

## 禁止事项

- 扩大到计划外文件或要求清理无关工作区。
- 用任务进度决定是否推送代码。
- 在本 skill 内执行上线、归档、会话日志或分支合并。
- 自动解决 push rejection、冲突、凭证或远端保护规则问题。
- 在业务失败后伪造已完成进度，或因进度同步失败回滚业务提交。
