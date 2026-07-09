package service

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
)

const grokQuotaSnapshotExtraKey = "grok_usage_snapshot"
const grokBillingSnapshotExtraKey = "grok_billing_snapshot"

const grokBillingSnapshotTTL = 30 * time.Minute

type GrokQuotaFetcher struct{}

func NewGrokQuotaFetcher() *GrokQuotaFetcher {
	return &GrokQuotaFetcher{}
}

func (f *GrokQuotaFetcher) BuildUsageInfo(account *Account) *UsageInfo {
	now := time.Now()
	usage := &UsageInfo{
		Source:    "passive",
		UpdatedAt: &now,
	}
	if account == nil {
		usage.ErrorCode = "quota_unknown"
		usage.Error = "Grok quota is unknown until the first upstream response includes xAI rate-limit headers"
		return usage
	}

	snapshot, err := grokQuotaSnapshotFromExtra(account.Extra)
	if err != nil || snapshot == nil {
		usage.ErrorCode = "quota_unknown"
		usage.Error = "Grok quota is unknown until the first upstream response includes xAI rate-limit headers"
		attachGrokBillingQuota(usage, account, now)
		return usage
	}

	if parsedAt, err := time.Parse(time.RFC3339, snapshot.UpdatedAt); err == nil {
		usage.UpdatedAt = &parsedAt
	}
	usage.GrokRequestQuota = snapshot.Requests
	usage.GrokTokenQuota = snapshot.Tokens
	usage.GrokRetryAfterSeconds = snapshot.RetryAfterSeconds
	usage.SubscriptionTier = snapshot.SubscriptionTier
	usage.SubscriptionTierRaw = snapshot.SubscriptionTier
	usage.GrokEntitlementStatus = snapshot.EntitlementStatus
	usage.GrokLastQuotaProbeAt = snapshot.LastProbeAt
	usage.GrokLastHeadersSeenAt = snapshot.LastHeadersSeenAt
	usage.GrokLastStatusCode = snapshot.StatusCode
	if snapshot.HasObservedHeaders() {
		usage.GrokQuotaSnapshotState = "observed"
	} else {
		usage.GrokQuotaSnapshotState = "no_headers"
		usage.ErrorCode = "quota_unknown"
		usage.Error = "No xAI quota headers observed on the latest Grok probe"
	}

	switch snapshot.StatusCode {
	case 401:
		usage.NeedsReauth = true
		usage.ErrorCode = "unauthenticated"
	case 403:
		usage.IsForbidden = true
		usage.ForbiddenType = "forbidden"
		usage.ErrorCode = "forbidden"
		if usage.GrokEntitlementStatus == "" {
			usage.GrokEntitlementStatus = "forbidden"
		}
	case 429:
		usage.ErrorCode = "rate_limited"
	}
	attachGrokBillingQuota(usage, account, now)
	return usage
}

func grokQuotaSnapshotFromExtra(extra map[string]any) (*xai.QuotaSnapshot, error) {
	if extra == nil {
		return nil, nil
	}
	raw, ok := extra[grokQuotaSnapshotExtraKey]
	if !ok || raw == nil {
		return nil, nil
	}
	switch snapshot := raw.(type) {
	case *xai.QuotaSnapshot:
		return snapshot, nil
	case xai.QuotaSnapshot:
		return &snapshot, nil
	case map[string]any:
		data, err := json.Marshal(snapshot)
		if err != nil {
			return nil, err
		}
		var out xai.QuotaSnapshot
		if err := json.Unmarshal(data, &out); err != nil {
			return nil, err
		}
		return &out, nil
	default:
		data, err := json.Marshal(raw)
		if err != nil {
			return nil, fmt.Errorf("marshal grok quota snapshot: %w", err)
		}
		var out xai.QuotaSnapshot
		if err := json.Unmarshal(data, &out); err != nil {
			return nil, err
		}
		return &out, nil
	}
}

func grokBillingSnapshotFromExtra(extra map[string]any) (*xai.BillingSnapshot, error) {
	if extra == nil {
		return nil, nil
	}
	raw, ok := extra[grokBillingSnapshotExtraKey]
	if !ok || raw == nil {
		return nil, nil
	}
	return xai.BillingSnapshotFromRaw(raw)
}

func markGrokBillingSnapshotStale(snapshot *xai.BillingSnapshot, now time.Time) {
	if snapshot == nil {
		return
	}
	ts, err := time.Parse(time.RFC3339, snapshot.UpdatedAt)
	if err != nil {
		snapshot.Stale = true
		return
	}
	snapshot.Stale = now.Sub(ts) >= grokBillingSnapshotTTL
}

func attachGrokBillingQuota(usage *UsageInfo, account *Account, now time.Time) {
	if usage == nil || account == nil {
		return
	}
	if billingSnapshot, err := grokBillingSnapshotFromExtra(account.Extra); err == nil && billingSnapshot != nil {
		markGrokBillingSnapshotStale(billingSnapshot, now)
		usage.GrokBillingQuota = billingSnapshot
	}
}
