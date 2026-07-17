# Build 私有功能隔离指南

> **强制规则**：只属于 `build` 分支的业务能力必须优先放入按领域命名的独立文件或前端功能目录；与上游共享的文件只允许保留稳定、薄的装配调用。目标是降低后续 `main -> build` 同步的文本冲突和语义回退风险。

## 何时必须使用

- 新增或修改只在 `build` 存在的配置、协议兼容、管理能力、UI 或测试。
- 修改 `origin/main` 也在频繁变更的 handler、service、gateway、页面、locale 或综合测试。
- 解决 `main -> build` 冲突时需要决定代码最终所有权。
- 已有 build 逻辑仍内嵌在上游共享大文件中。

## 实施顺序

1. 使用 `git log -p`、`git blame`、共同基线和假想合并树确认功能所有权，不能仅凭文件名判断。
2. 为 build 业务语义选择明确领域 owner，例如 `openai_responses_lite_policy.go`、`features/responsesLite/`；禁止使用 `build_helpers.go` 一类模糊聚合文件。
3. 把规则、归一化、payload 投影、专属文案和专项测试放入领域 owner。
4. 上游共享入口只准备现有上下文、调用一次领域 API，并传播返回值或错误。
5. 对共享入口和领域 owner 分别保留测试：入口测试覆盖接线，领域测试覆盖 build 业务分支。
6. 修改后重新检查双方修改文件和 `git merge-tree`；“Git 自动合并成功”不能替代语义复核。
7. 对 locale spread、re-export、装饰器等“后定义覆盖前定义”的接入，必须比较 main 新值、build owner 值和最终运行时有效值；0 个文本冲突不代表上游新语义已经生效。

## 薄接入契约

共享文件允许：

- 保留不可拆分的 struct/DTO/interface 字段。
- 读取入口已经拥有的 account、model、request、form 或 config。
- 调用一个按领域命名的 helper/service/component。
- 传播 `result/error/handled`，或完成一次稳定注册、re-export、对象展开。

共享文件禁止：

- 定义 build 专属模型列表、默认值、通配规则或协议判断表。
- 重复解析、归一化或更新 build 专属 JSON/config 字段。
- 内嵌 build 专属大段 UI、文案或测试 fixture。
- 为减少文件数量把多个无关 build 能力重新聚合到同一 helper。

## 例外矩阵

| 情况 | 处理 |
| --- | --- |
| DTO、route、构造器、ProviderSet、共享类型 | 保留最小中央契约，业务语义迁出 |
| Wire、Ent 等生成文件 | 修改源定义后重新生成，禁止手工塑形 |
| 文件本身已经是单一 build 协议主体 | 保持原文件，不为形式继续拆碎 |
| main 已提供等价实现 | 复用 main owner，不创建 build wrapper 保存历史痕迹 |
| 只有一行稳定注册或 locale 展开 | 可以留在共享文件，但值和规则由领域模块拥有；main 修改被覆盖 key 时，必须把上游新语义合入领域 owner |
| 抽离会制造循环依赖、重复状态源或更复杂回调 | 保留最小中央逻辑，并在任务 design 中记录原因 |

## 示例

### Go

错误：在上游共享 gateway 中维护 build 模型规则。

```go
func (s *OpenAIGatewayService) Forward(...) {
	if strings.HasPrefix(model, "gpt-5") {
		// 大段 build 私有判断和请求改写
	}
}
```

正确：领域文件拥有规则，共享入口只做薄调用。

```go
// openai_responses_lite_policy.go
func applyResponsesLitePolicy(model string, body []byte) ([]byte, error) {
	// build 私有规则
}

// openai_gateway_forward.go
body, err = applyResponsesLitePolicy(model, body)
if err != nil {
	return nil, err
}
```

### Vue / TypeScript

错误：在 `SettingsView.vue` 中直接维护 build 私有校验、文案和 payload 转换。

正确：`src/features/<domain>/` 拥有组件和规则，页面只绑定 form 并处理保存。

```vue
<ResponsesLiteBlockedModelsSettings
  v-model="form.responses_lite_blocked_models"
/>
```

## 检查清单

- [ ] 每个 build 规则都有明确领域 owner，不存在模糊总 helper。
- [ ] 上游共享文件只保留薄调用、注册、re-export 或不可拆分中央字段。
- [ ] build 专属测试没有继续堆入 main 综合测试函数。
- [ ] 前端文案进入领域 locale 扩展，主 locale 只做稳定深层展开。
- [ ] locale spread、re-export 或装饰器覆盖点已核对最终有效值，没有静默遮蔽 main 的新语义。
- [ ] 生成文件来自生成器，ProviderSet/constructor 与生成结果一致。
- [ ] `rg` 搜索确认旧定义、重复规则和旧导入已清理。
- [ ] 双方修改文件完成语义复核，`git merge-tree` 没有新增硬冲突。
