export default {
  deepSeekMissingReasoningAutoDowngrade: '自动降级不完整的 DeepSeek 推理历史',
  deepSeekMissingReasoningAutoDowngradeHint: '默认开启。原生 Chat、Responses 转 Chat 和 Anthropic 转 Chat 请求最终发送到 DeepSeek 时，若 assistant 工具调用历史缺少推理内容，则自动关闭 thinking。关闭后可能恢复上游 400 错误。'
}
