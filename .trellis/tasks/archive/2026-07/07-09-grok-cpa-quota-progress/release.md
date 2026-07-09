# Release Operations

## Conclusion
Release operations exist.

## Evidence Checked
- task.json
- prd.md
- design.md / implement.md / implement.jsonl / check.jsonl
- release.md: missing before finish-work
- git commits / changed files:
  - 57e409da feat(grok): 新增套餐额度进度条
  - cbd34d3a fix(grok): 优化套餐额度摘要展示
  - a328495d fix(grok): 对齐套餐额度进度条展示

## Drift Check
Missing release.md. Release notes were inferred from task requirements and committed file lists.

## SQL Changes
None. No database migration or schema change was added.

## Configuration Changes
- No new environment variables or feature flags.
- Deployment egress / proxy policy must allow Grok OAuth accounts to request:
  - `https://cli-chat-proxy.grok.com/v1/billing?format=credits`
  - `https://cli-chat-proxy.grok.com/v1/billing`

## Batch / Deployment Scripts / Data Repair
None. No one-off command, scheduled job, or data repair is required.

## External Systems / Dependent Platforms
- Depends on Grok CLI Billing endpoints being reachable and accepting the existing Grok OAuth access token flow.
- The feature stores successful results in `accounts.extra.grok_billing_snapshot`; no backfill is required because rows lazy-refresh or refresh manually.

## Release Order
Deploy backend before or together with frontend. The frontend calls `GET /api/v1/admin/grok/accounts/:id/billing-quota`, so an older backend will not support the new refresh action.

## Rollback Notes
Rollback code only. The optional `extra.grok_billing_snapshot` cache can remain in account extra data because existing Grok request/token quota logic ignores it.

## Post-release Verification
- Open the admin account list with a Grok OAuth account.
- Confirm existing request/Token quota rows still render from `grok_usage_snapshot`.
- Confirm “Grok 套餐额度” shows month/week compact progress rows and can expand to details.
- Click refresh and confirm the billing endpoint updates or returns a sanitized error without affecting existing Grok quota display.
