# Implementation Plan: OpenAI 响应头超时分档

## Steps

- [x] 核对图片直调、Responses 桥接、文本原生与 passthrough 路径的上下文传递，并读取后端规范及相关 DTO/配置定义。
- [x] 添加图片、非流式文本上游 profile 和配置默认值/校验；保持通用 OpenAI profile 的原值与协议行为。
- [x] 在图片及两个文本入口标记入站请求类型，在共用 OpenAI 出站点按账号平台选取 profile；隔离客户端缓存键。
- [x] 同步 Compose 与配置示例，补充图片、文本分类以及连接池回归测试。
- [x] 运行聚焦 Go 测试、配置测试与相关静态检查；按 Trellis Check-All 检查跨层配置和实现。
- [x] 按现有生产发布路径核对构建来源、保留回滚 digest、更新应用容器并验证健康与三档运行值；不发起付费上游生成请求。

## Validation

```bash
cd backend && go test -tags=unit ./internal/config ./internal/repository ./internal/service ./internal/handler
git diff --check
```

部署后核对 `GATEWAY_OPENAI_RESPONSE_HEADER_TIMEOUT=20`、新配置默认 600/300、应用容器健康与错误日志；旧事故请求仅作回归参照，不重放原始图片。

## 生产验证

- 代码提交 `1f6a835bb2ec78dd852efafa51f08784fadcc40c` 的 Build Image、CI 和 Security Scan 均通过；生产镜像 digest 为 `sha256:f83444a653cc6ad24490496cc2b652dd78e5746331998372d00e04cf70de3992`，镜像 revision 与提交一致。
- 原镜像 digest `sha256:d7076719226e20851d43551aa64968f7719b62a19f6a1872d86316177b450987` 已保留本地回滚标签；原 Compose 文件备份为 `/root/sub2api/deploy/docker-compose.yml.bak-1f6a835`。生产 Compose 仅新增两项超时环境变量。
- 只重建 `sub2api` 应用容器；Postgres 和 Redis 容器 ID 未变化。容器与宿主机健康端点均返回 `{"status":"ok"}`，运行环境值为通用 20 秒、图片 600 秒、非流式文本 300 秒；近三分钟日志中无 `panic`、`fatal` 或错误级记录。
- 未发起付费图片或文本生成请求。
