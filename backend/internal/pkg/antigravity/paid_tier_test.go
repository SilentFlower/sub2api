package antigravity

import (
	"encoding/json"
	"testing"
)

func TestPaidTierInfoUnmarshal(t *testing.T) {
	raw := `{
		"id": "g1-ultra-tier",
		"name": "Google AI Ultra",
		"description": "Google AI Ultra",
		"availableCredits": [
			{"creditType": "GOOGLE_ONE_AI", "creditAmount": "25000", "minimumCreditAmountForUsage": "50"},
			{"creditType": "GOOGLE_ONE_AI", "creditAmount": "94", "minimumCreditAmountForUsage": "50"}
		]
	}`

	var tier PaidTierInfo
	if err := json.Unmarshal([]byte(raw), &tier); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if tier.ID != "g1-ultra-tier" {
		t.Errorf("ID = %q, want g1-ultra-tier", tier.ID)
	}
	if tier.Name != "Google AI Ultra" {
		t.Errorf("Name = %q, want Google AI Ultra", tier.Name)
	}
	if len(tier.AvailableCredits) != 2 {
		t.Fatalf("AvailableCredits len = %d, want 2", len(tier.AvailableCredits))
	}
	if tier.AvailableCredits[0].CreditAmount != "25000" {
		t.Errorf("CreditAmount[0] = %q, want 25000", tier.AvailableCredits[0].CreditAmount)
	}
	if tier.AvailableCredits[0].GetAmount() != 25000 {
		t.Errorf("GetAmount[0] = %f, want 25000", tier.AvailableCredits[0].GetAmount())
	}
	if tier.AvailableCredits[0].GetMinimumAmount() != 50 {
		t.Errorf("GetMinimumAmount[0] = %f, want 50", tier.AvailableCredits[0].GetMinimumAmount())
	}

	// 兼容旧格式：纯字符串
	var tierStr PaidTierInfo
	if err := json.Unmarshal([]byte(`"free-tier"`), &tierStr); err != nil {
		t.Fatalf("Unmarshal string tier failed: %v", err)
	}
	if tierStr.ID != "free-tier" {
		t.Errorf("String tier ID = %q, want free-tier", tierStr.ID)
	}
}
