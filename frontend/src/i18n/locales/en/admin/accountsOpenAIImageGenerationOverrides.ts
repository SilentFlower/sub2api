export default {
  codexImageTool: 'Codex image tool',
  codexImageToolDesc:
    'One policy for the image_generation tool on Codex /responses text requests: whether it is auto-injected, and whether client-provided tools pass through. For Responses Lite requests, auto-injection follows the final-model blocked-model list instead of being disabled globally. Account policy takes precedence over channel and global settings; standalone image-generation endpoints are unaffected.',
  codexImageToolInherit: 'Follow channel',
  codexImageToolInheritDesc:
    'No account override; injection follows the channel or global policy and the Responses Lite final-model blocked-model list, while client-provided image tools pass through.',
  codexImageToolEnabled: 'Force inject',
  codexImageToolEnabledDesc:
    'Inject the image tool for Codex /responses requests, subject to the Responses Lite final-model blocked-model list.',
  codexImageToolDisabled: 'No injection',
  codexImageToolDisabledDesc: 'Never auto-inject; client-provided image tools still pass through.',
  codexImageToolBlock: 'Block all',
  codexImageToolBlockDesc:
    'No injection, and client-provided image tools plus matching tool_choice are removed.',
  codexImageToolBadgeInherit: 'Channel policy',
  codexImageToolBadgeEnabled: 'Force inject',
  codexImageToolBadgeDisabled: 'No injection',
  codexImageToolBadgeBlock: 'Blocked'
}
