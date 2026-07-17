export default {
  codexCLIOnlyCustomUA: 'Custom allowed UA prefixes',
  codexCLIOnlyCustomUADesc:
    'Only takes effect when the switch above is on. Enter one User-Agent pattern per line; * is supported, and any matching line is allowed.',
  codexCLIOnlyCustomUABulkDesc:
    'Bulk overwrite custom User-Agent allow rules for the selected OpenAI OAuth accounts.',
  codexCLIOnlyCustomUABulkHint:
    'When checked, one pattern per line is submitted. Empty content clears custom UA rules on the selected accounts.',
  codexCLIOnlyCustomUAPlaceholder: 'my-client/*\ncustom-codex-wrapper/*'
}
