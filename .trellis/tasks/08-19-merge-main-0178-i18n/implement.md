# 合并 main 0.1.178 到 build 并保护多语言 - 实施计划

## 1. 准备与预演

- [x] 检查 `build` 工作区和目标分支，确认合并前提交 `442ad2e20`。
- [x] fetch 最新 `origin/main`，固定目标为 `359fd12b2`、版本 `0.1.178`。
- [x] 记录共同基线 `fbfdcef81` 与双方独有提交数 `241 / 124`。
- [x] 使用 merge-tree 预演，识别 11 个预计硬冲突。

## 2. 真实合并与冲突处理

- [x] 执行 `git merge --no-commit --no-ff origin/main`。
- [x] 按设计矩阵解决 README、Codex 模型发现、Gateway、Messages、测试和 Vue 表单冲突。
- [x] 在 Messages route 中补齐 Kimi、DeepSeek、Zhipu 协议覆盖及测试。
- [x] 同时保留 Codex beta features 与 Responses Lite Header 请求头处理。
- [x] 清理冲突标记，确认未合并索引为空并运行格式化。

## 3. 无标记语义回归修复

- [x] 修正 Grok 创建表单测试对旧三元表达式源码形态的脆弱依赖，改为断言现有 switch
  分支返回值。
- [x] 将 Responses 透传模型映射拆分为 OAuth、API Key、compact 和 Chat fallback
  四种契约。
- [x] 保证 Responses Lite 策略仍基于 OAuth 映射后的最终模型判断，同时不改写 API Key
  原生透传 body。
- [x] 修复 Check-All `CHK-001`：将最终上游模型解析集中到
  `openai_model_mapping.go`，由 Forward、渠道限制、Responses Lite 与 compact/reasoning
  共同复用，并补齐 OAuth/API Key/compact 组合回归测试。

## 4. i18n 审计

- [x] 检查本次变更的 12 个中英文主 locale 文件内容成对。
- [x] 检查 build locale extension 文件成对存在，index import 和对象展开仍有效。
- [x] 扫描主线新增键与 build 扩展键，无未决同键覆盖。
- [x] 运行 locale 编译、无键冲突、build extension、OpenAI Fast Policy locale 测试。

## 5. 验证

- [x] 运行 Codex 模型发现、Messages route、GLM/DeepSeek fallback、账号映射聚焦测试。
- [x] 运行 OAuth/API Key/compact/Chat fallback 与 Responses Lite 策略聚焦测试。
- [x] 运行前端 `pnpm typecheck` 和 `pnpm lint:check`。
- [x] 运行前端全量 Vitest：248 个测试文件、1709 个测试通过。
- [x] 修复后重新运行 `go test -tags=unit ./... -count=1`，所有包通过；
  `internal/service` 非缓存耗时 162.386 秒。
- [x] 运行 `git diff --cached --check` 和 `git ls-files -u`，结果通过。

## 6. 任务与 Git 收尾

- [x] 按用户要求回溯创建 Trellis task，并补齐 `prd.md`、`design.md`、`implement.md`。
- [x] 生成并展示最新 `brief.md`，等待用户确认回溯任务记录。
- [x] Repair Check-All 通过，剩余 `CHK=0`、`FBK=0`；将最终上游模型解析契约写入
  `.trellis/spec/backend/protocol-adapter-guidelines.md`。
- [ ] 通过 `trellis-push` 展示精确 merge commit 计划并取得确认。
- [ ] 创建双父 merge commit，验证父提交和 `origin/main` 祖先关系。
- [ ] 如用户确认推送，则以普通模式推送 `build -> origin/build`；禁止 force push。
