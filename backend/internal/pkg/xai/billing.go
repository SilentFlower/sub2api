package xai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
)

const (
	billingPeriodWeekly  = "weekly"
	billingPeriodMonthly = "monthly"
	billingPeriodUnknown = "unknown"

	billingPlanSuperGrok      = "supergrok"
	billingPlanSuperGrokHeavy = "supergrok_heavy"

	superGrokLimitCents      int64 = 15_000
	superGrokHeavyLimitCents int64 = 150_000
)

// BillingCent 表示 Grok billing 响应中的 cents 包装值。
type BillingCent struct {
	Val any `json:"val,omitempty"`
}

// BillingPeriod 表示 Grok billing 响应中的计费周期。
type BillingPeriod struct {
	Type  string `json:"type,omitempty"`
	Start string `json:"start,omitempty"`
	End   string `json:"end,omitempty"`
}

// BillingProductUsage 表示 Grok billing 响应中的产品用量项。
type BillingProductUsage struct {
	Product           string `json:"product,omitempty"`
	UsagePercent      any    `json:"usagePercent,omitempty"`
	UsagePercentSnake any    `json:"usage_percent,omitempty"`
}

// BillingConfig 表示 Grok CLI billing 响应的 config 节点。
type BillingConfig struct {
	CurrentPeriod       *BillingPeriod        `json:"currentPeriod,omitempty"`
	CurrentPeriodSnake  *BillingPeriod        `json:"current_period,omitempty"`
	CreditUsagePercent  any                   `json:"creditUsagePercent,omitempty"`
	CreditUsagePctSnake any                   `json:"credit_usage_percent,omitempty"`
	ProductUsage        []BillingProductUsage `json:"productUsage,omitempty"`
	ProductUsageSnake   []BillingProductUsage `json:"product_usage,omitempty"`
	MonthlyLimit        any                   `json:"monthlyLimit,omitempty"`
	MonthlyLimitSnake   any                   `json:"monthly_limit,omitempty"`
	Used                any                   `json:"used,omitempty"`
	OnDemandCap         any                   `json:"onDemandCap,omitempty"`
	OnDemandCapSnake    any                   `json:"on_demand_cap,omitempty"`
	OnDemandUsed        any                   `json:"onDemandUsed,omitempty"`
	OnDemandUsedSnake   any                   `json:"on_demand_used,omitempty"`
	BillingPeriodStart  string                `json:"billingPeriodStart,omitempty"`
	BillingStartSnake   string                `json:"billing_period_start,omitempty"`
	BillingPeriodEnd    string                `json:"billingPeriodEnd,omitempty"`
	BillingEndSnake     string                `json:"billing_period_end,omitempty"`
}

// BillingPayload 表示 Grok CLI billing 响应体。
type BillingPayload struct {
	Config *BillingConfig `json:"config,omitempty"`
}

// BillingProductUsageSummary 是前端展示用的产品用量摘要。
type BillingProductUsageSummary struct {
	Product      string   `json:"product"`
	UsagePercent *float64 `json:"usage_percent,omitempty"`
}

// BillingSnapshot 是保存到 accounts.extra.grok_billing_snapshot 的非敏感展示快照。
type BillingSnapshot struct {
	PeriodType             string                       `json:"period_type,omitempty"`
	WeeklyUsedPercent      *float64                     `json:"weekly_used_percent,omitempty"`
	WeeklyResetAt          string                       `json:"weekly_reset_at,omitempty"`
	ProductUsage           []BillingProductUsageSummary `json:"product_usage,omitempty"`
	MonthlyLimitCents      *int64                       `json:"monthly_limit_cents,omitempty"`
	MonthlyUsedCents       *int64                       `json:"monthly_used_cents,omitempty"`
	MonthlyRemainingCents  *int64                       `json:"monthly_remaining_cents,omitempty"`
	MonthlyUsedPercent     *float64                     `json:"monthly_used_percent,omitempty"`
	BillingPeriodStart     string                       `json:"billing_period_start,omitempty"`
	BillingPeriodEnd       string                       `json:"billing_period_end,omitempty"`
	OnDemandCapCents       *int64                       `json:"on_demand_cap_cents,omitempty"`
	OnDemandUsedCents      *int64                       `json:"on_demand_used_cents,omitempty"`
	OnDemandRemainingCents *int64                       `json:"on_demand_remaining_cents,omitempty"`
	OnDemandUsedPercent    *float64                     `json:"on_demand_used_percent,omitempty"`
	PlanLabel              string                       `json:"plan_label,omitempty"`
	UpdatedAt              string                       `json:"updated_at"`
	Stale                  bool                         `json:"stale"`
}

