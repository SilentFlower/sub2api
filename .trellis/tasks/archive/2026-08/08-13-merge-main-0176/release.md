# Release Operations

## Conclusion

Release operations exist. Deploying `build` after merge commit `5a2179828` applies
the embedded database migration `221_group_model_pricing.sql` during service startup.

## Evidence Checked

- `task.json`, `prd.md`, `design.md`, `implement.md`
- `implement.jsonl`, `check.jsonl`
- merge commit `5a2179828` and its changed-file set
- `backend/migrations/221_group_model_pricing.sql`
- `backend/migrations/migrations.go`
- `backend/internal/repository/migrations_runner.go`
- `.trellis/spec/backend/database-guidelines.md`

## Drift Check

Missing `release.md`; this file records the database migration introduced by the
merged main release. No configuration, batch job, data repair, or external-system
operation was found in the task evidence.

## SQL Changes

- `backend/migrations/221_group_model_pricing.sql` adds
  `groups.long_context_pricing_enabled BOOLEAN NOT NULL DEFAULT TRUE` and
  `groups.model_pricing JSONB`.
- The migration normalizes existing rows so `long_context_pricing_enabled=TRUE`,
  preserving the previous long-context billing behavior.
- The SQL is embedded in the server binary and applied automatically by the
  migration runner under the existing PostgreSQL advisory lock. Do not execute a
  second manual copy of the migration.

## Configuration Changes

None.

## Batch / Deployment Scripts / Data Repair

None. The existing-row update is part of the transactional migration.

## External Systems / Dependent Platforms

None.

## Release Order

1. Back up the PostgreSQL database using the normal release procedure.
2. Deploy the server containing `0.1.176`; allow one instance to complete the
   startup migration before treating the rollout as healthy.
3. Deploy or refresh the frontend after the backend is healthy.

## Rollback Notes

- Rolling the application code back does not remove the two new nullable/defaulted
  schema capabilities; the added columns are compatible with the previous build.
- Do not modify or delete migration `221` after it has been applied. If schema
  reversal is required, create a new forward-only migration after review.
- The pre-merge code reference remains
  `refs/backup/build-before-main-0176-20260814-000906-7a6ab3280` for code comparison.

## Post-release Verification

- Confirm `schema_migrations` records `221_group_model_pricing.sql` with no checksum
  or startup error.
- Confirm both new `groups` columns exist and existing rows have
  `long_context_pricing_enabled=TRUE`.
- Verify group create/edit APIs can round-trip model pricing overrides.
- Smoke-test Grok account usage refresh and confirm the removed `/billing-quota`
  endpoint and independent quota UI are absent.
- Verify representative build-only features listed in the task regression audit,
  especially Codex identity/fingerprint, Responses Lite, Web Search, image routing,
  and Grok force-chat.
