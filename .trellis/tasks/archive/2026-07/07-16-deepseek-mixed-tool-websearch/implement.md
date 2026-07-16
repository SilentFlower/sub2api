# 实施计划：Chat fallback 混合工具 Web Search

## 1. 准备与基线

- [x] 读取 `implement.jsonl` 中的协议、错误、日志、复用规范和历史研究。
- [x] 运行现有 Web Search/Web Run 定向测试，记录修改前基线。
- [x] 再次核对 `ResponsesRequest`、`ResponsesTool`、Chat tool、Responses output/annotation/event DTO，禁止猜测字段。

## 2. 能力分类与入口

- [x] 扩展 typed Web Search 决策，增加 `chat_tool_loop`，保留 direct/native/reject 现有语义。
- [x] 校验混合工具别名、重复声明、`tool_choice`、search context、filters、max uses 和无等价高级字段。
- [x] 更新决策日志与单元测试，确认策略关闭仍明确拒绝。

## 3. Chat 内部代理

- [x] 在 Chat fallback service 层注入固定 `sub2api_web_search` function 代理，不改变通用 apicompat 默认行为。
- [x] 实现工具名冲突检测、typed 查询 Schema、客户端约束映射和 `parallel_tool_calls=false`。
- [x] 保持其它 function/custom/namespace/tool_search Schema 与 tool choice 回程不变。

## 4. 复用受控搜索循环

- [x] 把现有 `web.run` 循环的公共部分提炼为可参数化内部 Web 工具循环，避免复制 provider、续跑、usage 和 writer 逻辑。
- [x] 为 typed 代理增加参数解析、结果预算、domain filter、max uses 与真实成功调用累计。
- [x] 保持 provider tool error、代理 failover、查询/轮次上限、缺失 call ID 和并行调用防护。
- [x] 确保模型选择其它客户端工具时直接沿现有 Responses 回程，不泄漏内部代理。

## 5. 来源引用

- [x] 新增共享来源投影：规范化 URL 去重、首次出现顺序、最多 5 条、长度限制。
- [x] 非流式最终文本追加 `Sources:` 后缀并生成 rune 索引正确的 `url_citation`。
- [x] 流式插入来源 delta/annotation 事件，并同步 done/item/completed 的文本、annotations 和 sequence/output index。
- [x] 结构化输出、无最终文本、客户端工具回程、失败或未执行搜索时不追加来源。

## 6. 测试

- [x] 在 service 测试中新增生产复现形态和 typed 混合工具循环的流式/非流式覆盖。
- [x] 覆盖不搜索、其它客户端工具、required/none/forced-other、冲突、重复声明和不支持字段。
- [x] 覆盖 provider/代理/上限/usage/计费、来源去重、Unicode 索引和客户端断连。
- [x] 运行现有 Web Run、直接 Web Search、Responses Chat bridge 和 Anthropic Web Search 回归。

## 7. 验证命令

```bash
cd backend
gofmt -w internal/service/openai_responses_websearch.go internal/service/openai_responses_web_run.go internal/service/openai_gateway_responses_chat_fallback.go internal/service/*websearch*_test.go internal/service/openai_gateway_responses_chat_fallback_test.go
go test -tags=unit ./internal/service -run 'Test.*(WebSearch|WebRun)' -count=1
go test -tags=unit ./internal/pkg/apicompat -count=1
go test -tags=unit ./internal/service ./internal/pkg/apicompat -count=1
go test -tags=unit ./... -count=1
golangci-lint run ./...
git diff --check
```

如实际修改文件集合不同，gofmt 仅对本任务修改的 Go 文件执行，不使用通配符改写无关文件。

验证结果：

- `go test -tags=unit ./internal/service -run 'Test.*(TypedWebSearch|WebSearch|WebRun)' -count=1` 通过。
- `go test -tags=unit ./internal/service ./internal/pkg/apicompat -count=1` 通过。
- `go test -tags=unit ./... -count=1` 通过。
- `GOTOOLCHAIN=go1.26.5 go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.9.0 run --timeout=30m ./...` 通过，`0 issues`。
- `git diff --check` 通过。

## 8. 风险与回滚点

- 工具代理注入前：确认只命中 Chat fallback + 策略允许 + 混合可选 typed Web Search。
- 搜索循环抽象后：先跑现有 Web Run 全量测试，任何回归先回到共享边界调整，不复制第二套循环。
- 流式引用完成后：逐事件检查 sequence、output index、item ID、content index 和 completed 快照。
- 无 migration、配置或数据变更；代码回滚即可恢复旧行为，账号开关关闭可作为运行时回滚手段。
- 不执行生产部署、镜像更新、容器重启或远端配置修改。
