package billingquota

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
)

const (
	periodWeekly  = "weekly"
	periodMonthly = "monthly"
	periodUnknown = "unknown"

	planSuperGrok      = "supergrok"
	planSuperGrokHeavy = "supergrok_heavy"

	superGrokLimitCents      int64 = 15_000
	superGrokHeavyLimitCents int64 = 150_000

	cliClientVersion = "0.2.93"
	cliUserAgent     = "grok-pager/" + cliClientVersion + " grok-shell/" + cliClientVersion + " (macos; aarch64)"
)

// Cent 表示 Grok Billing 响应中的 cents 包装值。
type Cent struct {
	Val any `json:"val,omitempty"`
}

// Period 表示 Grok Billing 响应中的计费周期。
type Period struct {
	Type  string `json:"type,omitempty"`
	Start string `json:"start,omitempty"`
	End   string `json:"end,omitempty"`
}

// ProductUsage 表示 Grok Billing 响应中的产品用量项。
type ProductUsage struct {
	Product           string `json:"product,omitempty"`
	UsagePercent      any    `json:"usagePercent,omitempty"`
	UsagePercentSnake any    `json:"usage_percent,omitempty"`
}

// Config 表示 Grok CLI Billing 响应中的 config 节点。
type Config struct {
	CurrentPeriod       *Period        `json:"currentPeriod,omitempty"`
	CurrentPeriodSnake  *Period        `json:"current_period,omitempty"`
	CreditUsagePercent  any            `json:"creditUsagePercent,omitempty"`
	CreditUsagePctSnake any            `json:"credit_usage_percent,omitempty"`
	ProductUsage        []ProductUsage `json:"productUsage,omitempty"`
	ProductUsageSnake   []ProductUsage `json:"product_usage,omitempty"`
	MonthlyLimit        any            `json:"monthlyLimit,omitempty"`
	MonthlyLimitSnake   any            `json:"monthly_limit,omitempty"`
	Used                any            `json:"used,omitempty"`
	OnDemandCap         any            `json:"onDemandCap,omitempty"`
	OnDemandCapSnake    any            `json:"on_demand_cap,omitempty"`
	OnDemandUsed        any            `json:"onDemandUsed,omitempty"`
	OnDemandUsedSnake   any            `json:"on_demand_used,omitempty"`
	BillingPeriodStart  string         `json:"billingPeriodStart,omitempty"`
	BillingStartSnake   string         `json:"billing_period_start,omitempty"`
	BillingPeriodEnd    string         `json:"billingPeriodEnd,omitempty"`
	BillingEndSnake     string         `json:"billing_period_end,omitempty"`
}

// Payload 表示 Grok CLI Billing 的顶层响应体。
type Payload struct {
	Config *Config `json:"config,omitempty"`
}

// ProductUsageSummary 表示可安全返回给管理端的产品用量摘要。
type ProductUsageSummary struct {
	Product      string   `json:"product"`
	UsagePercent *float64 `json:"usage_percent,omitempty"`
}

// Snapshot 表示独立保存的 Grok 套餐额度快照。
type Snapshot struct {
	PeriodType             string                `json:"period_type,omitempty"`
	WeeklyUsedPercent      *float64              `json:"weekly_used_percent,omitempty"`
	WeeklyResetAt          string                `json:"weekly_reset_at,omitempty"`
	ProductUsage           []ProductUsageSummary `json:"product_usage,omitempty"`
	MonthlyLimitCents      *int64                `json:"monthly_limit_cents,omitempty"`
	MonthlyUsedCents       *int64                `json:"monthly_used_cents,omitempty"`
	MonthlyRemainingCents  *int64                `json:"monthly_remaining_cents,omitempty"`
	MonthlyUsedPercent     *float64              `json:"monthly_used_percent,omitempty"`
	BillingPeriodStart     string                `json:"billing_period_start,omitempty"`
	BillingPeriodEnd       string                `json:"billing_period_end,omitempty"`
	OnDemandCapCents       *int64                `json:"on_demand_cap_cents,omitempty"`
	OnDemandUsedCents      *int64                `json:"on_demand_used_cents,omitempty"`
	OnDemandRemainingCents *int64                `json:"on_demand_remaining_cents,omitempty"`
	OnDemandUsedPercent    *float64              `json:"on_demand_used_percent,omitempty"`
	PlanLabel              string                `json:"plan_label,omitempty"`
	UpdatedAt              string                `json:"updated_at"`
	Stale                  bool                  `json:"stale"`
	Partial                bool                  `json:"partial,omitempty"`
	FailedWindows          []string              `json:"failed_windows,omitempty"`
}

