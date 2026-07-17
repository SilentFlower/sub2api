export default {
  codexImageTool: 'Codex 图片工具',
  codexImageToolDesc:
    '统一控制 Codex /responses 文本请求的 image_generation 图片工具：是否自动注入，以及客户端自带该工具时是否放行。对于 Responses Lite 请求，自动注入按最终模型阻止列表决定，并非全局关闭。账号级策略优先于渠道和全局配置，不影响独立图片生成接口。',
  codexImageToolInherit: '跟随渠道',
  codexImageToolInheritDesc:
    '不写入账号覆盖，是否注入由渠道或全局策略以及 Responses Lite 最终模型阻止列表共同决定；客户端自带的图片工具照常放行。',
  codexImageToolEnabled: '强制注入',
  codexImageToolEnabledDesc:
    '为 Codex /responses 请求注入图片工具，但仍遵循 Responses Lite 最终模型阻止列表。',
  codexImageToolDisabled: '关闭注入',
  codexImageToolDisabledDesc: '不自动注入；客户端自带的图片工具仍会放行。',
  codexImageToolBlock: '完全阻断',
  codexImageToolBlockDesc: '不注入，并移除客户端自带的图片工具及指向它的 tool_choice。',
  codexImageToolBadgeInherit: '渠道策略',
  codexImageToolBadgeEnabled: '强制注入',
  codexImageToolBadgeDisabled: '关闭注入',
  codexImageToolBadgeBlock: '完全阻断'
}
