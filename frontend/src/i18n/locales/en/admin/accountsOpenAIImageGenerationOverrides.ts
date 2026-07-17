export default {
  codexImageTool: 'Codex image bridge policy',
  codexImageToolDesc:
    'Controls the hosted image_generation bridge and client-declared image tools on Codex /responses text requests. For Responses Lite requests, hosted auto-injection follows the final-model blocked-model list instead of being disabled globally. Account policy takes precedence over channel and global settings; standalone image-generation endpoints are unaffected.',
  codexImageToolInherit: 'Follow channel',
  codexImageToolInheritDesc:
    'No account override; hosted injection follows the channel or global policy and the Responses Lite final-model blocked-model list, while client-provided hosted tools and local image_gen declarations pass through.',
  codexImageToolEnabled: 'Enable hosted bridge',
  codexImageToolEnabledDesc:
    'Inject the hosted image_generation tool subject to the Responses Lite final-model blocked-model list; client-provided image tools still pass through.',
  codexImageToolDisabled: 'No hosted injection',
  codexImageToolDisabledDesc:
    'Do not inject the hosted tool; client-provided hosted tools and local image_gen declarations still pass through.',
  codexImageToolBlock: 'Strip client image tools',
  codexImageToolBlockDesc:
    'Do not auto-inject through the bridge, and remove client-provided hosted image_generation tools, local image_gen declarations, and matching tool_choice. Image-only model routing remains unaffected.',
  codexImageToolBadgeInherit: 'Channel policy',
  codexImageToolBadgeEnabled: 'Hosted bridge on',
  codexImageToolBadgeDisabled: 'No hosted injection',
  codexImageToolBadgeBlock: 'Client image tools stripped'
}
