# Implement Plan: 合并 main 到 build 并保留 build 功能

## Checklist

1. 确认当前 `build` 工作区干净，拉取最新 `origin/main` / `origin/build`。
2. 在当前 `build` 执行 `git merge origin/main`，保留冲突现场。
3. 解决简单冲突：
   - `README_CN.md` 保留 build 的 Claude Code Plan Mode 已知问题说明。
4. 解决后端桥接冲突：
   - `openai_gateway_messages.go` 使用 `forwardAnthropicViaRawChatCompletions` 名称；
   - 将 build 的直连 Anthropic Chat 桥接逻辑迁入 main 的 fallback 文件结构；
   - 保留 `backend/internal/pkg/apicompat/anthropic_chatcompletions.go` 和测试。
5. 解决 gateway 大文件拆分冲突：
   - 以 main 的拆分文件为基底；
   - 迁移 build 的 raw chat 调试日志、OpenAI 生图设置调用、Anthropic Chat fallback 行为；
   - 不恢复 build 版 `openai_gateway_service.go` 大文件。
6. 解决 settings / handler 拆分冲突：
   - 将 build 的 OpenAI 生图设置、Codex reset、Codex custom UA 相关字段迁移到 main 的 `setting_*` / `setting_handler_*` 拆分文件；
   - 更新 DTO、settings view、wire 相关代码。
7. 解决前端账号弹窗冲突：
   - 保留 main 的 `codexImageToolMode` 和 Anthropic auth scheme；
   - 合入 build 的 custom UA 输入、回显和写入。
8. 解决 i18n 冲突：
   - 删除旧 `frontend/src/i18n/locales/en.ts` / `zh.ts`；
   - 把 build 新增文案迁移到模块化 locale 文件。
9. 检查 migration 编号重复与排序风险，必要时补充说明或新增后续 migration。
10. 运行 gofmt / 前端格式保持；确认无冲突标记。
11. 运行验证命令并按失败范围修复。

## Validation Commands

```bash
cd backend && go test -tags=unit ./internal/pkg/apicompat
cd backend && go test -tags=unit ./internal/service -run 'Test.*Messages|Test.*Chat|Test.*Anthropic|Test.*OpenAI|TestOpenAIImage|TestOpenAICodex|TestCodex'
cd backend && go test -tags=unit ./internal/handler ./internal/repository -run 'OpenAICodexReset|Setting|Account'
cd frontend && pnpm typecheck
```

可按冲突修复结果补充运行：

```bash
cd frontend && pnpm vitest run src/components/account/__tests__/EditAccountModal.spec.ts src/components/account/__tests__/BulkEditAccountModal.spec.ts src/components/admin/account/__tests__/OpenAICodexResetModal.spec.ts
```

## Risk Points

- `main` 与 `build` 都实现了 `/v1/messages` raw Chat fallback，但目标不同。错误选择 main 的两段转换会丢失 build 的缓存稳定修复。
- 大文件拆分导致同一逻辑移动到多个文件，容易遗漏 import、wire 注入或测试 stub。
- i18n 从大文件迁移到模块文件，容易出现 key 路径漂移。
- migration 编号重复不一定触发编译错误，但可能影响执行顺序理解。

## Rollback Points

- 当前用户已保留 `build-bak` 分支。
- 合并提交前可用 `git merge --abort` 回到 merge 前状态。
- 若某一类功能迁移风险过高，可先完成 merge 基底，再用独立提交 cherry-pick / 手动迁移 build 功能。
