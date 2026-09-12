export default {
  alphaSearchViaResponses: 'Alpha Search 经上游 Responses 执行',
  alphaSearchViaResponsesDesc:
    'Codex Responses Lite / code mode 的联网搜索会独立调用 /v1/alpha/search。开启后网关把该请求翻译成带 web_search 工具的 Responses 请求交给本账号上游执行；上游未真正搜索时自动改用本地 Web Search Emulation 供应商（仅覆盖 search_query）。关闭时该端点原样透传到上游。'
}
