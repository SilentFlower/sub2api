# build 独有候选提交逐项分类

## 1. 说明

以下 45 个提交来自：

```bash
git log --right-only --cherry-pick --no-merges origin/main...HEAD
```

并排除了常规 task progress、archive、journal、Trellis 升级等纯 bookkeeping 标题。该集合仍包含维护提交、撤销链和运维资产，因此不能把 45 直接等同于 45 个产品功能。

## 2. 提交分类

| 提交 | 主题 | 分类 | 当前处置 |
| --- | --- | --- | --- |
| `a0e2aaaf` | Chat fallback 混合工具 Web Search | Web Search/web.run | 保留功能主体；薄化 fallback 接入 |
| `a02dca33` | Responses Lite 生图桥接 | Responses Lite/生图 | 已有独立策略；继续迁出共享入口测试与编排 |
| `2f460f5e` | 补全 web.run 搜索事件 | Web Search/web.run | 保留在独立 web.run 主体 |
| `62661ccb` | 恢复独立 Grok 套餐额度显示 | Grok 独立 Billing | 保留独立 service/component/queue；迁出共享投影与文案 |
| `d7e97a71` | 支持 Codex web.run 搜索循环 | Web Search/web.run | 保留独立工具循环主体 |
| `831eaa4a` | JSON Schema 降级与 Web Search | Structured Outputs/Web Search/AnySearch | 保留独立实现；抽离账号/设置接入 |
| `c3e7a6a0` | 修正 merge commit 文件集校验 | Trellis 开发工作流 | 已位于 Trellis 资产，不进入产品代码重构 |
| `a9ad55b3` | 撤销旧 JSON Schema/权限恢复实现 | 撤销链 | 与 `62091bfa` 抵消，不作为当前独立功能 |
| `6d1adc9f` | 撤销旧任务进度提交 | bookkeeping 撤销 | 不属于产品功能 |
| `62091bfa` | 旧 JSON Schema/权限恢复实现 | 被撤销实现 | 当前由 `831eaa4a` 的新任务替代 |
| `1fdfe9dc` | HA 详细探测不阻塞续租 | HA/DR | 已在独立自动化 artifacts 中隔离 |
| `c7a40a73` | HA 心跳与租约规范 | HA/DR 规范 | 已在独立规范中隔离 |
| `f59991b5` | 避免新版 Codex 生图回退旧工具 | Codex 生图工具边界 | 保留功能主体；薄化 transform/WS 接入 |
| `53fea533` | 修复全量 lint 阻塞 | 维护 | 不形成独立产品功能，保留必要修复 |
| `91348728` | 共享 setting repository 测试桩 | 测试基础设施 | 可迁入功能测试 helper，但不视为产品能力 |
| `b5a51fd0` | Codex 生图 tool_choice none | Codex hosted bridge | 保留 `none -> auto` 与客户端工具分流契约 |
| `d390f057` | Codex Alpha Search 独立端点 | Alpha Search | handler/service 已独立，仅保留 route/endpoint 薄注册 |
| `0f1c4a6b` | HA 心跳与租约时长 | HA/DR | 已在独立 artifacts 中隔离 |
| `771765f9` | 自动故障切换与镜像同步编排 | HA/DR | 已在独立 artifacts 中隔离 |
| `45561e0d` | 容灾镜像同步与提升门禁 | HA/DR | 已在独立 artifacts 中隔离 |
| `d5fef3ec` | 双节点主备容灾 | HA/DR | 已在独立 artifacts 中隔离 |
| `792c51ff` | 统一 Codex 客户端版本身份 | Codex 客户端身份 | 独立 identity 文件作为单一来源；消费者薄调用 |
| `05918460` | Grok 4.5 effort 归一化 | Grok/GLM reasoning | 抽离纯归一化与 usage 投影，入口不复制规则 |
| `60d36274` | GPT-5.6 max effort | 模型能力元数据 | main 已覆盖部分运行时；保留 build 当前仍独有的模型/价格能力标记，删除重复算法 |
| `a328495d` | 套餐额度进度条对齐 | Grok 独立 Billing UI | 合并到独立组件测试与文案模块 |
| `cbd34d3a` | 套餐额度摘要展示 | Grok 独立 Billing UI | 合并到独立组件测试与文案模块 |
| `57e409da` | 新增套餐额度进度条 | Grok 独立 Billing | 当前由 `62661ccb` 恢复后的双链路契约承接 |
| `421df83b` | Grok messages 强制 Chat Completions | Grok force-chat | 保留 bridge/fallback 主体；抽离账号 extra 与入口判断 |
| `59d4c7e7` | 更新 Flower manifest | Trellis 开发工作流 | 已在独立元数据中隔离 |
| `74d2b819` | Codex 导入测试参数修复 | 测试维护 | 不形成产品功能 |
| `7e3b32af` | Codex 邀请弹窗中文文案修复 | Codex reset UI | 并入 reset locale/组件领域 |
| `c9d52416` | Codex reset 过期时间展示 | Codex reset | 并入独立 reset service/modal/API |
| `89e05bdc` | Raw Chat 调试日志 | 可观测性 | 当前仍修改 gateway 共享文件；应迁入明确 debug helper |
| `8f070522` | Anthropic metadata 粘性 | Anthropic direct bridge | 迁入 session/sticky 领域 helper，handler 薄调用 |
| `d6d3f1bf` | 并行工具结果顺序稳定 | Anthropic direct bridge | 保留在协议主体，不继续拆碎 |
| `014d69de` | Anthropic Chat 缓存前缀稳定 | Anthropic direct bridge | 保留在协议主体，不继续拆碎 |
| `524b9b7a` | 生图主模型与思考预算可配置 | OpenAI 生图设置 | 设置字段中央保留；cache/normalize/UI 面板按功能隔离 |
| `24316df9` | thinking disabled 与 reasoning effort 互斥 | Anthropic direct bridge | 保留在协议主体与类型契约 |
| `5dba3180` | 非流式 Messages Chat 强制 JSON content type | Anthropic direct bridge | 保留在 fallback 主体 |
| `e086ca5d` | Anthropic Messages ↔ Chat 直连桥 | Anthropic direct bridge | 已是功能主体；共享 messages 入口薄化 |
| `416943fe` | Codex 自定义 UA 放行 | Codex 客户端策略 | 后端 matcher/前端 utility 已独立；迁移 Account 方法、表单块、文案和测试 |
| `d53e8b6c` | Codex reset 完整上游错误 | Codex reset | 保留在独立 repository client |
| `c084180c` | Codex reset 上游错误原因 | Codex reset | 保留在独立 repository client |
| `e1a089e4` | Codex reset 邀请重置 | Codex reset | repository/service/modal 已独立；中央 route/Wire 最小注册 |
| `4e75bfce` | 手动镜像构建与 fork CLA 策略 | CI/部署 | 保留独立 GHCR workflow，并防止后续误恢复只适用于上游仓库的 CLA workflow；纳入 build 分支资产清单 |

