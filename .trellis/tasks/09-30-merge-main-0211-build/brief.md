# Brief — 同步 main 0.2.11 到 build 并保留定制功能

## Goal

- 将固定 main 0.2.11 合入 build，保留当前定制功能，并验证冲突及自动合并区域的行为正常。

## Scope

- 合入 main `42bc7f6cf`，处理 10 个冲突文件、13 个冲突块，逐一复核另外 61 个自动合并共同修改文件。
- 保留模型提示词、粘性会话、Grok、Lite、搜索、Schema/DeepSeek、图片/超时、HTTP/WS 路由、计费和账号设置定制。
- 接入上游新模型、预占、Key 限制、图片价格继承、风控白名单和 Claude 重置；补齐 Messages -> Chat 的 Ollama token 裁剪接入。

## Non-Goals

- 不同步固定 main 之后的提交或 other remote，不恢复已删除功能，不做无关重构或历史重写。
- 不操作业务数据库、调用付费模型或消费真实重置次数；本轮不自动提交、推送或部署。

## Technical Overview

- 为原 build 创建备份，执行未提交 merge；逐块结合双方调用链解决冲突，定制策略保持独立领域归属。
- 搜索解析同时保留 searched 和 error；工具首事件参数与 delta 分开处理；disabled 按实际供应商协议投影且优先于派生 effort。
- 复核预占与异步计费引用生命周期、最终出站 effort/tier、配置全链路往返和 Wire，并运行定向及完整回归。

## Context & Decisions

- 原 build 为 `9c914278c`，保留其三个本地工程提交；用户已确认原预览，详细策略见任务 research 存档。
- 原生 OpenAI 的 disabled 使用 none；兼容上游保留 thinking=disabled，GPT-6.1 Sol 按上游拒绝非法 none/minimal。
- 保留上游默认预占开启、TTL 900 秒、活动 Key 上限 200 和每小时创建上限 60；没有新增数据库迁移或依赖更新。

## Risks

- 搜索成功判定、推理关闭和最终计费字段存在跨侧契约变化；自动合并也可能破坏定制调用链。
- 预占可能影响并发余额准入；新旧配置可能互相覆盖；静态文件与 locale 检查不能替代行为回归。

## Acceptance

- 无未合并索引或冲突标记，版本正确，原 build 可恢复，独立文件保留或必要适配均有记录。
- 搜索成功/失败/截断/回退、工具参数、模型/推理、计费、预占释放、Lite/WS、图片/超时及 Ollama 实际出站测试通过。
- 账号与风控配置往返、新旧字段、Claude 重置 mock、最终中英文 key 和关键双语界面通过验证。
- 后端 unit/lint/构建、前端测试/typecheck/lint/构建通过，71 项文件复核有证据，未覆盖场景明确列出。

## Next Step

- 启动评审通过后激活任务，经 trellis-route 进入实施。
