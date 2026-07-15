# 任务摘要 - 同步 main 0.1.156 到 build

## 目标

- 将固定的 `origin/main=d515c304` 合并到 `build=4f683e95`，吸收 main
  0.1.156 的修复与新能力，同时保留 build 已验证且 main 未等价覆盖的协议、
  账号、额度和运维契约。

## 范围

- 实施前重新 fetch 并校验固定引用、merge base、提交数和 13 个冲突文件；创建
  `backup/build-before-main-0156-<短SHA>`。
- 使用 `git merge --no-commit --no-ff origin/main`，逐文件解决 13 个内容冲突，
  并反向检查 Git 未报告的重复符号、构造参数和自动合并行为冲突。
- 以 main 新 `chatcompletions_anthropic_bridge.go` 为唯一状态机，迁移 build 的
  typed content、attribution 过滤、tool_result 排序、确定性 ID、thinking、空流、
  强制上游 SSE 和本地非流式折叠契约，再删除旧重复实现。
- 保留 Grok 独立 `/billing-quota` 作为账号列表唯一自动 Billing 来源；同时吸收
  main 的手动 quota、OAuth pool refresh 和 reconcile。
- 合并 Codex Responses Lite 的 hosted fallback 与 main 的 namespace/function
  归一化，覆盖无工具、可执行 namespace、扁平 function、嵌套 function 和 WS。
- 合并 AccountHandler/GrokOAuthHandler/Wire 依赖，保留 Agent Identity、Codex
  reset、Grok 独立 Billing 与 reconciler；重新生成 `wire_gen.go`。
- 前端同时保留账号复制、Codex reset 和 Spark shadow 三类账户操作。
- 运行协议、Grok、Codex、handler、前端定向测试及后端/前端完整质量门槛。

## 非目标

- 不修改 Grok 独立套餐额度的产品边界。
- 不新增图片工具运行时、额度模型或数据库 schema。
- 不重构无关模块，不部署、不执行 migration、不自动发布镜像。
- 不从任务启动授权推断 merge commit 或 push；两者必须通过 `trellis-push`
  另行确认。

## 关键上下文

- 当前共同祖先为 `7c717365`；main 相对共同祖先修改 253 个文件。
- `git merge-tree` 报告 13 个内容冲突；隔离编译还确认新旧
  Chat->Anthropic 状态机重复声明。
- 旧 0.1.155 合并任务曾删除独立 Grok Billing，但后续 build 已重新建立双链路；
  用户明确确认本次继续保留独立 `/billing-quota`。
- `ProvideAccountUsageService` 只吸收 main 的 Agent Identity WS 注入，不得重新
  注入 `GrokQuotaService`。
- 合并后的 `NewAccountHandler` 需要同时接收 Grok OAuth 与 Codex reset；
  `NewGrokOAuthHandler` 需要同时接收独立 Billing 与 reconciler。所有调用点必须
  按参数语义更新。
- `wire_gen.go` 只接受 provider 源解析后的生成结果，不把手工冲突拼接作为最终
  方案。
- 任一 fetch 引用、冲突数或文件集合变化都必须停止实施并刷新规划。

## 验收标准

- 固定 main 已进入本地 build，备份分支有效，索引无冲突且没有冲突标记。
- 13 个冲突和高风险自动合并文件均有可追溯处理结论。
- 新旧桥接重复声明消失，build 稳定性与强制 SSE 契约在 main 新桥中通过测试。
- Grok 账号列表不自动调用 main Billing probe；独立 Billing UI/API 与 main
  手动 quota/reconcile 同时可用。
- Lite 标头不泄漏；无客户端图片工具时 hosted fallback 生效；namespace、flat、
  nested function 不重复注入且保留 tool_choice。
- failed/incomplete/completed 三类流终态正确；账号复制、Codex reset、Spark
  shadow 同时可用。
- Handler、provider 和 Wire 构造链编译通过；后端 unit、前端 test/typecheck/
  lint/build、生成一致性与 Git 检查通过。
- 未经单独确认不创建 merge commit、不 push、不部署、不运行 migration。

## 下一步

- 用户确认本 brief 后运行 `task.py start`；任务进入 `in_progress` 后先调用
  `trellis-route(implement)`，不得直接编辑或执行合并。
