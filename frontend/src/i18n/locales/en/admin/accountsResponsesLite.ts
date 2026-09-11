export default {
  responsesLiteDowngrade: 'Responses Lite downgrade',
  responsesLiteDowngradeDesc:
    'When Codex sends a Responses Lite request and this account forwards to a native Responses upstream, lift additional_tools into standard tools and strip Lite-only fields and headers before forwarding, so upstreams such as DeepSeek that do not understand Lite still receive every tool. The Chat Completions fallback handles Lite without this switch.'
}
