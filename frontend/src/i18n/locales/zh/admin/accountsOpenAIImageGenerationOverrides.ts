export default {
  codexImageTool: 'Codex 图片桥接策略',
  codexImageToolDesc:
    '统一控制 Codex /responses 文本请求的 hosted image_generation 桥接和客户端图片工具声明。对于 Responses Lite 请求，hosted 工具自动注入按最终模型阻止列表决定，并非全局关闭。账号级策略优先于渠道和全局配置，不影响独立图片生成接口。',
  codexImageToolInherit: '跟随渠道',
  codexImageToolInheritDesc:
    '不写入账号覆盖；hosted 工具是否注入由渠道或全局策略以及 Responses Lite 最终模型阻止列表共同决定，客户端显式携带的 hosted 工具和本地 image_gen 声明照常放行。',
  codexImageToolEnabled: '启用 Hosted 桥接',
  codexImageToolEnabledDesc:
    '在遵循 Responses Lite 最终模型阻止列表的前提下注入 hosted image_generation 工具；客户端显式携带的图片工具仍会放行。',
  codexImageToolDisabled: '不注入 Hosted 工具',
  codexImageToolDisabledDesc:
    '不注入 hosted 工具；客户端显式携带的 hosted 工具和本地 image_gen 声明仍会放行。',
  codexImageToolBlock: '移除客户端图片工具',
  codexImageToolBlockDesc:
    '不通过桥接自动注入 hosted 工具，并移除客户端显式携带的 hosted image_generation 工具、本地 image_gen 声明及相关 tool_choice；image-only 模型路由不受影响。',
  codexImageToolBadgeInherit: '渠道策略',
  codexImageToolBadgeEnabled: 'Hosted 桥接已开启',
  codexImageToolBadgeDisabled: '不注入 Hosted 工具',
  codexImageToolBadgeBlock: '客户端图片工具已移除'
}
