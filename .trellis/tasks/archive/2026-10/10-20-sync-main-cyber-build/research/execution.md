# 合并实施记录

## 当前结果

用户确认最终 Brief 和精确提交计划后，已在 build 上完成固定 main 的合并提交并推送 origin/build，同时应用指定 cyber 修复的限定增量。13 文件/19 块冲突全部解决，77 个共同文件和双方独立文件已核对，V01–V16 本地验收完成。推送触发 GHCR 镜像构建；未部署生产或连接业务数据库。

- 原 build：`f03c930bef832df4d49c90de0e895316e7ffc1fd`。
- 合入 main：`8f2cbb53d9d1c40e1865b07ac460571f7d4c48de`（0.2.17）。
- 恢复引用：`backup/build-before-main-cyber-20261021`，指向原 build。
- 合并提交：`f450d4e5b84718bdb590f28cf1221ec8299fb4c4`，双父顺序为原 build、固定 main；远端推送已核对。实际 tree、命令、结果和日志校验值见 `execution-state.json`。
- 对照清单：[77 文件](shared-files-audit.md)、[16 组验收](validation-matrix.md)、[独立文件保留](independent-retention-actual.json)。

## 冲突与组合适配

- 统一路由保留 main provider profile、新平台、模型退役与 build Grok 默认 Responses/显式强制 Chat。所有实际选中 raw Chat 的 Responses 形状均转换，并复用 main 的 reasoning cache；搜索保护保持独立 owner 与薄调用。
- `ResponsesContentPart` 同时保留 refusal 与 annotations；流聚合保持稀疏索引映射、引用累积，并先拒绝负索引。
- Antigravity native Gemini 先补 functionCall 思考签名，再走 GIF 抽帧和完整包装大小校验；实际出站同时断言 PNG 和 thoughtSignature。
- Messages 直接 Chat 桥接接入可配置 token 下限，保留 disabled、派生 effort、最终实际计费值、SSE 折叠和 Ollama 最终上限。组合断言覆盖 floor=512 与 cap=256。
- 账号编辑字段统一加载/保存，新平台/显式缓存/scope 与 Schema/Lite/Alpha Search/Codex 搜索桥共存；未知 extra 保留，双方 suites 和主要业务断言保留。
- cyber 仅按认证 API Key 与六类明确会话头派生 v2 键；移除 transcript/历史超限及来源推断。保留实际拒绝、白名单、日志和真实用量语义，增加同 ID 已封禁 WS 白名单首轮入口断言。

没有 Git 文本冲突的部分也发现并处理了三项实际组合问题：

1. Claude 5.5 独立文本项遗漏引用：扩展四模型回归后 Opus/Sonnet/Haiku 三种复现失败，统一文本 part 保留 annotations 后通过。
2. OAuth 透传/Chat fallback 的模型目录与实际映射链不一致：目录复用最终模型策略，同时保留未配置映射的透传原生模型；API Key 原生 Responses 继续忽略残留映射。原有透传回归未删改，新旧目录测试和完整 unit 通过。
3. Lite 降级删除请求头后误开 HTTP 图片桥：实际出站红测复现，门禁复用已有降级标记后通过。普通请求的有效桥接、OAuth 不降级和原 Lite 阻止规则同时保留。

## 最终验证

所有 Go 命令在 `backend/`，使用 `/tmp/sub2api-go-verified`（Go 1.27.2、独立模块缓存、并发限制 4）。前端最终在 Node 20.20.2 / pnpm 9.15.9 下运行。精确命令、退出码、时长和 JSON 测试统计见 `execution-state.json`。

