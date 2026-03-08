package antigravityls

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSendUserCascadeMessageRequest_JSONUsesFlatTextItem(t *testing.T) {
	req := &SendUserCascadeMessageRequest{
		CascadeID: "cascade-1",
		Items: []CascadeItem{
			{TextOrScopeItem: &TextOrScopeItem{Text: "你好，世界"}},
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request failed: %v", err)
	}

	payload := string(data)
	if !strings.Contains(payload, `"items":[{"text":"你好，世界"}]`) {
		t.Fatalf("unexpected payload: %s", payload)
	}
	if strings.Contains(payload, "textOrScopeItem") {
		t.Fatalf("unexpected wrapped item payload: %s", payload)
	}
}