type billingSummary struct {
	periodType          string
	usagePercent        *float64
	periodStart         string
	periodEnd           string
	productUsage        []BillingProductUsageSummary
	monthlyLimitCents   *int64
	usedCents           *int64
	includedUsedCents   *int64
	onDemandCapCents    *int64
	onDemandUsedCents   *int64
	onDemandUsedPercent *float64
	billingPeriodStart  string
	billingPeriodEnd    string
	usedPercent         *float64
}

// BuildBillingURL 构造 Grok CLI billing 接口地址。
func BuildBillingURL(baseURL string, formatCredits bool) (string, error) {
	validatedBaseURL, err := ValidatedBaseURL(baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid base url: %w", err)
	}
	if formatCredits {
		return validatedBaseURL + "/billing?format=credits", nil
	}
	return validatedBaseURL + "/billing", nil
}

// ParseBillingPayload 解析 Grok CLI billing 响应体。
func ParseBillingPayload(data []byte) (*BillingPayload, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, fmt.Errorf("empty billing payload")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var payload BillingPayload
	if err := decoder.Decode(&payload); err != nil {
		return nil, err
	}
	return &payload, nil
}

// BuildBillingSnapshot 合并 weekly/monthly 两个 Grok CLI billing 响应为展示快照。
func BuildBillingSnapshot(weeklyPayload, monthlyPayload *BillingPayload, now time.Time) *BillingSnapshot {
	weeklySummary := buildBillingSummary(billingConfigFromPayload(weeklyPayload))
	monthlySummary := buildBillingSummary(billingConfigFromPayload(monthlyPayload))
	merged := mergeBillingSummaries(weeklySummary, monthlySummary)
	if merged == nil {
		return nil
	}

	updatedAt := now.UTC().Format(time.RFC3339)
	snapshot := &BillingSnapshot{
		PeriodType:             merged.periodType,
		ProductUsage:           merged.productUsage,
		MonthlyLimitCents:      merged.monthlyLimitCents,
		MonthlyUsedCents:       merged.includedUsedCents,
		MonthlyRemainingCents:  remainingCents(merged.monthlyLimitCents, merged.includedUsedCents),
		MonthlyUsedPercent:     merged.usedPercent,
		BillingPeriodStart:     merged.billingPeriodStart,
		BillingPeriodEnd:       merged.billingPeriodEnd,
		OnDemandCapCents:       merged.onDemandCapCents,
		OnDemandUsedCents:      merged.onDemandUsedCents,
		OnDemandRemainingCents: remainingCents(merged.onDemandCapCents, merged.onDemandUsedCents),
		OnDemandUsedPercent:    merged.onDemandUsedPercent,
		PlanLabel:              resolveBillingPlanLabel(merged.monthlyLimitCents),
		UpdatedAt:              updatedAt,
	}
	if merged.periodType == billingPeriodWeekly {
		snapshot.WeeklyUsedPercent = merged.usagePercent
		snapshot.WeeklyResetAt = merged.periodEnd
	}
	return snapshot
}

// BillingSnapshotFromRaw 将 Extra 中的原始 JSON 值还原为 BillingSnapshot。
func BillingSnapshotFromRaw(raw any) (*BillingSnapshot, error) {
	if raw == nil {
		return nil, nil
	}
	switch snapshot := raw.(type) {
	case *BillingSnapshot:
		return snapshot, nil
	case BillingSnapshot:
		return &snapshot, nil
	default:
		data, err := json.Marshal(raw)
		if err != nil {
			return nil, fmt.Errorf("marshal grok billing snapshot: %w", err)
		}
		var out BillingSnapshot
		if err := json.Unmarshal(data, &out); err != nil {
			return nil, err
		}
		return &out, nil
	}
}

func billingConfigFromPayload(payload *BillingPayload) *BillingConfig {
	if payload == nil {
		return nil
	}
	return payload.Config
}