| 验证 | 最终结果 | 日志 |
| --- | --- | --- |
| 后端完整 unit：`go test -json -p=2 -parallel=2 -tags=unit ./...` | 退出 0；58 包，23985 个 pass 事件（含子用例），17 skip | `/tmp/sub2api-backend-unit-all-verified.jsonl` |
| 完整 integration + 修复后 service 全包 | 其余 51 包通过；service 最终退出 0，8533 pass、3 skip | `/tmp/sub2api-backend-integration.jsonl`、`/tmp/sub2api-backend-service-integration-verified.jsonl` |
| 私有审计真实 Postgres/Redis | 退出 0；214 pass，0 skip，专用容器已删除 | `/tmp/sub2api-private-audit-integration.jsonl` |
| 最后组合回归 | 退出 0；746 pass、0 skip，含 Lite/图片和目录 | `/tmp/sub2api-final-combinations.jsonl` |
| lint / mod verify | 全部退出 0，golangci-lint v2.14.0 为 0 issues | `/tmp/sub2api-final-lint.log`、`/tmp/sub2api-final-mod-verify.log` |
| 普通/内嵌构建与真实 web 产物测试 | 全部退出 0 | `/tmp/sub2api-backend-build.log`、`/tmp/sub2api-final-embed-build.log`、`/tmp/sub2api-final-embed-web.log` |
| 前端 frozen install / lint / typecheck / test / build | 全部退出 0；391 文件、2969 测试，完整测试启用单次 retry | `/tmp/sub2api-ci-frontend-*.log` |
| 最终 locale 与定价 | en/zh 各 8534 叶子键，差集零；双方改变的定价零丢失 | `/tmp/sub2api-final-locale-keys.json`、结构化源码比较 |
| Ent / Wire | 生成成功，与最终生成物无非预期差异 | `/tmp/sub2api-generate-ent.log`、`/tmp/sub2api-generate-wire-final.log` |
| 旧库升级与空库 | 289→296 迁移、幂等重跑、历史订单、分组默认值、新平台、cache/channel/lease 均通过；空库 repository integration 通过 | `/tmp/sub2api-migration-upgrade.log`、完整 integration |
| 部署资产 | 5 脚本通过；4 Compose×空/含空格密码共 8 项通过，内置 Redis 的实际 argv/PONG 正常 | `/tmp/sub2api-deploy-*.log`、`/tmp/sub2api-compose-runtime-result.json` |

迁移探针复制真实 migration runner，先运行原 build 289 条，再写入 fixture 并升级到当前 296 条；没有启动业务应用。旧 289 条 SQL 与原 build/main 逐字节一致，新 7 条与 main 一致。

## 中间失败及归因

- 原 Go 缓存存在大量空 go.mod/Go 源文件；官方 checksum 与项目记录一致。使用独立完整缓存，没有禁用校验。生成工具补入五个传递模块共十行 go.sum，go.mod 未额外变化，最终 mod verify 通过。
- Wire 首次缺 subcommands 校验和，按锁定工具版本补齐后重新生成成功。
- 旧监控空模型用例依赖公网域名 DNS，提前触发 hostname 错误；仅该 fixture 换成已有公网 IP，不执行真实探测，不改变生产校验。
- 模型目录初版把透传映射当作白名单，原生模型保留测试抓到回归；修正实现后原测试保持不变并通过。退休 OAuth 模型未重新开放，API Key 原生透传的 NotModified/ETag 原语义保留。
- 协议 golden 原始文件未改。1296 项仅允许 input→messages 的完整观察修正，48 项 Grok 只允许精确默认 Responses URL/认证/模型/错误；其余观察和异常继续严格比较。
- 前端中途 JIT 配置试验已完全撤回。最终新翻译测试使用真实聚合文案与预编译消息函数，Vitest 全局配置没有额外改动；最终在 CI 对齐环境完整通过，retry=1 如实保留。
- 中间缓存 TTL 和裸错误流出现时序失败。缓存源码/测试与两边相同，独立重复 20 次通过；裸错误超时在固定原 main 重复 20 次也有 1 次 3 秒超时，当前重复 12 次同样有 1 次。未改测试等待条件或产品超时；最终完整 unit 退出 0。原始失败和 baseline 日志均保留。

## 范围与交付边界

显式 skip、真实供应商、真实支付、TLS 外部捕获、缺失签名插件包和历史 NULL fixture 限制均没有算作通过。审计的 opt-in DB/Redis 场景已单独补跑。前端测试/后端模拟请求不等于生产全链路验收。

上线后由发布方确认下游稳定传递独立会话 ID、所有旧实例退出、实际供应商响应及生产数据/计费。代码备份不等于数据库回滚；已提交合并采用正常 revert 保留历史，不回滚业务数据库。代码推送已执行并触发镜像构建，生产部署/迁移及真实模型调用没有执行。

## 规范收尾与交付

- Update-Spec 已补齐 cyber v2 身份、统一路由与 Grok 覆盖、目录最终模型、Lite 图片桥与 GIF 签名顺序；仅改 3 份规范文档，签名、索引和格式验证通过。
- 用户已确认 674 个业务文件与 36 个任务文件的两次提交/推送计划；业务合并提交及远端结果已核对，任务记录由本次独立提交同步。
