# Brief — 合并 main 并配置 Responses Lite Header 模型策略

## Goal

- 在保留 `build` 私有能力和 `main` 新功能的前提下完成当前 `main -> build` 合并，并新增按最终上游模型控制 Responses Lite Header/WS metadata 透传的系统设置。

## Scope

- 重新审查并解决当前 12 个冲突文件；先前手工移除 marker 的结果不视为已批准，`wire_gen.go` 必须由最终依赖图重新生成。
- 复核冲突周边自动合并链路，特别是 `AccountUsageService` 的 main Grok quota 主动刷新与 build 独立 `GrokBillingQuotaService`，两套能力同时保留。
- 新增 `openai_responses_lite_header_blocked_models: string[]`，持久化为 JSON 数组；缺失键默认 `gpt-5.4`、`gpt-5.4-mini`、`gpt-5.5`，管理员可显式保存空数组。
- 模型规则 trim、稳定去重，支持精确匹配和仅末尾 `*`；更新字段未提供时保留旧值，显式 `[]` 整体覆盖。
- 使用 `SettingService` 60 秒 TTL + singleflight，保存后立即刷新，禁止逐请求查库。
- HTTP managed、HTTP passthrough、WS 直连、WS HTTP bridge 都按每次 attempt/turn 完成映射和归一化后的最终上游模型判断。
- 未命中阻止列表时透传 Lite Header/metadata，并保留 PR #4380 的工具布局和 `reasoning.context=all_turns`。
- 命中时删除 Header/metadata、禁止 bridge 重建 Header，并跳过网关的 `all_turns` 强制补齐；客户端已有 developer message、`input.additional_tools`、`parallel_tool_calls` 和显式 context 保持原样，不做完整 Lite -> 标准 Responses 转换。
- 保留 hosted `image_generation` 与客户端 `image_gen` 的执行域边界，以及 group/账号/compact/Spark/passthrough 门禁。
- 在后台 SettingsView 增加模型规则列表编辑器、API 类型、中英文 i18n 和 Vitest 覆盖。
- 更新协议规范，替代旧的“Lite Header 永不透传”绝对规则。

## Non-Goals

- 不从 OpenAI 远端模型目录自动同步能力列表。
- 不修改 OpenAI Codex 或 `/root/project/CLIProxyAPI`。
- 不托管 Codex 客户端 `image_gen` 运行时，也不保证其二次请求经过 sub2api。
- 不改变独立 Images API、批量图片 API 或与本次冲突/Header 策略无关的业务。
- 不在质量检查和用户确认前创建 merge commit；本任务不自动 push。

## Key Context

- 合并基线：`build=934982ae`，`main/MERGE_HEAD=b960ec19`；回退分支为 `backup/build-before-main-0157-934982ae`。
- 当前仍有 12 个未解决索引项，覆盖 Wire、Grok quota、OpenAI Lite/WS/生图测试和账号创建/编辑弹窗。
- PR #4380 只补 `reasoning.context=all_turns`，没有模型能力判断；`gpt-5.6-terra` 是 allow 回归模型。
- 冲突解决以“双方能力可组合”为默认：Grok `cfg` + 独立 billing quota、WS 终态 + build reasoning helper、main Header 候选透传 + 新模型策略终态过滤、前端多类 extra 基于既有对象逐键更新。
- 风险最高的是最终模型判定时机、WS 会话内模型切换、显式空数组语义、Header 与 body normalizer 同步，以及只清除 `UU` 却漏掉跨文件构造器依赖。
- 完整逐文件冲突矩阵、数据流和回滚设计见 `design.md`；执行顺序和命令见 `implement.md`。

## Acceptance

- 12 个原始冲突都有决策记录，`git ls-files -u` 为空、无 conflict marker，生成的 `wire_gen.go` 与 ProviderSet 一致。
- 设置 API/后台页面可读取、编辑和保存列表；缺失键返回三个默认项，显式空数组保持为空。
- `gpt-5.6-terra` Lite 请求在 HTTP/WS 保留标记、Lite 工具布局和 `all_turns`。
- 最终模型精确或通配符命中时，HTTP Header、WS metadata、WS -> HTTP Header 均不进入上游，且不强制改写 context/body。
- 模型映射、failover 和同一 WS 会话模型切换证明策略按当前最终模型重新判断；Grok 永不收到该 Header。
- 客户端 `image_gen`、hosted 生图 fallback、图片权限/计费、Grok quota/billing、WS 终态、账号 extra 均无回退。
- 后端定向及完整 unit、Wire、lint，前端 Vitest/typecheck/lint/build 和 `git diff --check` 通过；无法运行项有明确记录。

## Next Step

- 用户确认本 brief 和三件套后运行 `task.py start`；随后进入 `trellis-route(implement)`，按 `implement.md` 先审计/解决冲突，再实现设置与四条传输路径，最后执行全范围检查。