func buildBillingSummary(config *BillingConfig) *billingSummary {
	if config == nil {
		return nil
	}
	summary := &billingSummary{
		periodType:   billingPeriodUnknown,
		productUsage: make([]BillingProductUsageSummary, 0),
	}
	currentPeriod := coalesceBillingPeriod(config.CurrentPeriod, config.CurrentPeriodSnake)
	periodType := resolveBillingPeriodType(currentPeriod)
	creditUsagePercent := normalizeFloatPtr(firstNonNil(config.CreditUsagePercent, config.CreditUsagePctSnake))
	periodStart := firstNonEmpty(currentPeriodString(currentPeriod, "start"), config.BillingPeriodStart, config.BillingStartSnake)
	periodEnd := firstNonEmpty(currentPeriodString(currentPeriod, "end"), config.BillingPeriodEnd, config.BillingEndSnake)
	productUsage := normalizeBillingProductUsage(coalesceProductUsage(config.ProductUsage, config.ProductUsageSnake))

	monthlyLimitCents := normalizeCentPtr(firstNonNil(config.MonthlyLimit, config.MonthlyLimitSnake))
	usedCents := normalizeCentPtr(config.Used)
	onDemandCapCents := normalizeCentPtr(firstNonNil(config.OnDemandCap, config.OnDemandCapSnake))
	explicitOnDemandUsedCents := normalizeCentPtr(firstNonNil(config.OnDemandUsed, config.OnDemandUsedSnake))
	billingPeriodStart := firstNonEmpty(config.BillingPeriodStart, config.BillingStartSnake)
	billingPeriodEnd := firstNonEmpty(config.BillingPeriodEnd, config.BillingEndSnake)

	includedUsedCents := includedMonthlyUsedCents(usedCents, monthlyLimitCents)
	derivedOnDemandUsedCents := derivedOnDemandUsedCents(usedCents, monthlyLimitCents)
	onDemandUsedCents := explicitOnDemandUsedCents
	if onDemandUsedCents == nil {
		onDemandUsedCents = derivedOnDemandUsedCents
	}
	usedPercent := percentPtr(includedUsedCents, monthlyLimitCents)
	onDemandUsedPercent := percentPtr(onDemandUsedCents, onDemandCapCents)

	hasWeeklyData := creditUsagePercent != nil || periodType == billingPeriodWeekly || len(productUsage) > 0
	hasMonthlyData := monthlyLimitCents != nil ||
		usedCents != nil ||
		(!hasWeeklyData && (onDemandCapCents != nil || billingPeriodEnd != ""))
	if !hasWeeklyData && !hasMonthlyData {
		return nil
	}

	if hasWeeklyData {
		if periodType == billingPeriodUnknown {
			summary.periodType = billingPeriodWeekly
		} else {
			summary.periodType = periodType
		}
		summary.usagePercent = creditUsagePercent
		summary.periodStart = periodStart
		summary.periodEnd = periodEnd
	} else {
		summary.periodType = billingPeriodMonthly
		summary.usagePercent = usedPercent
		summary.periodStart = billingPeriodStart
		summary.periodEnd = billingPeriodEnd
	}
	summary.productUsage = productUsage
	summary.monthlyLimitCents = monthlyLimitCents
	summary.usedCents = usedCents
	summary.includedUsedCents = includedUsedCents
	summary.onDemandCapCents = onDemandCapCents
	summary.onDemandUsedCents = onDemandUsedCents
	summary.onDemandUsedPercent = onDemandUsedPercent
	if hasMonthlyData {
		summary.billingPeriodStart = billingPeriodStart
		summary.billingPeriodEnd = billingPeriodEnd
	}
	summary.usedPercent = usedPercent
	return summary
}

func mergeBillingSummaries(primary, fallback *billingSummary) *billingSummary {
	if primary == nil {
		return fallback
	}
	if fallback == nil {
		return primary
	}
	return &billingSummary{
		periodType:          firstKnownPeriod(primary.periodType, fallback.periodType),
		usagePercent:        coalesceFloatPtr(primary.usagePercent, fallback.usagePercent),
		periodStart:         firstNonEmpty(primary.periodStart, fallback.periodStart),
		periodEnd:           firstNonEmpty(primary.periodEnd, fallback.periodEnd),
		productUsage:        coalesceProductUsageSummary(primary.productUsage, fallback.productUsage),
		monthlyLimitCents:   coalesceInt64Ptr(primary.monthlyLimitCents, fallback.monthlyLimitCents),
		usedCents:           coalesceInt64Ptr(primary.usedCents, fallback.usedCents),
		includedUsedCents:   coalesceInt64Ptr(primary.includedUsedCents, fallback.includedUsedCents),
		onDemandCapCents:    coalesceInt64Ptr(primary.onDemandCapCents, fallback.onDemandCapCents),
		onDemandUsedCents:   coalesceInt64Ptr(primary.onDemandUsedCents, fallback.onDemandUsedCents),
		onDemandUsedPercent: coalesceFloatPtr(primary.onDemandUsedPercent, fallback.onDemandUsedPercent),
		billingPeriodStart:  firstNonEmpty(primary.billingPeriodStart, fallback.billingPeriodStart),
		billingPeriodEnd:    firstNonEmpty(primary.billingPeriodEnd, fallback.billingPeriodEnd),
		usedPercent:         coalesceFloatPtr(primary.usedPercent, fallback.usedPercent),
	}
}

func coalesceBillingPeriod(primary, fallback *BillingPeriod) *BillingPeriod {
	if primary != nil {
		return primary
	}
	return fallback
}

func coalesceProductUsage(primary, fallback []BillingProductUsage) []BillingProductUsage {
	if len(primary) > 0 {
		return primary
	}
	return fallback
}

