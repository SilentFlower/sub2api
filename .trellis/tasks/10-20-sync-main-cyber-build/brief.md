# Brief — main 0.2.17 与 cyber 修复合入 build

## Goal

- 合入最新 main 和指定 cyber 修复，保留双方功能，并通过实际回归验收。

## Scope

- `build@f03c930be` 合入已拉取的 `main@8f2cbb53d`（0.2.17），再移植 [xlplbo/sub2api#1](https://github.com/xlplbo/sub2api/pull/1) 的 11 文件修复增量。
- 处理 13 个文件、19 块文本冲突，并核对全部 77 个共同修改文件和双方独立功能。

## Non-Goals

- 方案与提交计划已确认，代码已提交并推送；生产部署、业务数据库操作及付费供应商调用留待后续交付。
- 不全量合入第三方 fork，也不恢复历史已经移除的定制。

## Technical Overview

- 先合 main，再叠加限定 cyber 补丁；逐块合并双方行为，保留独立功能 owner。

| 冲突文件 | 处理方案 |
| --- | --- |
| `handler/endpoint.go` | 保留中文/Grok 说明，补齐 Gemini 显式缓存端点。 |
| `apicompat/types.go` | 同时保留 main 的拒绝内容字段与 build 的引用字段。 |
| `service/account.go` | 接入新模型退役判断；搜索三态只保留 build 独立 owner。 |
| `service/account_websearch_test.go` | 保留双方 API Key、OAuth 和其他平台测试。 |
| `service/antigravity_gateway_gemini.go` | 先补工具思考签名，再走 build 的 GIF 抽帧和大小校验。 |
| `service/gateway_forward_as_responses.go` | 保留索引映射/引用累积，加入负索引保护。 |
| `service/openai_gateway_chat_completions.go` | 接入统一协议路由，保留 Grok、Schema、搜索和错误回退保护。 |
| `service/openai_gateway_messages.go` | 接入新三协议分流，保留 Grok 强制 Chat 与搜索入口。 |
| `service/openai_gateway_messages_chat_fallback.go` | 接入可配置输出下限，保留直连桥接、推理及 Ollama 上限。 |
| `service/openai_gateway_responses_chat_fallback_test.go` | 保留写失败/搜索回归和 main 原始计时测试。 |
| `service/openai_image_generation_controls_test.go` | 同时保留合法生图桥接与非 OpenAI 禁止注入测试。 |
| `account/EditAccountModal.vue` | 保留 build 兼容字段，接入新平台、缓存和停调范围，统一加载/保存。 |
| `account/__tests__/EditAccountModal.spec.ts` | 完整保留双方测试组和独立 fixture。 |

## Context & Decisions

- main 待合入 326 个提交、648 个文件；cyber 补丁预演没有新增冲突。
- cyber 改为按 API Key + 明确会话 ID 精确屏蔽，移除历史超限推断；无 ID 时由上游审核，保留审计、计费及白名单。
- 已验证独立文件保留、双边定价保留；最终 en/zh 各 8534 个叶子键且集合一致。实际树的冲突已全部解决；最终完整后端 unit、前端检查与构建通过，隔离 integration 和迁移也已验证，详见 research/execution.md。

## Risks

- main 带来 7 个新 SQL 迁移、Go 1.27.2 和依赖更新，必须做隔离升级验证。
- 统一路由、Lite/搜索工具、长连接计费和设置保存是自动合并也可能回归的重点。
- 共享 Key 的网关必须传递稳定且不复用的会话 ID；旧实例仍会执行旧封禁逻辑。

## Acceptance

- 完成逐文件核对与 16 组回归：覆盖 cyber、三协议/新平台、搜索、推理/Schema、Lite/图片、GIF/Gemini、计费/超时、账号配置、多语言、支付认证、迁移及部署资产。
- 定向测试后执行必要的后端/前端全量检查、构建和隔离集成；失败与原 build/main 对照，不删除有效断言。
- 以实际证据报告通过项、原有失败和未覆盖场景；不把“无冲突”当成功能正常。

## Next Step

- 合并、验证、Full Check-All 与规范收尾完成，代码已推送 origin/build；任务记录随独立提交同步。后续由发布方核对镜像并安排生产验收。