type summary struct {
	periodType          string
	usagePercent        *float64
	periodStart         string
	periodEnd           string
	productUsage        []ProductUsageSummary
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

// BuildURL 构造独立 Grok CLI Billing 接口地址。
// @param baseURL Grok CLI API 基础地址。
// @param weekly 是否构造 weekly credits 地址。
// @return 校验后的请求地址和构造错误。
func BuildURL(baseURL string, weekly bool) (string, error) {
	validatedBaseURL, err := xai.ValidatedBaseURL(baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid base url: %w", err)
	}
	if weekly {
		return validatedBaseURL + "/billing?format=credits", nil
	}
	return validatedBaseURL + "/billing", nil
}

// ApplyHeaders 设置独立 Grok CLI Billing 请求所需的鉴权和客户端标识头。
// @param req 待补充请求头的 HTTP 请求。
// @param accessToken Grok OAuth access token。
// @return 无返回值。
func ApplyHeaders(req *http.Request, accessToken string) {
	if req == nil {
		return
	}
	if token := strings.TrimSpace(accessToken); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("x-xai-token-auth", "xai-grok-cli")
	req.Header.Set("x-grok-client-version", cliClientVersion)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("User-Agent", cliUserAgent)
}

// ParsePayload 解析独立 Grok CLI Billing 响应体。
// @param data 上游 Billing JSON 响应体。
// @return 解析后的 Payload 和解析错误。
func ParsePayload(data []byte) (*Payload, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, fmt.Errorf("empty billing payload")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var payload Payload
	if err := decoder.Decode(&payload); err != nil {
		return nil, err
	}
	return &payload, nil
}

// BuildSnapshot 合并 weekly 和 monthly 响应为独立展示快照。
// @param weeklyPayload weekly credits 响应，可为 nil。
// @param monthlyPayload monthly Billing 响应，可为 nil。
// @param now 快照更新时间。
// @return 可展示快照；没有有效额度数据时返回 nil。
func BuildSnapshot(weeklyPayload, monthlyPayload *Payload, now time.Time) *Snapshot {
	weeklySummary := buildSummary(configFromPayload(weeklyPayload))
	monthlySummary := buildSummary(configFromPayload(monthlyPayload))
	merged := mergeSummaries(weeklySummary, monthlySummary)
	if merged == nil {
		return nil
	}

	snapshot := &Snapshot{
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
		PlanLabel:              resolvePlanLabel(merged.monthlyLimitCents),
		UpdatedAt:              now.UTC().Format(time.RFC3339),
	}
	if merged.periodType == periodWeekly {
		snapshot.WeeklyUsedPercent = merged.usagePercent
		snapshot.WeeklyResetAt = merged.periodEnd
	}
	return snapshot
}

// SnapshotFromRaw 将账号 extra 中的原始值还原为独立套餐额度快照。
// @param raw extra 中读取到的原始值。
// @return 还原后的快照和转换错误。
func SnapshotFromRaw(raw any) (*Snapshot, error) {
	if raw == nil {
		return nil, nil
	}
	switch snapshot := raw.(type) {
	case *Snapshot:
		return snapshot, nil
	case Snapshot:
		return &snapshot, nil
	default:
		data, err := json.Marshal(raw)
		if err != nil {
			return nil, fmt.Errorf("marshal grok billing quota snapshot: %w", err)
		}
		var out Snapshot
		if err := json.Unmarshal(data, &out); err != nil {
			return nil, err
		}
		return &out, nil
	}
}

func configFromPayload(payload *Payload) *Config {
	if payload == nil {
		return nil
	}
	return payload.Config
}

func buildSummary(config *Config) *summary {
	if config == nil {
		return nil
	}
	result := &summary{periodType: periodUnknown, productUsage: make([]ProductUsageSummary, 0)}
	currentPeriod := coalescePeriod(config.CurrentPeriod, config.CurrentPeriodSnake)
	periodType := resolvePeriodType(currentPeriod)
	creditUsagePercent := normalizeFloatPtr(firstNonNil(config.CreditUsagePercent, config.CreditUsagePctSnake))
	periodStart := firstNonEmpty(periodValue(currentPeriod, "start"), config.BillingPeriodStart, config.BillingStartSnake)
	periodEnd := firstNonEmpty(periodValue(currentPeriod, "end"), config.BillingPeriodEnd, config.BillingEndSnake)
	productUsage := normalizeProductUsage(coalesceProductUsage(config.ProductUsage, config.ProductUsageSnake))

	monthlyLimitCents := normalizeCentPtr(firstNonNil(config.MonthlyLimit, config.MonthlyLimitSnake))
	usedCents := normalizeCentPtr(config.Used)
	onDemandCapCents := normalizeCentPtr(firstNonNil(config.OnDemandCap, config.OnDemandCapSnake))
	explicitOnDemandUsedCents := normalizeCentPtr(firstNonNil(config.OnDemandUsed, config.OnDemandUsedSnake))
	billingPeriodStart := firstNonEmpty(config.BillingPeriodStart, config.BillingStartSnake)
	billingPeriodEnd := firstNonEmpty(config.BillingPeriodEnd, config.BillingEndSnake)

	includedUsedCents := includedMonthlyUsedCents(usedCents, monthlyLimitCents)
	onDemandUsedCents := explicitOnDemandUsedCents
	if onDemandUsedCents == nil {
		onDemandUsedCents = derivedOnDemandUsedCents(usedCents, monthlyLimitCents)
	}
	usedPercent := percentPtr(includedUsedCents, monthlyLimitCents)
	onDemandUsedPercent := percentPtr(onDemandUsedCents, onDemandCapCents)

	hasWeeklyData := creditUsagePercent != nil || periodType == periodWeekly || len(productUsage) > 0
	hasMonthlyData := monthlyLimitCents != nil || usedCents != nil || (!hasWeeklyData && (onDemandCapCents != nil || billingPeriodEnd != ""))
	if !hasWeeklyData && !hasMonthlyData {
		return nil
	}

	if hasWeeklyData {
		if periodType == periodUnknown {
			result.periodType = periodWeekly
		} else {
			result.periodType = periodType
		}
		result.usagePercent = creditUsagePercent
		result.periodStart = periodStart
		result.periodEnd = periodEnd
	} else {
		result.periodType = periodMonthly
		result.usagePercent = usedPercent
		result.periodStart = billingPeriodStart
		result.periodEnd = billingPeriodEnd
	}
	result.productUsage = productUsage
	result.monthlyLimitCents = monthlyLimitCents
	result.usedCents = usedCents
	result.includedUsedCents = includedUsedCents
	result.onDemandCapCents = onDemandCapCents
	result.onDemandUsedCents = onDemandUsedCents
	result.onDemandUsedPercent = onDemandUsedPercent
	if hasMonthlyData {
		result.billingPeriodStart = billingPeriodStart
		result.billingPeriodEnd = billingPeriodEnd
	}
	result.usedPercent = usedPercent
	return result
}

func mergeSummaries(primary, fallback *summary) *summary {
	if primary == nil {
		return fallback
	}
	if fallback == nil {
		return primary
	}
	return &summary{
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

func coalescePeriod(primary, fallback *Period) *Period {
	if primary != nil {
		return primary
	}
	return fallback
}

func coalesceProductUsage(primary, fallback []ProductUsage) []ProductUsage {
	if len(primary) > 0 {
		return primary
	}
	return fallback
}

func coalesceProductUsageSummary(primary, fallback []ProductUsageSummary) []ProductUsageSummary {
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
	if primary != "" && primary != periodUnknown {
		return primary
	}
	if fallback != "" {
		return fallback
	}
	return periodUnknown
}

func resolvePeriodType(period *Period) string {
	rawType := strings.ToLower(strings.TrimSpace(periodValue(period, "type")))
	switch {
	case strings.Contains(rawType, periodWeekly):
		return periodWeekly
	case strings.Contains(rawType, periodMonthly):
		return periodMonthly
	default:
		return periodUnknown
	}
}

func periodValue(period *Period, field string) string {
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

func normalizeProductUsage(items []ProductUsage) []ProductUsageSummary {
	if len(items) == 0 {
		return nil
	}
	out := make([]ProductUsageSummary, 0, len(items))
	for _, item := range items {
		product := strings.TrimSpace(item.Product)
		if product == "" {
			continue
		}
		out = append(out, ProductUsageSummary{
			Product:      product,
			UsagePercent: normalizeFloatPtr(firstNonNil(item.UsagePercent, item.UsagePercentSnake)),
		})
	}
	return out
}

func normalizeFloatPtr(value any) *float64 {
	switch typed := value.(type) {
	case nil:
		return nil
	case float64:
		if !math.IsNaN(typed) && !math.IsInf(typed, 0) {
			return &typed
		}
	case float32:
		converted := float64(typed)
		if !math.IsNaN(converted) && !math.IsInf(converted, 0) {
			return &converted
		}
	case int:
		converted := float64(typed)
		return &converted
	case int64:
		converted := float64(typed)
		return &converted
	case json.Number:
		if converted, err := typed.Float64(); err == nil && !math.IsNaN(converted) && !math.IsInf(converted, 0) {
			return &converted
		}
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return nil
		}
		if converted, err := json.Number(trimmed).Float64(); err == nil && !math.IsNaN(converted) && !math.IsInf(converted, 0) {
			return &converted
		}
	}
	return nil
}

func normalizeCentPtr(value any) *int64 {
	switch typed := value.(type) {
	case Cent:
		return normalizeCentPtr(typed.Val)
	case *Cent:
		if typed == nil {
			return nil
		}
		return normalizeCentPtr(typed.Val)
	case map[string]any:
		return normalizeCentPtr(typed["val"])
	case map[string]json.Number:
		return normalizeCentPtr(typed["val"])
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

func resolvePlanLabel(monthlyLimitCents *int64) string {
	if monthlyLimitCents == nil {
		return ""
	}
	switch *monthlyLimitCents {
	case superGrokLimitCents:
		return planSuperGrok
	case superGrokHeavyLimitCents:
		return planSuperGrokHeavy
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
