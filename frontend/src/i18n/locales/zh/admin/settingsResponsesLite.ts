export default {
  openaiResponsesLiteBlockedModels: 'Responses Lite Header 阻止模型',
  openaiResponsesLiteBlockedModelsHint: '按最终上游模型决定是否移除 Responses Lite Header 与 WebSocket metadata。支持精确模型名和仅末尾 * 的前缀规则；空列表表示全部允许透传。',
  openaiResponsesLiteBlockedModelPlaceholder: '例如 gpt-5.4 或 gpt-5.4*',
  openaiResponsesLiteBlockedModelAdd: '添加模型规则',
  openaiResponsesLiteBlockedModelRemove: '删除模型规则',
  openaiResponsesLiteBlockedModelEmpty: 'Responses Lite 阻止模型规则不能为空。',
  openaiResponsesLiteBlockedModelWildcardInvalid: 'Responses Lite 阻止模型规则仅支持一个位于末尾的 *。'
}