## 3. 非普通提交来源

仅看上述非 merge 提交仍会遗漏直接形成于 merge commit 的 build 增量。当前至少包括：

- `d3988a03`：Responses Lite 阻止模型设置、HTTP/WS/bridge 最终模型策略、Header Override 禁止注入内部 Lite Header，以及对应前后端契约。
- 历次 `merge(main)` 决策迁移到 main 新文件结构中的 Anthropic bridge 稳定性、Grok 双链路、Codex 图片 namespace/tool_choice、Codex reset 双入口共存和模型能力元数据。
- 合并时为适配 main 新构造器、DTO、Wire 和测试结构产生的兼容修改；这些不是独立功能，但会形成当前树差异。
- `README_CN.md` 的 Claude Code Plan Mode 已知问题说明，以及 `.agents` / `.trellis` 中累积的工作流、规范、归档任务和 HA/DR artifacts，不一定对应单个当前候选提交，但仍是 build 分支资产。
- 最新 main 已覆盖的 GPT-5.6 模型感知算法、Grok 通用 quota/SSO/reconcile 和其它共享能力不得因为历史曾在 build 先实现就继续计作当前私有功能；只记录仍存在的差异契约。

因此最终功能清单必须同时使用当前树差异、first-parent merge 任务记录和本表，不能只依赖普通提交标题。
