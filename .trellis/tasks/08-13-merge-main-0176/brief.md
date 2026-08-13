# Brief - 合并 main 0.1.176 到 build

## 目标

将 `main@fbfdcef81`（版本 `0.1.176`）以普通 merge 方式合入
`build@7a6ab3280`，保留确认需要的 build 私有能力，删除 Grok 独立套餐额度
查询与展示链路，并验证合并不会让 build 现有特性静默消失或失效。

## 范围

- 合并基线为 `0b3fe95af`；合并预演结果为 9 个冲突文件、24 个内容冲突块，
  另有 162 个文件可自动合并。
- Grok 冲突以 main 行为为基线，只删除独立 `/billing-quota` API、独立
  Billing parser/service/UsageInfo 投影、前端独立额度组件与请求队列、共享类型、
  locale 和专项测试。
- 保留 main 的 `/quota`、`grok_billing`、`grok_usage_snapshot`、JWT tier、
  Grok 4.6、长上下文、模型级定价和主线账号用量展示。
- 保留 build 的 Grok force-chat、Grok 4.5 provider reasoning 归一化和批量登录
  用户脚本。
- Anthropic 冲突同时保留 main 的 `reasoning_content` 别名支持，以及 build 的
  reasoning-only 回显、流式缓存、合法工具过滤和稳定工具调用 ID。
- Codex 创建、编辑、批量编辑同时保留 build 自定义 User-Agent 与 main
  fingerprint mode；运行时统一身份和指纹元数据不得互相覆盖。
- 对所有 build 私有能力执行 owner、共享入口、DTO/设置/extra、最终行为、专项测试
  五段式回归审计；自动合并成功不视为功能通过。

## 非目标

- 不新增 main 与 build 均不存在的产品能力。
- 不顺带重构与本次分支同步无关的模块。
- 不删除或迁移数据库中历史 `grok_billing_quota_snapshot`；应用不再读取或展示，
  repository 可继续将该旧 key 视为调度中性字段。
- 本任务不会自动提交或推送，完成实现和检查后仍需单独确认。

## 关键决策

- 使用普通 merge，禁止 rebase、squash 或改写 build 历史；实施前创建精确保护引用。
- Grok 的“只保留 main”限定为独立套餐额度链路，不扩大到 force-chat、provider
  reasoning 或批量登录脚本。
- 不对大型共享文件整体选择 ours/theirs；按领域和冲突块处理，避免删除同文件内无关
  的 build 能力。
- Wire 等生成文件从源定义重新生成，不手工拼接生成结果。
- 主体文件仍存在不等于功能仍可用；必须验证共享入口仍调用、配置仍能往返、调用顺序
  没有被 main 后续转换覆盖，并以最终入口测试证明。

## 功能回归重点

- 高风险：Codex 自定义 UA、Codex reset、统一身份与 fingerprint mode、Responses
  Lite、JSON Schema 降级、Web Search/`web.run`/AnySearch、Codex 生图策略、
  Anthropic 直连桥、provider reasoning、Grok force-chat。
- 中风险：Codex Alpha Search、Raw Chat 调试快照、Antigravity GIF 多帧兼容、
  OpenAI 生图设置、Web Search 设置、locale 最终覆盖值。
- 分支资产：手动 GHCR workflow、fork CLA 删除策略、`README_CN.md`、Trellis 与
  HA/DR 资产不能被 main 恢复、覆盖或静默删除。
- 特别检查 main 新增原生 `/x_search` 与 build 搜索模拟链路的分流、Responses Lite
  与图片流 failover 的顺序、Codex identity 与 fingerprint headers 的组合，以及
  Grok 4.5/4.6 不同 reasoning 规则。

## 已知风险

- 目前没有证据表明某项 build 特性必然消失，但存在多处可编译却会语义失效的高风险
  交叉点，尤其是 Codex 表单、Gateway 转换顺序、Web Search、Responses Lite 和
  Anthropic 桥。
- main 与 build 对同一共享入口的修改有一部分会自动合并；若只处理 9 个硬冲突，
  可能留下入口绕过、字段不再往返、locale 覆盖或 Wire 依赖图漂移。
- Grok 独立套餐额度跨后端 route/service/DTO/Wire 和前端 API/type/locale/UI，必须
  显式解除全链路接入，不能只在冲突文件中选择 main。

## 验收标准

- `build` 包含 `main@fbfdcef81`，版本为 `0.1.176`，无未解决冲突和冲突标记。
- Grok 独立套餐额度生产链路和专项展示被删除，main 主线额度展示正常；Grok
  force-chat、4.5 reasoning 归一化和批量登录脚本仍有入口及测试证据。
- Anthropic 桥同时满足 main reasoning alias 与 build 流式/工具稳定性契约。
- Codex 自定义 UA、reset、统一身份、fingerprint mode 在创建、编辑、批量编辑、
  HTTP/WS 出站链路中均保持正确行为。
- 私有能力回归矩阵逐项完成，发现的特性丢失或失效在本任务内修复并补充测试。
- 后端受影响单测与全量 unit gate、前端 lint/typecheck/定向 Vitest/全量测试/build、
  Wire 生成验证和最终双边热点审计全部通过。
- 实现与检查结束后不自动提交、不自动推送。

## 下一步

真实 merge、冲突处理、Grok 独立链路删除、build 私有能力回归审计和全量验证均已完成；
当前保持未提交的 merge 状态，等待后续单独确认提交。
