export default {
  jsonSchemaDowngrade: 'JSON Schema compatibility mode',
  jsonSchemaDowngradeDesc:
    'For upstreams without json_schema support, sends json_object and keeps the original Schema as a best-effort output constraint. Strict Schema enforcement is not guaranteed, and tool parameter Schemas are unchanged.'
}
