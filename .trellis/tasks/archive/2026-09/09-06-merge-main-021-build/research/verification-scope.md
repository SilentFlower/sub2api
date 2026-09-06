# 检查范围与证据分层

## 必需验证基线

1. 后端 `go test -tags=unit ./...`。
2. 后端 `CGO_ENABLED=0 go build ... ./cmd/server`。
3. 后端标准 `golangci-lint run ./...`。
4. 前端 `pnpm typecheck`。
5. 前端 `pnpm lint:check`。
6. 前端 `pnpm test:run --maxWorkers=2 --minWorkers=2`。
7. 前端 `pnpm build`。

测试进程退出码、命令和耗时以最终 `validation-results.json` 为准。初次失败与被中断的运行不算通过；不以缓存存在、进程消失或日志没有 FAIL 推断成功。

## 定向复查

- GPT-6 名称、WebRun 搜索、Typed/Lite/Bridge、GLM 与强制 Chat 的相关用例：覆盖响应头 fixture 和旧 GPT-6 断言修正。
- `TestSelectAccountWithLoadAwareness_`：在全量通过之后，对最后一次新增测试 fixture 的类型断言检查做定向复测。
- 增强 `--build-tags=unit --new-from-rev=HEAD` lint：校验本次相对原 build 的新增告警。初次完整 unit lint 的 57 项告警为额外诊断基线，不能把增强检查的失败表述为全部 lint 已通过。
- Git：工作区和索引空白检查、冲突标记扫描、未解决索引、版本、固定 MERGE_HEAD 和最终索引树指纹。

## 交叉检查

- 需求 R1–R8 的实现映射见 `../merge-review.md`，原始保留决策见 `retention.md`。
- 63 个共同改动文件完成语义复核；202 个上游独立文件及 565 个 build 独立新增资产按最终树核对，差异例外见 `final-tree-audit.json`。
- API/数据流：账号 extra → 前端保存及后端读取，GLM 最终模型映射 → 归一化 → effort 记录，协议回退结果头 → 请求 ID → usage 存储/展示。
- 历史数据：上游迁移、Ent/DTO 成套合入；缺省/null 处理与静态存储契约已核对。真实数据库迁移不在本轮执行范围。
- 组件状态：新建、编辑及重新打开账号时，同时保留兼容配置和上游新增图片设置；最终中英文聚合校验纳入全量前端测试。

## 上线验证边界

部署负责人在授权上线时检查新增字段迁移、旧记录读取、上游请求 ID 展示、真实 provider 的 GLM-5.3 与生图/搜索行为；本次没有执行部署、真实数据库迁移或外部 provider 调用。
