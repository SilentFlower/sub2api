# OpenAI OAuth Codex 自定义客户端放行规则实现计划

## Checklist

- [x] 确认本期只支持自定义 UA 前缀/通配符规则，不做任意 header 匹配。
- [x] 后端定义自定义规则结构、解析和匹配函数。
- [x] 后端扩展 `Account` getter 读取 `extra.codex_cli_only_custom_user_agent_prefixes`。
- [x] 后端 detector 增加自定义规则匹配 reason。
- [x] 后端补充单元测试：
  - [x] UA 前缀匹配和 `*`。
  - [x] 多 pattern OR、多规则 OR。
  - [x] 非 OpenAI OAuth / 未开启 `codex_cli_only` / 空规则不放行。
  - [x] 现有官方客户端和 Claude Code preset 兼容。
- [x] 前端创建账号 modal 增加自定义规则编辑与提交。
- [x] 前端编辑账号 modal 增加回显、修改、清空。
- [x] 确认批量编辑纳入本期。
- [x] 批量编辑增加自定义 UA 规则写入/清空能力，且未勾选时不提交字段。
- [x] 确认前端输入形态为多行文本框，每行一个 UA pattern。
- [x] 更新中英文 i18n。
- [x] 补充前端测试。

## Validation Commands

```bash
cd backend && go test ./internal/pkg/openai ./internal/service
cd frontend && pnpm test -- --run src/components/account/__tests__/EditAccountModal.spec.ts src/components/account/__tests__/BulkEditAccountModal.spec.ts
```

如前端测试范围调整，补充对应 modal spec。

## Risky Files

- `backend/internal/service/openai_gateway_service.go`：网关热路径，避免增加 DB 查询或高成本匹配。
- `backend/internal/service/openai_client_restriction_detector.go`：访问控制决策，默认必须安全失败。
- `frontend/src/components/account/CreateAccountModal.vue` 和 `EditAccountModal.vue`：大文件，改动要控制范围，避免影响其他 OpenAI 账号选项。

## Rollback

回滚新增 extra 字段读取和 UI 即可；已存储的新字段在旧代码中会被忽略，不需要迁移清理。
