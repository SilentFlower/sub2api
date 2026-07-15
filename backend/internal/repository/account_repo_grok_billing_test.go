package repository

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGrokBillingSnapshotsAreSchedulerNeutral(t *testing.T) {
	t.Parallel()

	for _, key := range []string{"grok_billing_snapshot", "grok_billing_quota_snapshot"} {
		require.True(t, isSchedulerNeutralExtraKey(key), "观测型 Billing 快照不应触发调度重建: %s", key)
		require.False(t, shouldEnqueueSchedulerOutboxForExtraUpdates(map[string]any{
			key: map[string]any{"usage_percent": 50},
		}), "观测型 Billing 快照不应进入 scheduler outbox: %s", key)
	}
}
