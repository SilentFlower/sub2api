# 技术设计

## 边界与基线

在 build 原工作区执行固定 SHA 的 --no-commit 合并。开始前创建 backup/build-before-main-0201-4e9829519，记录原 HEAD、MERGE_HEAD 与工作区状态。任务文档独立保存，避免冲突处理覆盖工程资产。目标 SHA 不在执行中随远程变化。

## 冲突处置

1. GPT-6：constants、model alias、Codex model descriptor、transform、reasoning max 等共享文件中仅将 GPT-6 语义收敛到 main；GPT-5.6 专用模板保留。移除 GPT-6 本地模板及选择分支，测试跟随 main。
2. GLM：build 已把归一化迁至 openai_provider_reasoning_effort.go。保留领域结构，增加 main 的 GLM-5.3 low 特例及 Anthropic thinking 适配；所有现有 provider 入口共用该行为，保留 Grok 4.5 分支及 none/minimal 原契约。迁移上游新测试，避免恢复同名旧实现。
3. Messages fallback：保留 build 的上游强制流式、缓存稳定、工具顺序和非流式聚合，将 UpstreamHeaders 接入实际成功/不完整响应路径，不能把上游旧 JSON 函数尾部误拼到请求头 helper。
4. 账号组件：保留 build 的 JSON Schema 状态、兼容 payload helper、搜索桥接设置；合入 images_url_to_b64_json 状态及初始化/保存/清理逻辑。其它 main 自动合并的新功能照常保留。
5. locale：保留双方 import、深层 spread 和新 key；核对最终聚合后的中英文 key 与文案，避免领域 override 隐藏上游新语义。

## 自动合并语义复核

完整复核 research/overlap.txt 的 63 个共同修改文件。重点包括模型 metadata/default prompt、HTTP/WS Lite 策略、搜索循环、图片工具、响应头与计费字段、设置 DTO/handler/service/frontend，以及自动合并重复定义。保留清单与源码依据见 research/retention.md。

## 数据与生成文件

上游新增 migration、Ent schema/生成代码与 DTO 按上游成套合入；不对现有迁移做修改，不执行真实迁移。若冲突解决确需修改生成源才重新生成，禁止手改生成文件。

## 回退

未提交合并使用 git merge --abort 回到原 build；任务文档保留。备份分支提供原 HEAD。若产生环境性验证失败，记录命令和输出，先修复合并引入的问题。
