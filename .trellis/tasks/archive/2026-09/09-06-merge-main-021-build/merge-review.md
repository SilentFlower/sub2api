# main 0.2.1 合并复核

## 合并结果

- 原 build：`4e9829519707f0743a7cbcabcf3e73b8dd0beeb0`；固定上游：`ab99d56e9626e6cd731592dae8553c9758a0efa2`。
- 使用未提交合并；原分支备份为 `backup/build-before-main-0201-4e9829519`。版本文件已为 `0.2.1`。
- 12 个冲突文件、17 个文本冲突块已处理，合并索引没有未解决项。共同修改的 63 个文件已完成语义复核。
- 上游独立修改的 202 个文件中，201 个与 main 完全一致；另 1 个仅补测试 fixture 类型断言的 `ok` 检查。565 个 build 独立新增文件中，仅按约定移除 GPT-6 专用提示词。

## 需求与实现映射

| 需求 | 最终行为及核对依据 |
| --- | --- |
| R1 | 固定 main 的全部更新已合入；版本、合并父提交、独立文件一致性和工程资产核对见 `research/final-tree-audit.json`。 |
| R2 | GPT-6 按 main：模型名称及别名、上下文 1050000、最高 max、priority 服务档位、图像能力、计费与提示词选择；移除 build 的 GPT-6 模板，保留 GPT-5.6 专用模板。`gpt6` 不再被旧 build 规则强制映射到 Astra。 |
| R3 | 保留 AnySearch、本地搜索、web.run、混合工具和 Lite 搜索桥；搜索预算耗尽或搜索服务失败后仍能完成回答。搜索循环结果补齐最后一次上游响应头，接续请求 ID 记录。 |
| R4 | 保留账号 JSON Schema 降级、DeepSeek 推理历史修复；Anthropic → Chat 保留缓存前缀、工具顺序、会话粘性、强制消费 SSE 和非流式 JSON 折叠。四个返回路径均接入 main 的响应头。 |
| R5 | 保留可配置 Lite HTTP/WS 模型阻止名单、OAuth 生图主模型与 effort、客户端 image_gen 识别，以及 Lite 不自动注入 hosted 图片工具的既有边界。 |
| R6 | 保留 Grok 强制 Chat、4.5 effort、额度展示与登录脚本；保留 GIF 多帧与重试、自定义 UA 和出站身份一致性、GPT-5.6 提示词。 |
| R7 | GLM-5.3 规则整合到 provider 领域模块，消除重复实现；保留 GLM none/minimal、Grok 既有规则。账号新建/编辑同时保存兼容设置与上游图片 URL 转 base64 开关；已修正自动合并把 OpenAI 配置插入 Grok 保存分支的问题。工程资产和 main alpha/search 入口保留。 |
| R8 | 中英文最终 locale 聚合及 key 检查通过；已删除的旧 Codex 邀请重置没有恢复。 |

## 验证记录

- Go 全量 unit 测试、后端二进制构建已通过；最新 fixture 改动的定向复测也已通过。
- 前端类型检查、lint、271 个测试文件 / 1936 个用例和生产构建已通过。
- 后端标准 lint 和增强 unit lint 的新增告警检查均已通过（0 issues），完整退出记录见 `research/validation-results.json`，最终审查结论见 `check-report.md`。
- 首轮新增响应头断言揭示 mock Header 键未规范化，已修正为 `X-Request-Id`；GPT-6 旧测试期望已按 main 更新，相关复测通过。
- 增强 unit lint 的初次完整扫描报告 57 项告警，逐行证据见 `research/lint-baseline-lines.json`：55 项所在文件与原 build 完全一致，另 2 项为 main 原样带入的测试辅助函数未使用。增量扫描另发现上游新增用例的未检查类型断言，已修正并单独验证。
- 本机新增资源规则后，所有重任务通过 `/root/.local/bin/wsl-safe-run` 串行执行，保留内存与 CPU 限制。

## 边界

当前未创建合并提交、未推送或部署。没有执行真实数据库迁移和外部 provider 调用；上线时由部署负责人核对新增字段迁移、请求 ID 展示及真实 provider 行为。

Update-Spec 已将既有协议规范中 GPT-6 与 GLM 的旧描述同步为本次用户确认的 main 行为，并补充协议回退与搜索循环的响应头契约；验证见 `spec-update-result.json`。

推送影响核对：现有 `.github/workflows/my-ci.yml` 对 `build` 分支每次 push 自动构建并发布 GHCR 镜像，更新 `build-latest` 和 `latest`；正式提交方案会将此影响一并交由用户确认。
