# main 0.2.17 与 cyber 修复的实施计划

## 当前阶段

用户已确认最终 Brief 与精确提交计划。已完成 main 合并提交及 origin/build 推送、全部冲突处理、cyber 修复及必要组合适配；必要的全量与定向回归、隔离集成、Full Check-All 和规范收尾均已完成。具体命令、失败归因和恢复引用见 research/execution.md。

## 实施前核对

- [x] 用户已审阅完整 Brief 并确认本方案；按 Phase 1.4 启动任务，再进入实现路由。
- [x] 重新核对 `research/refs.json` 的 build/main/补丁提交、工作树及索引；输入变化则刷新预演，不能套用旧冲突表。
- [x] 加载 `trellis-before-dev`、后端协议/数据库/错误规范、前端规范和 build 私有隔离规范；代码变更前读取相关 Account、DTO、响应类型和调用定义。
- [x] 确认 Go 1.27.2、lint v2.14.0、前端 Node/pnpm 与 main CI 兼容，使用 frozen lock 安装依赖。
- [x] 为原 `build@f03c930be` 建立恢复引用，任务文档单独保留。

## 顺序清单

1. [x] 以固定 main 执行 `git merge --no-commit --no-ff 8f2cbb53d9d1c40e1865b07ac460571f7d4c48de`；保留现有 build 历史。
2. [x] 按 `research/merge-preview.md` 的 C01–C13 处理全部 19 块冲突，记录每项最终决定；不整文件选边。
3. [x] 优先完成统一协议路由与类型：字段并集、模型退役 wrapper、搜索 owner、Grok 特例、Responses 形状、负索引、最小 token 配置、GIF 顺序。
4. [x] 按 `research/shared-files-audit.md` 完成 77 项共同文件检查；追踪独立 owner 的入口、参数、调用顺序与返回类型。对 main 删除/替代的旧符号重新核对全部引用。
5. [x] 在 main 合并结果上应用限定 cyber 修复，参考已落盘 `research/cyber-isolation.patch`：先检查来源文件清单，再执行 `git apply --index --3way`。只纳入 11 文件；不 merge fork、不 cherry-pick 空 CI 提交。
6. [x] 确认修复的 service/handler/WS/白名单与新版主线不互相覆盖；旧超限标记和 transcript 推断没有生产引用，v2 namespace、无 ID 语义和文案一致。
7. [x] 整合编辑页搜索三态与所有兼容字段，完整保留双方测试 suites；补充此次组合的新出站/组件断言，避免只验证 helper 被调用。
8. [x] 核对 Ent 和 Wire。若需要适配生成物，从源运行相应生成器：`go generate ./ent`、`go generate ./cmd/server`；检查是否出现非预期依赖或生成差异。
9. [x] 执行 V01–V16 定向回归，修复确认的新回归；保留失败原始日志和双分支基线对照。
10. [x] 定向通过后执行完整后端 unit/lint/构建、完整前端 lint/typecheck/test/build，以及隔离迁移/Redis/部署脚本检查。测试使用现有 harness，明确跳过的项目不得算通过。
11. [x] 重新执行最终文件保留、实际模型/定价、配置字段、i18n 最终值、冲突标记、旧引用和生成物检查；逐项写回 77 文件及矩阵结果。
12. [x] Full Check-All 通过，规范收尾完成；按用户确认的精确范围完成合并提交与推送，任务记录随独立提交同步。生产部署与上线后验证不在本轮执行。

## 定向验证命令

以下从仓库根目录起；先在 backend 目录运行，重任务限制并发。命令中的名称均对应 source/main/build 已有测试或按矩阵需要补充的组合测试，不把无匹配返回 0 当成通过。

