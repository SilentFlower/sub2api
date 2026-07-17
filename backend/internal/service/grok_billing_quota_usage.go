package service

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai/billingquota"
)

const grokBillingQuotaSnapshotTTL = 30 * time.Minute

func grokBillingQuotaSnapshotFromExtra(extra map[string]any) (*billingquota.Snapshot, error) {
	if extra == nil {
		return nil, nil
	}
	raw, ok := extra[grokBillingQuotaSnapshotExtraKey]
	if !ok || raw == nil {
		return nil, nil
	}
	return billingquota.SnapshotFromRaw(raw)
}

func attachGrokBillingQuota(usage *UsageInfo, account *Account, now time.Time) {
	if usage == nil || account == nil {
		return
	}
	snapshot, err := grokBillingQuotaSnapshotFromExtra(account.Extra)
	if err != nil || snapshot == nil {
		return
	}
	updatedAt, err := time.Parse(time.RFC3339, snapshot.UpdatedAt)
	snapshot.Stale = err != nil || now.Sub(updatedAt) >= grokBillingQuotaSnapshotTTL
	usage.GrokBillingQuota = snapshot
}
