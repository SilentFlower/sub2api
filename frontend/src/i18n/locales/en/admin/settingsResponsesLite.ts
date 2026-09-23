export default {
  openaiResponsesLiteBlockedModels: 'Responses Lite Header blocked models',
  openaiResponsesLiteBlockedModelsHint: 'Remove the Responses Lite Header and WebSocket metadata according to the final upstream model. Supports exact names and prefix rules with one trailing *; an empty list adds no model restrictions. OpenAI OAuth / SetupToken accounts using GPT-5.5 always have Lite markers removed, regardless of this list.',
  openaiResponsesLiteBlockedModelPlaceholder: 'For example, gpt-5.4 or gpt-5.4*',
  openaiResponsesLiteBlockedModelAdd: 'Add model rule',
  openaiResponsesLiteBlockedModelRemove: 'Remove model rule',
  openaiResponsesLiteBlockedModelEmpty: 'A Responses Lite blocked-model rule cannot be empty.',
  openaiResponsesLiteBlockedModelWildcardInvalid: 'A Responses Lite blocked-model rule supports only one trailing *.'
}
