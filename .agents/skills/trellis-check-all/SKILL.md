---
name: trellis-check-all
description: "统一 Check-All：按行为影响选择 light/full，审查规划、实现假设与完整性；区分 CHK、FBK、DOC，按需加载报告和自修规则。触发：检查、轻量/全面/提交前检查、check-all。"
---
# Check All 统一入口

本 skill 是 **薄入口**：做对了 → 假设成立 → 做全了且写得规范。不要提前读取未命中的 profile 或低频细则。

## 默认流程

1. 读 [depth-routing.md](references/depth-routing.md)，确认实际 diff、事项、规范和运行上下文，生成 `check_profile`；它拥有 requested/effective depth 与 light/full 路由。
2. `effective_depth=light` 读 [light-profile.md](references/light-profile.md)，full 读 [full-profile.md](references/full-profile.md)。两个 profile 共用 [verification.md](references/verification.md)，复用当前有效验证证据；范围无法闭合时单向升级 full。
3. 按 profile 审查三个维度：task 的 PRD/可选 design/implement（untracked 为 N/A）、实现假设、完整性与项目规范。同一追踪与验证复用，保留独立验收判断。
4. 发生下表场景时先读对应细则；完成检查与允许的 DOC 处理后，读 [reporting-and-disposition.md](references/reporting-and-disposition.md) 输出并分流。

## 按条件加载

| 场景 | 先读取 |
| --- | --- |
| 有问题候选需要分类或确定严重度 | [fallback-findings.md](references/fallback-findings.md)；完整模型与处置见 [findings-and-reporting.md](references/findings-and-reporting.md) |
| 发现低风险事实漂移候选，或准备写入 DOC | [document-drift-auto-remediation.md](references/document-drift-auto-remediation.md)；仅源码注释候选再读其专项 |
| 当前 action 已验证来自 auto-loop | [auto-loop-return.md](references/auto-loop-return.md) |

引用缺失时报告阻塞，不凭记忆重建。未命中 DOC 候选不读取自修细则；任何自修之前必须读取并满足白名单。full、findings、风险接受和修复报告由 reporting 的条件入口所有。

## 核心边界

- 默认 **audit-only collect-all**：读取、搜索、运行无业务写入的验证；不直接修复普通代码、配置、测试或任务规格语义。低风险事实漂移进入 `DOC-*` 通道，是唯一自修例外，须符合专项证据与写入边界；用户明确绝对只读时也不写 DOC。
- 分类先于严重度：主路径与非兜底契约问题为 `CHK-*`；失败、异常、防御保护路径问题为 `FBK-*`；可唯一证明的事实漂移候选为 `DOC-*`。两类代码问题都按实际影响评 P0/P1/P2；具体准入交给 fallback reference，不报告泛化“更健壮”偏好。
- 同根因合并并列全独立问题；统一报告后只确认一次修复范围或风险接受，`修复全部` 覆盖 CHK/FBK。接受不能从摘要、dirty 状态或无关历史推断。
- 只在业务决策、前提失效或必需验证涉及未授权生产/外部/破坏性副作用时中途暂停；报告已完成范围和必要缺口。`[上线后验证]` 保留且不阻断，不代执行。
- 所有检查都从本入口进入；`trellis-route` 只决定 inline/subagent。inline 主会话可处理合规 DOC；subagent 只读返回 `CHK-*` / `FBK-*` / `DOC-*` 候选、画像和证据，不编辑、不补测试、不选择修复范围、不使用 workspace-write `trellis-check`。
- untracked helper 只存游标：未处置 findings 或新编辑回 `implement`；通过且 disposition 继续才 `advance --stage spec`，明确停止保持 check。它不证明检查或风险接受已完成。
