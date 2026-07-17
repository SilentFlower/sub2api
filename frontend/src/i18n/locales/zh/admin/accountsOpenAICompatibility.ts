export default {
  jsonSchemaDowngrade: 'JSON Schema 兼容模式',
  jsonSchemaDowngradeDesc:
    '上游不支持 json_schema 时，将格式改为 json_object，并把原 Schema 作为尽力遵循的输出约束；不保证 strict Schema，也不会修改工具参数 Schema。'
}
