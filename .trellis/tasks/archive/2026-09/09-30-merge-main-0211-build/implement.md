# 实施计划：main 0.2.11 合入 build

## 1. 规划与启动

- [x] 获取 main，固定输入提交，完成预览并取得用户对预览和任务跟踪的确认。
- [x] 建立 PRD、技术设计、实施计划和原预览存档。
- [x] 生成 71 个共同修改文件的待复核清单，配置实施与检查上下文。
- [x] 从最新三件套生成并完整展示 Brief，完成项目启动评审，再运行 `task.py start`。
- [x] 加载 Phase 2.1，经 `trellis-route(target=implement)` 确认为 inline；按 `trellis-before-dev` 加载规范，具体领域章节随对应修改补充。

## 2. 合并与契约适配

- [x] 重查 HEAD、main、工作区和索引；目标未变化，保护本任务材料。
- [x] 备份名未占用，已创建 `backup/build-before-main-0211-9c914278c` 并执行固定 SHA 的未提交 merge；实际 10 个冲突文件与预览一致。
- [x] 按 design 逐块处理 10 个冲突文件，不整文件选 ours/theirs；读接口、请求类型、DTO、实际调用者与测试后修改。
- [x] 同步搜索解析器、归一化返回值及异步任务/趋势查询等跨侧签名；补齐 stub/mock。
- [x] 补接 Messages -> Chat Ollama helper，添加真实出站回归。
- [x] 按 `research/shared-files-audit.md` 完成 71 个文件的复核、处置、证据链接。
- [x] 对照构造器复核 Wire；实际源码/生成文件依赖一致且完整 unit、构建通过，无需生成，不运行 Ent 生成。

## 3. 分层验证

- [x] 先跑定向测试：搜索成功/失败/截断/回退、首事件工具参数、模型/disabled/最终 effort、预占生命周期、Lite/WS、图片和非流式超时、Ollama 出站参数；其余匹配不到的入口由完整 unit 覆盖。
- [x] 复核设置和账号完整往返，运行关键新旧字段组件测试、Claude 重置 mock 测试和 i18n 专项。
- [x] 后端完整 unit、CI 默认全量 lint、二进制构建通过；额外 unit-tag lint 的存量问题和本次增量复查分开记录于 `research/validation.md`。
- [x] 前端 typecheck、lint、全量测试和生产构建通过；最终 353 文件、2660 用例全部通过，并完成真实前端资源的后端 embed 测试和构建。
- [x] 隔离验证新增 Redis/Key 限制/重置幂等数据路径；为每项记录测试名称、退出码和环境。无法执行的真实上游验证明确列为上线后验证。
- [x] 复核版本、冲突标记、索引、两个预期父提交及独立文件保留；必要偏差记录于研究结果。
- [x] 汇总验证结果至任务研究记录，逐项更新 PRD 验收状态，完成后才能宣称相应行为通过。

各命令从对应目录单独执行，重任务串行。先检查工具和资源可用性，Go 编译并发及测试并行度显式限制；不把 `wsl-safe-run` 视为限额工具。

```bash
# backend
go test -tags=unit ./...
golangci-lint run ./...
make build
```

```bash
# frontend
pnpm typecheck
pnpm lint:check
pnpm test:run
pnpm build
```

```bash
# frontend：国际化专项，可复用同一轮全量测试中的实际通过证据
pnpm exec vitest run \
  src/i18n/__tests__/localeKeyCompleteness.spec.ts \
  src/i18n/__tests__/localesMessageCompile.spec.ts \
  src/i18n/__tests__/localesNoKeyCollision.spec.ts \
  src/i18n/__tests__/buildFeatureLocaleExtensions.spec.ts
```

## 4. 质量收尾

- [x] 加载 Phase 2.2，经合法 inline route 进入 Full Check-All；已确认范围内的修复经定向和必要完整重检，复用仍有效的验证证据。
- [x] Check-All 三维度及 11 项 AC 通过，按交互停止门禁展示结果与残余风险；尚未进入后续规范收尾。
- [x] 保留未提交 merge 结果，等待后续提交/推送流程的精确范围确认。

## 执行约束与回退点

- 新增注释使用中文，复制原缩进，修改前读取真实类型与签名；定制规则由独立领域文件拥有。
- 失败测试区分回归、上游有意变化和存量问题，不能删除有效断言来通过检查。
- 保留用户新改动；不运行历史重写、线上操作、真实模型请求或真实重置。
- 未提交 merge 的回退基线为原 build；仅确认放弃合并且不存在用户新增改动时使用正常 abort。
