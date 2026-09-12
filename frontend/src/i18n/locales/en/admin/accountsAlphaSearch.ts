export default {
  alphaSearchViaResponses: 'Alpha Search via upstream Responses',
  alphaSearchViaResponsesDesc:
    'Codex Responses Lite / code mode runs web search through a standalone /v1/alpha/search call. When enabled, the gateway translates that call into a Responses request with the web_search tool and lets this account\'s upstream execute it; if the upstream did not actually search, it falls back to the local Web Search Emulation providers (search_query only). When disabled, the endpoint is proxied to the upstream as-is.'
}