func coalesceProductUsageSummary(primary, fallback []BillingProductUsageSummary) []BillingProductUsageSummary {
	if len(primary) > 0 {
		return primary
	}
	return fallback
}

func coalesceFloatPtr(primary, fallback *float64) *float64 {
	if primary != nil {
		return primary
	}
	return fallback
}

func coalesceInt64Ptr(primary, fallback *int64) *int64 {
	if primary != nil {
		return primary
	}
	return fallback
}

func firstKnownPeriod(primary, fallback string) string {
	if primary != "" && primary != billingPeriodUnknown {
		return primary
	}
	if fallback != "" {
		return fallback
	}
	return billingPeriodUnknown
}

func resolveBillingPeriodType(period *BillingPeriod) string {
	rawType := strings.ToLower(strings.TrimSpace(currentPeriodString(period, "type")))
	switch {
	case strings.Contains(rawType, billingPeriodWeekly):
		return billingPeriodWeekly
	case strings.Contains(rawType, billingPeriodMonthly):
		return billingPeriodMonthly
	default:
		return billingPeriodUnknown
	}
}

func currentPeriodString(period *BillingPeriod, field string) string {
	if period == nil {
		return ""
	}
	switch field {
	case "type":
		return strings.TrimSpace(period.Type)
	case "start":
		return strings.TrimSpace(period.Start)
	case "end":
		return strings.TrimSpace(period.End)
	default:
		return ""
	}
}

func normalizeBillingProductUsage(items []BillingProductUsage) []BillingProductUsageSummary {
	if len(items) == 0 {
		return nil
	}
	out := make([]BillingProductUsageSummary, 0, len(items))
	for idx, item := range items {
		product := strings.TrimSpace(item.Product)
		if product == "" {
			product = fmt.Sprintf("Product %d", idx+1)
		}
		out = append(out, BillingProductUsageSummary{
			Product:      product,
			UsagePercent: normalizeFloatPtr(firstNonNil(item.UsagePercent, item.UsagePercentSnake)),
		})
	}
	return out
}

func normalizeFloatPtr(value any) *float64 {
	switch v := value.(type) {
	case nil:
		return nil
	case float64:
		if !math.IsNaN(v) && !math.IsInf(v, 0) {
			return &v
		}
	case float32:
		f := float64(v)
		if !math.IsNaN(f) && !math.IsInf(f, 0) {
			return &f
		}
	case int:
		f := float64(v)
		return &f
	case int64:
		f := float64(v)
		return &f
	case json.Number:
		if f, err := v.Float64(); err == nil && !math.IsNaN(f) && !math.IsInf(f, 0) {
			return &f
		}
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return nil
		}
		var parsed json.Number = json.Number(trimmed)
		if f, err := parsed.Float64(); err == nil && !math.IsNaN(f) && !math.IsInf(f, 0) {
			return &f
		}
	}
	return nil
}

func normalizeCentPtr(value any) *int64 {
	switch v := value.(type) {
	case BillingCent:
		return normalizeCentPtr(v.Val)
	case *BillingCent:
		if v == nil {
			return nil
		}
		return normalizeCentPtr(v.Val)
	case map[string]any:
		return normalizeCentPtr(v["val"])
	case map[string]json.Number:
		return normalizeCentPtr(v["val"])
	}
	number := normalizeFloatPtr(value)
	if number == nil {
		return nil
	}
	rounded := int64(math.Round(*number))
	return &rounded
}

func includedMonthlyUsedCents(usedCents, monthlyLimitCents *int64) *int64 {
	if usedCents == nil {
		return nil
	}
	used := *usedCents
	if monthlyLimitCents != nil && *monthlyLimitCents > 0 && used > *monthlyLimitCents {
		used = *monthlyLimitCents
	}
	return &used
}

func derivedOnDemandUsedCents(usedCents, monthlyLimitCents *int64) *int64 {
	if usedCents == nil || monthlyLimitCents == nil {
		return nil
	}
	used := *usedCents - *monthlyLimitCents
	if used < 0 {
		used = 0
	}
	return &used
}

func percentPtr(used, limit *int64) *float64 {
	if used == nil || limit == nil || *limit <= 0 {
		return nil
	}
	percent := (float64(*used) / float64(*limit)) * 100
	return &percent
}

func remainingCents(limit, used *int64) *int64 {
	if limit == nil || used == nil {
		return nil
	}
	remaining := *limit - *used
	if remaining < 0 {
		remaining = 0
	}
	return &remaining
}

func resolveBillingPlanLabel(monthlyLimitCents *int64) string {
	if monthlyLimitCents == nil {
		return ""
	}
	switch *monthlyLimitCents {
	case superGrokLimitCents:
		return billingPlanSuperGrok
	case superGrokHeavyLimitCents:
		return billingPlanSuperGrokHeavy
	default:
		return ""
	}
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
