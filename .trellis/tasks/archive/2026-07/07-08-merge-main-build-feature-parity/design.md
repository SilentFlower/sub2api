# Design: 合并 main 到 build 并保留 build 功能

## 合并策略

本任务以 `origin/main` 的拆分后结构为目标形态，在当前 `build` 分支执行 merge。冲突解决时优先保留 `main` 的文件拆分、命名和新增功能，再把 `build` 的业务增量迁移到对应新文件中。

不要使用“整文件选 ours/build”的方式解决后端大文件冲突。`build` 的旧大文件包含真实功能，但如果直接保留会撤销 `main` 的大文件拆分，后续继续同步 main 会持续高冲突。

## Anthropic Messages 到 Chat Completions 桥接

`main` 的 `forwardAnthropicViaRawChatCompletions` 解决的是 APIKey 上游不支持 Responses API 时 `/v1/messages` 的可用性 fallback。它原始实现通过 Anthropic -> Responses -> Chat 两段转换完成，并复用 CC pipeline。

`build` 的 `forwardMessagesViaRawChatCompletions` 直接做 Anthropic -> Chat 转换，配套新增 `backend/internal/pkg/apicompat/anthropic_chatcompletions.go`。该方案对缓存稳定更强，因为它显式控制 Chat payload 形态：

- system/user/assistant content 固定 typed content part array；
- 动态 attribution system block 不进入上游；
- tool result 顺序按 tool call 顺序稳定化；
- 缺失 tool call id 时使用确定性 fallback；
- thinking disabled 与 reasoning effort 互斥。

合并后的设计使用 `main` 的函数名 `forwardAnthropicViaRawChatCompletions`，但实现语义采用 `build` 的直连桥接，并继续接入 `main` 的错误处理、failover、日志和 shared CC pipeline 中仍适用的 helper。

## 设置与服务拆分迁移

`main` 已将 settings 和 gateway 大文件拆分。迁移位置如下：

- OpenAI 生图主模型 / reasoning effort：
  - settings view 字段保留在 `backend/internal/service/settings_view.go`；
  - setting key 保留在 `backend/internal/service/domain_constants.go`；
  - 读取缓存逻辑迁移到 `backend/internal/service/setting_features.go` 或同类 feature 设置文件；
  - 更新写入逻辑迁移到 `backend/internal/service/setting_update.go`；
  - parse 回显逻辑迁移到 `backend/internal/service/setting_parse.go`；
  - OpenAI 生图调用点保持在 `backend/internal/service/openai_images_responses.go` / gateway 相关文件。
- Codex custom UA allowlist：
  - 后端 policy / detector 保留在 `backend/internal/pkg/openai/` 与 `backend/internal/service/openai_client_restriction_detector.go`；
  - 前端读写 helper 保留 `frontend/src/components/account/codexClientAllowlist.ts`；
  - 创建/编辑账号弹窗写入 account extra。
- Codex reset：
  - repository/client/service 保留 build 增量；
  - wire 注入按 `main` 生成文件结构合入；
  - 前端 modal 和 API type 保持 snake_case 字段。

## 前端与 i18n

账号弹窗以 `main` 的状态模型为基底：

- 保留 `codexImageToolMode`，因为它覆盖 build 的 enabled/disabled/inherit，并新增 block 策略。
- 保留 Anthropic APIKey auth scheme。
- 合入 build 的 `codexCLIOnlyCustomUserAgentInput`，并保证 create/edit 两个弹窗都能写入和回显。

i18n 以 `main` 的模块化 locale 目录为准。旧 `frontend/src/i18n/locales/en.ts` / `zh.ts` 删除，build 新增文案迁移到模块文件，避免重新引入大文件。

## 数据库与迁移

Git 合并不会对 migration 编号重复报冲突，但合并后可能出现同编号多文件，例如 `158_*` 和 `160_*`。当前 runner 按文件名排序、按 filename 记录 checksum，历史中已有重复编号。处理原则：

- 不修改已发布 migration 内容。
- 若重复编号仅是不同文件名且排序无依赖冲突，可记录风险并保留。
- 若发现执行顺序依赖，则新增后续 migration 修正，不重命名历史文件。

## 验证策略

后端重点验证 apicompat、OpenAI/Messages/Gateway、settings、Codex reset。前端重点验证 typecheck 和相关账号组件测试。若全量测试耗时过高，先跑 PRD 中列出的目标测试，再根据失败范围补跑。
