# Release Operations

## Conclusion

Release operations exist. The merged `main` 0.1.178 code introduces automatic PostgreSQL
migrations, startup-time data backfill behavior, and new optional gateway configuration.

## Evidence Checked

- `task.json`, `brief.md`, `prd.md`, `design.md`, `implement.md`
- `implement.jsonl`, `check.jsonl`
- Merge commit `c50a1633860e4445aa2b58b3a543cedac8caa9dd`
- `backend/internal/repository/ent.go` and `backend/internal/repository/migrations_runner.go`
- `backend/internal/config/config.go`
- Added migration files `222` through `226`

## Drift Check

`release.md` was missing. Git evidence confirms database and configuration changes that must
remain visible during deployment.

## SQL Changes

The service automatically applies the following embedded PostgreSQL migrations during startup,
under an advisory lock and a 10-minute migration context:

- `222_group_usage_daily_rollups.sql`: creates group daily rollup tables, state, functions, and
  `usage_logs` invalidation triggers.
- `223_group_usage_rollup_timezone.sql`: adds the rollup timezone and updates invalidation
  functions to use the configured database timezone.
- `224_user_platform_quotas_add_cn_providers.sql`: expands the platform quota constraint to
  Kimi, Zhipu, and DeepSeek.
- `225_backfill_codex_fingerprint_seed.sql`: backfills valid Codex fingerprint seeds for eligible
  enabled OpenAI OAuth accounts.
- `225_channel_model_time_pricing.sql`: adds `channel_model_pricing.time_pricing`.
- `226_channel_monitor_quota_mode.sql`: expands channel monitor providers, adds quota mode,
  account linkage, quota snapshots, indexes, constraints, and the default
  `channel_monitor_show_quota=false` setting.

Before deployment, ensure a recoverable database backup exists. After the first upgraded instance
starts, verify these filenames are recorded in `schema_migrations` before continuing the rollout.

## Configuration Changes

- Standard `TZ` now explicitly overrides the application `timezone` setting when non-empty.
- New optional environment-backed keys are available for pay-as-you-go Kimi, DeepSeek, and Zhipu
  account balance handling:
  - `GATEWAY_CN_PROVIDERS_BALANCE_CHECK_ENABLED` defaults to `true`.
  - `GATEWAY_CN_PROVIDERS_BALANCE_THRESHOLD` defaults to `0.5`.
  - `GATEWAY_CN_PROVIDERS_BALANCE_CHECK_INTERVAL_MINUTES` defaults to `10`.
- No mandatory configuration value is required because defaults are registered. Operators should
  review the threshold and interval against their account currencies and monitoring policy.
- Build and CI images now use Go `1.26.6`; rebuild application images from the merged source.

## Batch / Deployment Scripts / Data Repair

- The Codex fingerprint seed backfill runs as migration `225_backfill_codex_fingerprint_seed.sql`.
- Historical group usage daily rollups are populated by the background aggregation job after the
  schema migration. Monitor its progress and database load during the first deployment.
- No separate one-off command or repository deployment script must be run manually.

## External Systems / Dependent Platforms

No external platform configuration change is required. Real OpenAI, xAI, Kimi, Zhipu, and
DeepSeek requests were not sent during local verification, so deployment verification must use
valid configured accounts where those providers are enabled.

## Release Order

1. Create or verify a recoverable PostgreSQL backup.
2. Rebuild the application image from `build` and deploy one upgraded instance first.
3. Wait for startup migrations to finish and verify migrations `222` through `226` in
   `schema_migrations`.
4. Confirm the instance is healthy, then continue the rolling deployment.
5. Observe group usage rollup backfill, channel monitor quota behavior, and gateway errors before
   completing the rollout.

## Rollback Notes

Revert the merge commit or redeploy the previous application version without rewriting Git
history. Database migrations are not automatically reversed: the new tables, columns, functions,
triggers, constraints, settings, and fingerprint seed data should normally remain because the
changes are additive or compatibility-expanding. Any schema or data rollback requires a separately
reviewed SQL plan and a verified backup.

## Post-release Verification

- Verify application startup and all new migration records.
- Verify existing build-only Responses Lite, Web Search, image generation, custom Codex UA, and
  Grok/GLM compatibility paths.
- Verify main-side CN provider routing, channel monitor quota mode, time pricing, Codex fingerprint,
  and partial-usage billing.
- Verify Chinese and English locale screens for account creation/editing, settings, channels, and
  monitoring.
- Run real-provider smoke requests for enabled OpenAI/xAI/CN provider accounts and confirm protocol
  routing, quota reads, model mapping, billing, and error handling.