```bash
cd backend
GOMAXPROCS=4 go test -p 4 -parallel 4 -tags=unit ./internal/service ./internal/handler ./internal/repository \
  -run 'CyberSession|FindCyberSession|CyberAllowlisted|BuildCyberSessionBlockWritePlan|RejectIfCyberSessionBlocked|OpenAIResponsesWebSocketV2Passthrough.*Cyber|GatewayCacheCyber' -count=1

GOMAXPROCS=4 go test -p 4 -parallel 4 -tags=unit ./internal/pkg/apicompat ./internal/pkg/openai ./internal/service ./internal/handler \
  -run 'UpstreamProtocolRouting|ProviderProfile|GPT55Retirement|ResponsesShape|ConvertedResponsesInputString|Refusal|WebSearch|AlphaSearch|WebRun|Reasoning|MinOutput|MaxTokensFloor|Grok|GIF|GeminiCachedContent|CreatedAt|ImageGeneration' -count=1
```

必要的新组合断言由 `research/validation-matrix.md` 拥有。超时/图片/GIF/搜索、平台/协议、配置保存都应断言实际出站或可见行为，不能以纯私有函数返回值替代入口接线覆盖。

## 全量检查与构建

```bash
cd backend
GOMAXPROCS=4 go test -p 4 -parallel 4 -tags=unit ./...
GOMAXPROCS=4 GOFLAGS=-p=4 golangci-lint run --concurrency=4 --timeout=30m ./...
GOMAXPROCS=4 GOFLAGS=-p=4 make build
```

```bash
cd frontend
pnpm install --frozen-lockfile
NODE_OPTIONS=--max-old-space-size=4096 pnpm lint:check
NODE_OPTIONS=--max-old-space-size=4096 pnpm typecheck
NODE_OPTIONS=--max-old-space-size=4096 pnpm test:run --maxWorkers=2 --minWorkers=1
NODE_OPTIONS=--max-old-space-size=4096 pnpm build
```

```bash
cd frontend
pnpm exec vitest run \
  src/i18n/__tests__/localeKeyCompleteness.spec.ts \
  src/i18n/__tests__/localesMessageCompile.spec.ts \
  src/i18n/__tests__/localesNoKeyCollision.spec.ts \
  src/i18n/__tests__/buildFeatureLocaleExtensions.spec.ts \
  --maxWorkers=2 --minWorkers=1
```

前端构建完成后，验证真实产物嵌入与 API alias：

```bash
cd backend
GOMAXPROCS=4 go test -p 4 -parallel 4 -tags=embed ./internal/web -count=1
GOMAXPROCS=4 GOFLAGS='-p=4 -tags=embed' make build
```

隔离环境具备 Docker 后运行 main CI 约定的集成测试；优先确认 `gemini_cached_content_repo_integration_test.go`、迁移/schema、额度/支付数据约束。缺 Docker 时记录阻塞，不连接线上服务补跑。

```bash
cd backend
GOMAXPROCS=4 go test -p 4 -parallel 4 -tags=integration ./...
```

脚本验证从根目录执行，依据实际 main 工作流核对：

```bash
sh deploy/tests/docker-compose-security-test.sh
sh deploy/tests/docker-compose-gateway-env-test.sh
sh deploy/tests/docker-compose-simple-mode-env-test.sh
sh deploy/tests/docker-runtime-resources-test.sh
sh deploy/test-caddyfile-cache.sh
```

## 停止条件与回退点

- 输入提交漂移、当前未归属代码变更或协议业务范围扩大：停止套用预演，先刷新材料。
- 安全策略/凭据/租户边界、真实计费或数据库行为出现无法确定的新变化：回到设计澄清；不通过删除防护来放行。
- 单元/组件/集成出现失败：先在固定原 build/main 定向对照，记录原因，不能把 main 自带失败算作合并新增，也不能用已有失败掩盖新回归。
- 未提交源码可通过正常 merge abort 回退；已有用户新增改动时先保存并单独处理，不清理用户文件。
- 若后续批准发布，必须另行处理实际迁移与回滚边界；代码备份不等于数据库回滚。
