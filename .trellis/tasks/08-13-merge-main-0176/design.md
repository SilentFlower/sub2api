# 合并 main 0.1.176 到 build - 技术设计

## 1. 合并策略

在 `build` 上创建保护引用后执行普通 merge，不 squash、不 rebase、不改写历史。
先得到完整冲突索引，再按领域 owner 解决；禁止对大型共享文件直接整体选择 ours/theirs，
除非 Git 历史证明另一侧差异全部属于明确删除的单一能力。

## 2. 冲突所有权

### 2.1 Grok

- `grok_quota_fetcher_test.go` 采用 main 新增测试与最终主线行为。
- `AccountUsageCell.vue` 及其专项测试以 main Grok 展示为基线，并移除独立 Billing
  组件装配。
- `AccountsView.vue` 采用 main 的 JWT tier、`grok_usage_snapshot`、Grok 4.5 Heavy
  判断，但保留同文件内与 Grok 无关的 build 能力，例如 Codex reset。
- `AccountsView.sparkShadow.spec.ts` 采用 main Grok 场景，再补回非 Grok build 断言。
- 独立套餐额度主体与接入按 research 清单显式删除，Wire 重新生成。

### 2.2 Anthropic 协议桥

main 的 `reasoningText()` 是 reasoning alias 的单一读取入口；build 继续拥有：

- reasoning-only 可见正文回退；
- 流式 reasoning 缓存和 finalize 回显；
- 严格合法的 tool_use 输出；
- 无上游 ID 时的确定性工具 ID；
- Messages/Chat 会话粘性和缓存前缀稳定性。

### 2.3 Codex 表单与运行时

Create/Edit/Bulk 中自定义 UA 与 fingerprint mode 是两个并列字段：

- 自定义 UA 继续受 `codex_cli_only` 父开关约束。
- fingerprint 默认 `session`，默认值不落库，`off/device/full` 显式写入。
- 表单重置、编辑回填、批量 enable flag 和最终 extra payload 分别处理，不能共享一个
  开关或互相删除字段。
- 出站统一 Codex UA/originator/version 先保持 build 契约；main fingerprint 仅决定
  device/session/turn ID 与 client metadata，不能重新引入客户端任意身份。

## 3. 私有能力回归审计

每项 build 能力建立以下链路：

```text
领域 owner -> 共享入口 -> DTO/设置/extra -> 用户可见或上游行为 -> 专项测试
```

若任一环缺失，即使编译通过也视为回归。重点检查调用顺序：

1. 入站限制检测与账号策略。
2. 模型映射和最终 provider/model 解析。
3. JSON Schema、Web Search、Responses Lite、reasoning、生图等 body 转换。
4. Codex fingerprint/identity 和最终 headers。
5. 上游发送、流式 failover、usage/计费记录。

## 4. 数据兼容

- 不迁移或删除账号 `extra` 中历史 `grok_billing_quota_snapshot`。
- 应用不再读取、刷新或返回独立套餐额度字段。
- repository 继续忽略该历史 key 的调度影响，避免旧数据造成无意义的调度缓存变化。
- 其它 build 私有设置和 account extra 必须保持原字段名、默认值和往返行为。

## 5. 生成文件与回滚

- 修改 `wire.go`、handler constructor 或 ProviderSet 后运行 `go generate ./cmd/server`。
- `wire_gen.go` 只接受生成结果，不手工合并依赖图。
- 实施前创建精确 backup ref；出现不可控语义回归时可回到合并前 ref。
- 合并提交前保留完整测试证据和最终 diff 审计；本任务不自动提交或推送。
