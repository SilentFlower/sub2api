# 实施计划

## 执行顺序

- [x] 完成任务上下文、Brief 展示并沿用本会话对最终功能方案的确认启动任务。
- [x] 创建 build 备份，使用固定 SHA ab99d56e9626e6cd731592dae8553c9758a0efa2 执行 git merge --no-commit --no-ff。
- [x] 解决 GPT-6、GLM、协议回退、账号组件和中英文冲突，消除自动合并的重复定义。
- [x] 核对全部 63 个共同修改文件及保留功能，修复可达的语义回归，按需补充有意义的回归测试。
- [x] 执行以下验证，记录真实结果和限制。
- [x] 进入统一 Check-All；按审查结果修复并复查。
- [x] 更新必要规范与任务进度，准备具体合并结果供提交审阅。

## 验证计划

后端（backend 工作目录）：

```bash
go test -tags=unit ./...
CGO_ENABLED=0 go build -o /tmp/sub2api-main-021-server ./cmd/server
golangci-lint run ./...
```

实现期间可先跑 internal/service、internal/pkg/openai、internal/pkg/apicompat、internal/handler 的相关用例定位冲突，全量单元测试通过后避免无依据重复执行。

前端（frontend 工作目录）：

```bash
pnpm typecheck
pnpm lint:check
pnpm test:run
pnpm build
```

必须包含 localesMessageCompile、localesNoKeyCollision、buildFeatureLocaleExtensions，账号新建/编辑、Web Search、Lite、生图、GIF 等现有回归；同时核对静态翻译调用和最终中英文叶子 key。全量测试已覆盖时不重复单跑。

Git：git diff --check、git diff --cached --check、git ls-files -u、冲突标记扫描；确认 MERGE_HEAD 为固定上游 SHA，VERSION 正确，build 私有资产和已删除旧功能的状态符合保留方案。

## 审查与回退

所有新代码改动前读取实际类型、字段与签名；新注释使用中文。后端格式化使用 gofmt。保留聚合入口与领域文件的契约，避免直接用整文件 ours/theirs 抹掉另一侧功能。提交前由 Trellis Check-All 给出证据；未提交时可 merge --abort，备份分支为 backup/build-before-main-0201-4e9829519。

## 当前验证结果（2026-09-07）

必需验证 7/7 通过；前端 271 个文件、1936 个用例通过。后端标准 lint 与增强 unit lint 新增告警检查均为 0 issues；最后一次测试 fixture 修改已定向复测。完整命令与退出记录见 `research/validation-results.json`，检查结论见 `check-report.md`。

后端构建实际产物保存于 gitignored 的 `.trellis/.runtime/merge-main-021-validation/server`。初次完整 unit lint 的既存告警、早期失败及中断均未记作通过。新 WSL 资源规则后，重任务经 `/root/.local/bin/wsl-safe-run` 串行运行。

协议规范已同步，具体合并结果已准备好供提交审阅；当前合并待提交，任务保持 `in_progress`。
