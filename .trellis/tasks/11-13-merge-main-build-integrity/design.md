# 技术设计

## 合并边界

固定 build 的 HEAD e63f900efd14c8429fba0011bdeb69a43abe22e1 与 main 530c5e3a4。共同基线 8f2cbb53d。采用保留历史的普通 merge，实施时使用 --no-commit --no-ff，先记录实际父提交。Flower 升级文件独立保留；合并前比较其路径与 main 变动的交集，遇覆盖则先解决升级变动的保存方式。

## 冲突处理原则

通过共同基线、双方提交、定义和调用方确定所有权。main 已提供等价机制时复用 main owner。build 独有规则放在明确领域文件，共享入口保持薄调用。生成物按源定义重新生成并检查输出范围。

## 冲突域与兼容方案

| 域 | 方案与重点 |
| --- | --- |
| cyber（6 个文件） | 保留 build 明确请求头身份、v2 哈希、白名单、HTTP/WS 准入、审计与用量 guard。依据 protocol-adapter-guidelines.md 的 cyber 场景，保持 transcript 文件删除；main 相关签名变化由现有 owner 与调用方统一消解。 |
| 配置及部署（8 个文件） | 同时保留图片/非流式响应头超时与 DisableOpenAITransportErrorCooldown。合并校验测试、默认值、环境变量及四套 Compose。 |
| 协议与模型 | 合并引用 annotations、逐文本项顺序与 main 新 Claude 模型；采纳流式 buffer 与资源约束时保留 build 分块输出及工具参数修正。GPT 6.1 Terra 支持与 Sol 原始模型记录、effort 校验一起贯通所有调用方。 |
| 服务/设置 | Account 导入、SettingService 双方字段、SettingsView DTO 字段按定义并集处理。sessionMemoryCache 接替 sync.Map 后检查 build 的读写、失效和并发约束。 |
| 路由与计费 | build 的 Fast、thinking、SSE 折叠、搜索工具与 main 最终 effort 校验、冷却策略、cache TTL、Grok 响应模型审计协同。实际出站参数决定最终日志和计费。 |
| 前端（4 个冲突文件） | EditAccountModal 保留领域 helper 与 main 图片/配额新字段；ChannelsView 的领域 helper 吸收平台白名单、禁用显式 false 和服务端更新维度。locale 同时复核主对象和最终 override 文案。 |

## 已确认范围内的兼容细节

- 分组 effort 策略先消费 `none`；管理员显式映射后的有效档位保留。最终仍关闭的兼容供应商请求再转为 `thinking.type=disabled` 并删除互斥 effort。这样同时保留 main 分组映射/拒绝和 build 关闭推理契约。
- Messages→Chat 始终使用 SSE，main 新增测试的上游 fixture 按此契约适配；下游非流式仍断言完整 JSON 正文及实际计费档位。
- Responses message 的 annotations 按 main 线上协议始终为数组，build 有引用时保留条目，无引用时为 `[]`。
- 在途预留内存测试采用 main 已有的子进程隔离模式，保持 20,000 个随机模型与 8 MiB 上限，排除其它 service 测试后台任务对进程级 HeapAlloc 的干扰。

- 阿里云验证码本地 SDK fixture 清空大小写 HTTP/HTTPS 代理环境变量，并通过持续占有端口的主动断流模拟传输错误，避免代理把 EOF 转换为 502 及端口复用干扰。生产错误归一逻辑沿用 main。

## 自动合并区域

research/merge-preview.md 按精确路径保存 68 个自动合并交集的复核清单。重点涵盖 Ent/usage_log、账户与设置 DTO、handler 路由、模型目录、WS、计费、账号弹窗、导出配置、locale 展开及设置页。双方各自新增文件按入口可达性、注册、类型和测试链复核。新增 migration 同号文件按现有迁移器真实排序与校验规则审查；不单凭编号判断冲突。

## 镜像构建一致性

main 已把 go.mod、根 Dockerfile 和 CI 提升到 Go 1.27.3。另两套 backend/deploy Dockerfile 仍为 1.27.2，本轮机械同步到目标版本，并同步 DEV_GUIDE 的版本事实。

## 验证与兼容

本机默认 Go 1.22.2；目标 go.mod 要求 1.27.3。优先寻找现有目标工具链，缺失时在用户目录准备 Go 工具链并验证版本。pnpm 已为 10.23.0，按合并后的 lockfile 安装前端依赖。先执行受影响包，再执行全量 unit、前端测试、构建、lint；外部依赖测试根据现有隔离 harness 执行并记录边界。

## 风险与回退

静态成功不等于运行时完整。cyber 身份与模型策略属于高风险兼容点；两侧冲突测试要结合各自行为重建，不能机械拼接。迁移和认证新增能力进行跨层审查，不在生产运行。合并提交前保留 HEAD 与目标哈希。需要取消 merge 时先保存本轮修改与 Flower 升级补丁，再按实际状态采用 merge --abort，核对原始工作区文件。提交/推送由独立精确 Git 计划控制。
