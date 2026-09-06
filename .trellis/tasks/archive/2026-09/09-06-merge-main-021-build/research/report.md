# main 0.2.1 → build 合并预检

本报告基于 Git 假想合并树与双方源代码比对；没有实际合并、修改源码或执行测试。

- build：`4e9829519707f0743a7cbcabcf3e73b8dd0beeb0`（0.2.0）
- origin/main：`ab99d56e9626e6cd731592dae8553c9758a0efa2`（0.2.1）
- 共同基线：`b1748c4ea99ce2120401a269142aa071e18a84da`
- 假想合并树：`862c23a64297381a7c57dbd5e7fdd1d4e0c99415`
- 上游待合入 77 个提交，涉及 265 个文件；双方相对共同基线都改过的文件有 63 个。
- 文本冲突：12 个文件、17 个冲突块；全部是内容冲突。

## 文本冲突与处理建议

| 文件 | 冲突块 | 内容与建议 |
| --- | ---: | --- |
| `backend/internal/pkg/openai/constants.go` | 1 | 双方均增加 GPT-6/Astra，创建时间和插入位置不同；统一模型条目，避免重复 ID。 |
| `backend/internal/service/gateway_request.go` | 1 | build 已将 GLM 归一化抽到 openai_provider_reasoning_effort.go，上游仍修改旧位置。将 GLM-5.3 low 保留规则及 Anthropic thinking 适配整合到实际领域实现，避免重复定义，也不能让 build 的 provider 入口继续走旧行为。 |
| `backend/internal/service/gateway_request_test.go` | 1 | GLM 测试在 build 已迁移，上游追加 GLM-5.3 low 案例。保留新案例并迁入领域测试。 |
| `backend/internal/service/openai_codex_models_service.go` | 5 | GPT-6 配置存在实质差异：build context_window=272000、max_context_window=872000；上游两者均为 1050000。模型识别表达式及 gpt-6 的 Responses Lite 限制列表也重叠。用户已明确 GPT-6 按 main：模型识别、上下文、能力元数据、GPT-6 提示词选择和模型目录 Lite 标记均以 main 为准；build 的通用可配置 Lite 策略独立保留，并同步相关断言。 |
| `backend/internal/service/openai_codex_transform.go` | 1 | 双方新增 Astra，build 还在模型表中显式保留 gpt-6 别名。统一别名来源并验证归一化行为，避免重复或遗漏。 |
| `backend/internal/service/openai_codex_transform_test.go` | 1 | 同一测试表位置分别新增 GPT-5.6 max 和 GPT-6 别名案例；保留双方有效案例。 |
| `backend/internal/service/openai_gateway_messages_chat_fallback.go` | 1 | build 将非流式响应处理改为消费上游 SSE 并折叠为 JSON，上游在原 JSON 实现中新增 UpstreamHeaders。保留 build 直连桥接、强制上游流式和自定义请求头，将响应头记录补入实际返回路径。 |
| `backend/internal/service/openai_gateway_request_body.go` | 1 | GPT-5.6 与 GPT-6 判断的逻辑或顺序不同；这处属于等价表达式冲突。 |
| `frontend/src/components/account/CreateAccountModal.vue` | 1 | 同一位置分别新增 build 的 JSON Schema 降级状态和上游生图 URL 转 base64 状态；两项均应保留，并验证初始化、提交和重置。 |
| `frontend/src/components/account/EditAccountModal.vue` | 2 | 状态声明及 extra 保存逻辑重叠；保留 build 的兼容设置/Web Search 桥接 helper，同时加入上游 images_url_to_b64_json 的保存和清理。 |
| `frontend/src/i18n/locales/en/admin/accounts.ts` | 1 | build 领域文案展开与上游生图 base64 文案占据同一插入点；保留双方并检查最终英文聚合值。 |
| `frontend/src/i18n/locales/zh/admin/accounts.ts` | 1 | 同英文对应位置；保留双方并检查中英文最终 key 与文案。 |

## Git 未标记的冲突

`backend/internal/service/openai_model_alias.go` 被 Git 自动合并，但结果包含两个 `isOpenAIGPT6AstraModel` 函数，会导致 Go 重复定义编译错误。build 的版本约束允许已知后缀并排除 `gpt-6-astra-custom`；上游版本接受任意 `gpt-6-astra-` 前缀。用户已明确 GPT-6 按 main，因此采用 main 的单一定义；旧 build GPT-6 断言随上游行为更新，不保留旧边界。

## 合并后的重点验证

- GPT-6 默认模型去重、别名、上下文、max/ultra effort、提示词及 Responses Lite 策略。
- GLM-5.3 low / Anthropic thinking 新行为与 build 现有 GLM/Grok provider 行为。
- Messages → Chat Completions 的流式/非流式、响应头记录、自定义请求头和用量。
- 新建/编辑账号的兼容设置和 images_url_to_b64_json 可共存，保存/清理/重开正确。
- 中英文最终 locale key、覆盖顺序及可见文案；运行规范要求的三个 locale 测试。
- 按合入影响执行后端编译/相关测试、前端类型检查/相关测试。上游还包含数据库迁移文件，本次未执行任何数据库操作。

## 范围限制

这是冲突预检，尚未对 63 个重叠文件完成全量行为审查，也没有编译或测试合并结果；不能据此声称全部兼容。

原始 Git 输出：`merge-tree.txt`。双方重叠文件完整清单：`overlap.txt`。
