package service

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// shouldStripMappedGPT55Lite 统一 main 的 GPT-5.5 兼容规则，避免 HTTP 与 WebSocket 决策漂移。
//
// @param account 本次转发使用的账号。
// @param finalModel 账号映射及归一化后的最终上游模型。
// @return OpenAI OAuth 类账号使用 GPT-5.5 时返回 true。
func shouldStripMappedGPT55Lite(account *Account, finalModel string) bool {
	return account.IsOpenAIOAuthLike() && strings.TrimSpace(finalModel) == "gpt-5.5"
}

// The account mapping changes the model, not the upstream capability header.
// Apply at the final HTTP request boundary, after account/model resolution.
// Never mutate the ingress headers/body: failover may select a native account.
func applyMappedGPT55LiteCompatibility(req *http.Request, account *Account, body []byte) error {
	if req == nil || !shouldStripMappedGPT55Lite(account, gjson.GetBytes(body, "model").String()) {
		return nil
	}
	if !isOpenAIResponsesLiteHeader(req.Header.Get(responsesLiteHeader)) && !isOpenAIResponsesLiteWebSocketPayload(body) {
		return nil
	}
	// The Codex non-Lite endpoint accepts additional_tools, namespaces and
	// reasoning.context=all_turns. Preserve them and all history/tool results.
	if isOpenAIResponsesLiteWebSocketPayload(body) {
		var err error
		body, err = sjson.DeleteBytes(body, "client_metadata."+responsesLiteWSMetadataKey)
		if err != nil {
			return fmt.Errorf("remove mapped GPT-5.5 Lite metadata: %w", err)
		}
		req.Body = io.NopCloser(bytes.NewReader(body))
		req.ContentLength = int64(len(body))
		savedBody := append([]byte(nil), body...)
		req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(savedBody)), nil }
	}
	req.Header.Del(responsesLiteHeader)
	return nil
}
