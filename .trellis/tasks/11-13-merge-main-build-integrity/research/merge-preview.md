# main → build 合并预演记录

- build 基线：`e63f900efd14c8429fba0011bdeb69a43abe22e1`
- main 目标：`530c5e3a4f9c73ca2b0881191d0ef8e0d82c5484`
- 共同基线：`8f2cbb53d9d1c40e1865b07ac460571f7d4c48de`
- 假想合并树：`02c95fa6b3733288a18dca50bbed9c7a373c2dbb`
- 上游差异：813 文件，+65254 / -2990。
- 双方修改交集：100 文件；文本冲突：32 文件；自动合并交集：68 文件。
- 本文记录静态预演。功能验证在实施阶段执行。

## 文本冲突清单

- [x] `backend/internal/config/config.go`
- [x] `backend/internal/config/config_test.go`
- [x] `backend/internal/handler/openai_cyber_allowlist_test.go`
- [x] `backend/internal/handler/openai_gateway_cyber_test.go`
- [x] `backend/internal/handler/openai_gateway_handler.go`
- [x] `backend/internal/pkg/apicompat/anthropic_to_responses_response.go`
- [x] `backend/internal/pkg/apicompat/chatcompletions_anthropic_bridge.go`
- [x] `backend/internal/pkg/apicompat/responses_stream_event_wire.go`
- [x] `backend/internal/pkg/openai/constants.go`
- [x] `backend/internal/service/account.go`
- [x] `backend/internal/service/gateway_forward_as_responses.go`
- [x] `backend/internal/service/openai_compat_model.go`
- [x] `backend/internal/service/openai_cyber_session_block.go`
- [x] `backend/internal/service/openai_cyber_session_block_test.go`
- [x] `backend/internal/service/openai_cyber_transcript.go`（build 删除、main 修改）
- [x] `backend/internal/service/openai_gateway_converted_reasoning_pricing_test.go`
- [x] `backend/internal/service/openai_gateway_grok.go`
- [x] `backend/internal/service/openai_gateway_messages_chat_fallback.go`
- [x] `backend/internal/service/openai_gateway_passthrough.go`
- [x] `backend/internal/service/openai_gateway_service.go`
- [x] `backend/internal/service/setting_service.go`
- [x] `backend/internal/service/settings_view.go`
- [x] `deploy/.env.example`
- [x] `deploy/config.example.yaml`
- [x] `deploy/docker-compose.dev.yml`
- [x] `deploy/docker-compose.local.yml`
- [x] `deploy/docker-compose.standalone.yml`
- [x] `deploy/docker-compose.yml`
- [x] `frontend/src/components/account/EditAccountModal.vue`
- [x] `frontend/src/i18n/locales/en/admin/settings.ts`
- [x] `frontend/src/i18n/locales/zh/admin/settings.ts`
- [x] `frontend/src/views/admin/ChannelsView.vue`

## 自动合并交集逐项审查清单

- [x] `.gitignore`
- [x] `README_CN.md`
- [x] `backend/ent/schema/usage_log.go`
- [x] `backend/ent/usagelog.go`
- [x] `backend/go.sum`
- [x] `backend/internal/handler/admin/account_handler.go`
- [x] `backend/internal/handler/admin/setting_handler_runtime.go`
- [x] `backend/internal/handler/admin/setting_handler_update.go`
- [x] `backend/internal/handler/dto/settings.go`
- [x] `backend/internal/handler/dto/types.go`
- [x] `backend/internal/handler/openai_alpha_search.go`
- [x] `backend/internal/handler/openai_chat_completions.go`
- [x] `backend/internal/handler/openai_gateway_handler_test.go`
- [x] `backend/internal/pkg/apicompat/anthropic_to_responses.go`
- [x] `backend/internal/pkg/apicompat/chatcompletions_responses_bridge.go`
- [x] `backend/internal/pkg/apicompat/responses_to_anthropic_request.go`
- [x] `backend/internal/pkg/apicompat/types.go`
- [x] `backend/internal/repository/account_repo.go`
- [x] `backend/internal/repository/http_upstream.go`
- [x] `backend/internal/server/api_contract_test.go`
- [x] `backend/internal/server/http.go`
- [x] `backend/internal/server/routes/admin.go`
- [x] `backend/internal/service/account_usage_service.go`
- [x] `backend/internal/service/antigravity_gateway_claude.go`
- [x] `backend/internal/service/antigravity_gateway_gemini.go`
- [x] `backend/internal/service/billing_service.go`
- [x] `backend/internal/service/domain_constants.go`
- [x] `backend/internal/service/gateway_forward_as_chat_completions.go`
- [x] `backend/internal/service/gateway_service.go`
- [x] `backend/internal/service/openai_codex_models_service.go`
- [x] `backend/internal/service/openai_codex_transform.go`
- [x] `backend/internal/service/openai_gateway_cc_pipeline.go`
- [x] `backend/internal/service/openai_gateway_chat_completions.go`
- [x] `backend/internal/service/openai_gateway_chat_completions_raw.go`
- [x] `backend/internal/service/openai_gateway_chat_completions_test.go`
- [x] `backend/internal/service/openai_gateway_forward.go`
- [x] `backend/internal/service/openai_gateway_grok_test.go`
- [x] `backend/internal/service/openai_gateway_messages.go`
- [x] `backend/internal/service/openai_gateway_request_body.go`
- [x] `backend/internal/service/openai_gateway_response_handling.go`
- [x] `backend/internal/service/openai_gateway_responses_chat_fallback.go`
- [x] `backend/internal/service/openai_gateway_scheduling.go`
- [x] `backend/internal/service/openai_gateway_service_hotpath_test.go`
- [x] `backend/internal/service/openai_images_direct_test.go`
- [x] `backend/internal/service/openai_oauth_passthrough_test.go`
- [x] `backend/internal/service/openai_ws_forwarder_ingress.go`
- [x] `backend/internal/service/openai_ws_http_bridge.go`
- [x] `backend/internal/service/openai_ws_http_bridge_test.go`
- [x] `backend/internal/service/openai_ws_v2_passthrough_adapter.go`
- [x] `backend/internal/service/usage_log.go`
- [x] `backend/internal/service/wire.go`
- [x] `backend/internal/web/embed_test.go`
- [x] `backend/resources/model-pricing/model_prices_and_context_window.json`
- [x] `frontend/src/api/admin/accounts.ts`
- [x] `frontend/src/api/admin/settings.ts`
- [x] `frontend/src/components/account/BulkEditAccountModal.vue`
- [x] `frontend/src/components/account/CreateAccountModal.vue`
- [x] `frontend/src/components/account/__tests__/BulkEditAccountModal.spec.ts`
- [x] `frontend/src/components/account/__tests__/CreateAccountModal.spec.ts`
- [x] `frontend/src/components/account/__tests__/EditAccountModal.spec.ts`
- [x] `frontend/src/components/admin/account/AccountActionMenu.vue`
- [x] `frontend/src/components/keys/__tests__/UseKeyModal.spec.ts`
- [x] `frontend/src/i18n/locales/en/admin/accounts.ts`
- [x] `frontend/src/i18n/locales/en/admin/channels.ts`
- [x] `frontend/src/i18n/locales/zh/admin/accounts.ts`
- [x] `frontend/src/i18n/locales/zh/admin/channels.ts`
- [x] `frontend/src/views/admin/SettingsView.vue`
- [x] `frontend/src/views/admin/__tests__/SettingsView.spec.ts`

## 双方冲突原文


### backend/internal/config/config.go

冲突块 1：

```text
build:
	OpenAIResponseHeaderTimeout int `mapstructure:"openai_response_header_timeout"`
	// OpenAIImagesResponseHeaderTimeout: 图片生成和编辑等待上游响应头的超时时间（秒），0 表示无超时。
	OpenAIImagesResponseHeaderTimeout int `mapstructure:"openai_images_response_header_timeout"`
	// OpenAINonstreamResponseHeaderTimeout: 非流式文本等待上游响应头的超时时间（秒），0 表示无超时。
	OpenAINonstreamResponseHeaderTimeout int `mapstructure:"openai_nonstream_response_header_timeout"`
main:
	OpenAIResponseHeaderTimeout         int  `mapstructure:"openai_response_header_timeout"`
	DisableOpenAITransportErrorCooldown bool `mapstructure:"disable_openai_transport_error_cooldown"`
```

冲突块 2：

```text
build:
	viper.SetDefault("gateway.openai_images_response_header_timeout", 600)
	viper.SetDefault("gateway.openai_nonstream_response_header_timeout", 300)
main:
	viper.SetDefault("gateway.disable_openai_transport_error_cooldown", false)
```


### backend/internal/config/config_test.go

冲突块 1：

```text
build:
			name:    "gateway openai images response header timeout",
			mutate:  func(c *Config) { c.Gateway.OpenAIImagesResponseHeaderTimeout = -1 },
			wantErr: "gateway.openai_images_response_header_timeout",
		},
		{
			name:    "gateway openai nonstream response header timeout",
			mutate:  func(c *Config) { c.Gateway.OpenAINonstreamResponseHeaderTimeout = -1 },
			wantErr: "gateway.openai_nonstream_response_header_timeout",
main:
			name:    "gateway gemini same account retries negative",
			mutate:  func(c *Config) { c.Gateway.MaxSameAccountRetriesGemini = -1 },
			wantErr: "gateway.max_same_account_retries_gemini",
		},
		{
			name:    "gateway gemini same account retries too large",
			mutate:  func(c *Config) { c.Gateway.MaxSameAccountRetriesGemini = 11 },
			wantErr: "gateway.max_same_account_retries_gemini",
```


### backend/internal/handler/openai_cyber_allowlist_test.go

冲突块 1：

```text
build:
	c.Request.Header.Set("session_id", "ws-test-session")
	wsKey := service.CyberSessionExplicitBlockKey(harness.apiKey.ID, c, payload)
	require.NotEmpty(t, wsKey)
	require.NotEqual(t, explicitKey, wsKey)
	matched, err := store.FindCyberSessionBlocked(ctx, []string{wsKey})
main:
	transcriptKeys := make([]string, 0, 2)
	for _, transcript := range service.CyberSessionTranscriptBlocks(harness.apiKey.ID, payload) {
		transcriptKeys = append(transcriptKeys, transcript.Key)
	}
	matched, err := store.FindCyberSessionBlocked(ctx, transcriptKeys)
```


### backend/internal/handler/openai_gateway_cyber_test.go

冲突块 1：

```text
build:
func TestBuildCyberSessionBlockWritePlanUsesOnlyExplicitIdentity(t *testing.T) {
main:
func TestBuildCyberSessionBlockCombinesExplicitAndTranscriptKeys(t *testing.T) {
```

冲突块 2：

```text
build:
	plan := buildCyberSessionBlockWritePlan(7, c, body)
	require.Empty(t, plan.keys)

	c.Request.Header.Set("session_id", "sess-explicit")
	plan = buildCyberSessionBlockWritePlan(7, c, body)
	require.Equal(t, []string{service.CyberSessionExplicitBlockKey(7, c, body)}, plan.keys)

	c.Request.Header.Del("session_id")
	require.Empty(t, buildCyberSessionBlockWritePlan(7, c, []byte(`{"prompt_cache_key":"shared-cache"}`)).keys)
main:
	block := buildCyberSessionBlock(7, c, body)
	require.Len(t, block.Keys, 2)
	require.Equal(t, []int{3, 2}, block.TranscriptLengths)
	require.Equal(t, int64(7), block.APIKeyID)
	require.NotEmpty(t, block.ScopeKey)

	c.Request.Header.Set("session_id", "sess-explicit")
	block = buildCyberSessionBlock(7, c, body)
	require.Len(t, block.Keys, 3)
	require.Equal(t, []int{3, 2}, block.TranscriptLengths)
	require.NotEmpty(t, block.ScopeKey)
```


### backend/internal/handler/openai_gateway_handler.go

冲突块 1：

```text
build:
type cyberSessionBlockWritePlan struct {
	keys []string
}

func buildCyberSessionBlockWritePlan(apiKeyID int64, c *gin.Context, body []byte) cyberSessionBlockWritePlan {
	plan := cyberSessionBlockWritePlan{}
main:
func buildCyberSessionBlock(apiKeyID int64, c *gin.Context, body []byte) service.CyberSessionBlock {
	block := service.CyberSessionBlock{APIKeyID: apiKeyID}
```

冲突块 2：

```text
build:
	return plan
main:
	transcriptBlocks := service.CyberSessionTranscriptBlocks(apiKeyID, body)
	for _, transcript := range transcriptBlocks {
		block.Keys = append(block.Keys, transcript.Key)
		block.TranscriptLengths = append(block.TranscriptLengths, transcript.Length)
	}
	if len(transcriptBlocks) > 0 {
		block.ScopeKey = cyberSessionScopeKey(apiKeyID, c)
	}
	return block
```

冲突块 3：

```text
build:
			gwSvc.MarkCyberSessionBlocked(blockCtx, "", plan.keys)
main:
			gwSvc.MarkCyberSessionBlocked(blockCtx, block)
```


### backend/internal/pkg/apicompat/anthropic_to_responses_response.go

冲突块 1：

```text
build:
			part := ResponsesContentPart{
				Type:        "output_text",
				Text:        block.Text,
				Annotations: anthropicCitationsToResponses(block.Citations, block.Text),
			}
			if (claude.IsOpus55(resp.Model) || claude.IsSonnet55(resp.Model) || claude.IsHaiku55(resp.Model)) && block.Text != "" {
				// 5.5 保留独立文本项的顺序，同时携带搜索引用，避免签名分支绕过引用转换。
				outputs = append(outputs, ResponsesOutput{Type: "message", ID: generateItemID(), Role: "assistant", Status: "completed", Content: []ResponsesContentPart{part}})
main:
			if (claude.IsOpus55(resp.Model) || claude.IsSonnet55(resp.Model) || claude.IsHaiku55(resp.Model) || claude.IsFable52(resp.Model) || claude.IsMythos52(resp.Model)) && block.Text != "" {
				outputs = append(outputs, ResponsesOutput{Type: "message", ID: generateItemID(), Role: "assistant", Status: "completed", Content: []ResponsesContentPart{{Type: "output_text", Text: block.Text}}})
```

冲突块 2：

```text
build:
		state.CurrentText += evt.Delta.Text
		state.TextAccum += evt.Delta.Text
		events := []ResponsesStreamEvent{makeResponsesEvent(state, "response.output_text.delta", &ResponsesStreamEvent{
main:
		state.TextAccum = appendStreamString(&state.textAccumBuffer, state.TextAccum, evt.Delta.Text)
		return []ResponsesStreamEvent{makeResponsesEvent(state, "response.output_text.delta", &ResponsesStreamEvent{
```


### backend/internal/pkg/apicompat/chatcompletions_anthropic_bridge.go

冲突块 1：

```text
build:
		state.toolArgs[idx] += toolCall.Function.Arguments
		if !state.toolAnnounced[idx] {
			state.pendingToolArgs[idx] += toolCall.Function.Arguments
main:
		if state.toolAnnounced[idx] {
			blockIdx := state.toolBlockIndex[idx]
			if state.ContentBlockOpen && blockIdx == state.ContentBlockIndex {
				state.CurrentToolHadDelta = true
			}
			events = append(events, AnthropicStreamEvent{
				Type:  "content_block_delta",
				Index: &blockIdx,
				Delta: &AnthropicDelta{
					Type:        "input_json_delta",
					PartialJSON: toolCall.Function.Arguments,
				},
			})
		} else {
			buffer := state.pendingToolArgs[idx]
			if buffer == nil {
				buffer = &strings.Builder{}
				state.pendingToolArgs[idx] = buffer
			}
			_, _ = buffer.WriteString(toolCall.Function.Arguments)
```

冲突块 2：

```text
build:
	if pending := state.pendingToolArgs[idx]; pending != "" && name != "Read" {
main:
	if buffer := state.pendingToolArgs[idx]; buffer != nil && buffer.Len() > 0 {
		pending := buffer.String()
```


### backend/internal/pkg/apicompat/responses_stream_event_wire.go

冲突块 1：

```text
build:
		part := map[string]any{"type": typ, "text": p.Text}
		if len(p.Annotations) > 0 {
			part["annotations"] = p.Annotations
		}
		out = append(out, part)
main:
		switch typ {
		case "output_text":
			out = append(out, outputTextPartWire(&p))
		case "refusal":
			out = append(out, map[string]any{"type": typ, "refusal": p.Refusal})
		default:
			out = append(out, map[string]any{"type": typ, "text": p.Text})
		}
```


### backend/internal/pkg/openai/constants.go

冲突块 1：

```text
build:
// latestCodexInstructions 保持既有 GPT-5.5 回退，避免新增模板改变旧模型和未知模型的行为。
// 若 GPT-5.5 prompt 意外为空则回退到 DefaultInstructions 保证非空。
main:
// CodexGPT61TerraMetadata is the complete official descriptor from openai/codex
// 126a0ee92ed2c5e73cd8a8f7e37563e8527552d7, codex-rs/models-manager/models.json.
//
//go:embed codex_gpt61_terra.json
var CodexGPT61TerraMetadata []byte

// codexInstructionsTemplate returns the base prompt bundled in an official
// Codex model descriptor.
func codexInstructionsTemplate(descriptor []byte) string {
	var metadata struct {
		ModelMessages struct {
			InstructionsTemplate string `json:"instructions_template"`
		} `json:"model_messages"`
	}
	if err := json.Unmarshal(descriptor, &metadata); err != nil {
		panic(err)
	}
	return metadata.ModelMessages.InstructionsTemplate
}

// latestCodexInstructions 返回当前已知最新版本的 Codex base instructions，
// 当前为 GPT-5.5；若 5.5 prompt 意外为空则回退到 DefaultInstructions 保证非空。
```


### backend/internal/service/account.go

冲突块 1：

```text
build:
	"hash/fnv"
main:
	"log/slog"
```


### backend/internal/service/gateway_forward_as_responses.go

冲突块 1：

```text
build:
	var pendingCitations []apicompat.AnthropicCitation
	contentPositionByUpstreamIndex := make(map[int]int)
main:
	streamStatus := &anthropicCompatStream{body: resp.Body}
```


### backend/internal/service/openai_compat_model.go

冲突块 1：

```text
build:
func applyOpenAICompatModelNormalization(req *apicompat.AnthropicRequest) string {
	if req == nil {
		return ""
	}
	if openai.IsGPT61SolModelSpelling(req.Model) {
		canonical := openai.CanonicalizeOpenAIModelAliasSpelling(req.Model)
		if effort, ok := strings.CutPrefix(canonical, "gpt-6.1-sol-"); ok && effort != "openai-compact" {
			req.Model = "gpt-6.1-sol"
			// 显式关闭推理优先于后缀，防止调用方回填派生 effort 后重新开启。
			if req.Thinking != nil && req.Thinking.Type == "disabled" {
				return ""
			}
main:
func applyOpenAICompatModelNormalization(req *apicompat.AnthropicRequest) {
	if req != nil && (openai.IsGPT61SolModelSpelling(req.Model) || openai.IsGPT61TerraModelSpelling(req.Model)) {
		canonical := openai.CanonicalizeOpenAIModelAliasSpelling(req.Model)
		baseModel := NormalizeOpenAICompatRequestedModel(req.Model)
		if effort, ok := strings.CutPrefix(canonical, baseModel+"-"); ok && effort != "openai-compact" {
			req.Model = baseModel
```


### backend/internal/service/openai_cyber_session_block.go

冲突块 1：

```text
build:
// CyberSessionExplicitBlockKey 仅按认证 API Key 与明确会话请求头生成屏蔽键。
// 缓存提示、粘性标记和历史相似度均不能证明会话身份；共享 Key 的网关需为各会话提供独立 ID。
//
// @param apiKeyID 认证后的网关 API Key ID。
// @param c 包含原始会话请求头的请求上下文。
// @param _ 为保持调用兼容而保留的请求体，不参与会话身份判断。
// @return v2 会话哈希；身份缺失时返回空字符串。
func CyberSessionExplicitBlockKey(apiKeyID int64, c *gin.Context, _ []byte) string {
	if apiKeyID <= 0 || c == nil || c.Request == nil {
main:
// CyberSessionBlock is one cyber hit to persist. Keys are exact block keys;
// TranscriptLengths records the history length of each transcript key among
// them and, like ScopeKey, is only meaningful for transcript matching.
type CyberSessionBlock struct {
	APIKeyID          int64
	ScopeKey          string
	Keys              []string
	TranscriptLengths []int
}

// CyberSessionExplicitBlockKey returns an inexpensive exact key when the
// client supplies a stable session signal.
func CyberSessionExplicitBlockKey(apiKeyID int64, c *gin.Context, body []byte) string {
	return hashCyberSessionBlockKey(apiKeyID, explicitOpenAISessionID(c, body))
}

// CyberSessionScopeKey is a coarse, non-blocking fingerprint used only to
// avoid transcript parsing and MGET for sources that never produced a hit.
func CyberSessionScopeKey(apiKeyID int64, clientIP, userAgent string) string {
	if apiKeyID <= 0 {
```

冲突块 2：

```text
build:
	matched, err := store.FindCyberSessionBlocked(ctx, []string{key})
main:
	if explicitKey := CyberSessionExplicitBlockKey(apiKeyID, c, body); explicitKey != "" {
		key, err := store.FindCyberSessionBlocked(ctx, []string{explicitKey})
		if err != nil {
			logger.LegacyPrintf("service.openai_gateway", "cyber explicit session read failed: err=%v", err)
			return ""
		}
		if key != "" {
			return key
		}
	}
	lengths, err := store.CyberSessionTranscriptLengths(ctx, apiKeyID, CyberSessionScopeKey(apiKeyID, clientIP, userAgent))
```

冲突块 3：

```text
build:
	return matched
main:
	// Blocked prefixes are recomputed only at their recorded lengths, so any
	// number of appended items still reaches them and history length alone
	// never decides a block.
	keys := cyberSessionTranscriptKeysAt(apiKeyID, body, lengths)
	if len(keys) == 0 {
		return ""
	}
	key, err := store.FindCyberSessionBlocked(ctx, keys)
	if err != nil {
		logger.LegacyPrintf("service.openai_gateway", "cyber session block batch read failed: err=%v", err)
		return ""
	}
	return key
```


### backend/internal/service/openai_cyber_session_block_test.go

冲突块 1：

```text
build:
// --- fakes ---

type fakeCyberBlockStore struct {
	blocked    map[string]bool
	scopes     map[string]bool
	findCalls  int
	findErr    error
	scopeCalls int
main:
func TestCyberSessionExplicitBlockKey(t *testing.T) {
	c1, b1 := newCyberBlockTestCtx(map[string]string{"session_id": "sess-abc"}, `{}`)
	k1 := CyberSessionExplicitBlockKey(101, c1, b1)
	require.NotEmpty(t, k1)

	// Same session, different apiKey → different key (isolation).
	c2, b2 := newCyberBlockTestCtx(map[string]string{"session_id": "sess-abc"}, `{}`)
	require.NotEqual(t, k1, CyberSessionExplicitBlockKey(202, c2, b2))

	// Same session + same apiKey → stable key.
	c3, b3 := newCyberBlockTestCtx(map[string]string{"session_id": "sess-abc"}, `{}`)
	require.Equal(t, k1, CyberSessionExplicitBlockKey(101, c3, b3))

	// prompt_cache_key in body counts as explicit.
	c4, b4 := newCyberBlockTestCtx(nil, `{"prompt_cache_key":"pck-1"}`)
	require.NotEmpty(t, CyberSessionExplicitBlockKey(101, c4, b4))

	// No explicit signal → empty key → caller must skip blocking entirely.
	c5, b5 := newCyberBlockTestCtx(nil, `{"input":"hello world"}`)
	require.Empty(t, CyberSessionExplicitBlockKey(101, c5, b5))

	// conversation_id header counts as explicit; key is stable and non-empty.
	c6, b6 := newCyberBlockTestCtx(map[string]string{"conversation_id": "conv-xyz"}, `{}`)
	k6 := CyberSessionExplicitBlockKey(101, c6, b6)
	require.NotEmpty(t, k6)
	c6b, b6b := newCyberBlockTestCtx(map[string]string{"conversation_id": "conv-xyz"}, `{}`)
	require.Equal(t, k6, CyberSessionExplicitBlockKey(101, c6b, b6b), "conversation_id key must be stable")
}

func TestCyberTranscriptBlocksRequireModelGeneratedHistory(t *testing.T) {
	first := []byte(`{"instructions":"shared","input":[{"role":"user","content":"fixed environment"},{"role":"user","content":"question one"}]}`)
	second := []byte(`{"instructions":"shared","input":[{"role":"user","content":"fixed environment"},{"role":"user","content":"question two"}]}`)
	firstBlocks := CyberSessionTranscriptBlocks(77, first)
	secondBlocks := CyberSessionTranscriptBlocks(77, second)
	require.Len(t, firstBlocks, 1)
	require.Len(t, secondBlocks, 1)
	require.NotEqual(t, firstBlocks[0].Key, secondBlocks[0].Key)

	hit := []byte(`{"messages":[{"role":"user","content":"setup"},{"role":"assistant","content":"ready"},{"role":"user","content":"trigger"}]}`)
	continuation := []byte(`{"messages":[{"role":"user","content":"setup"},{"role":"assistant","content":"ready"},{"role":"user","content":"different trigger"},{"role":"assistant","content":"blocked"},{"role":"user","content":"continue"}]}`)
	hitBlocks := CyberSessionTranscriptBlocks(77, hit)
	require.Equal(t, []int{3, 2}, []int{hitBlocks[0].Length, hitBlocks[1].Length})
	require.Equal(t, []string{hitBlocks[1].Key}, cyberSessionTranscriptKeysAt(77, continuation, []int{hitBlocks[1].Length}))
	require.NotEqual(t, []string{hitBlocks[0].Key}, cyberSessionTranscriptKeysAt(77, continuation, []int{hitBlocks[0].Length}))
}

func TestCyberTranscriptBlocksWebSocketResponseCreate(t *testing.T) {
	body := []byte(`{"type":"response.create","response":{"prompt_cache_key":"ws-session","input":[{"role":"user","content":"setup"},{"role":"assistant","content":"ready"},{"role":"user","content":"trigger"}]}}`)
	c, _ := newCyberBlockTestCtx(nil, string(body))
	require.NotEmpty(t, CyberSessionExplicitBlockKey(88, c, body))
	require.Len(t, CyberSessionTranscriptBlocks(88, body), 2)
}

func TestCyberTranscriptBlocksStringInputMatchesSingleItemHistory(t *testing.T) {
	blocks := CyberSessionTranscriptBlocks(77, []byte(`{"input":"hello"}`))
	require.Equal(t, []CyberSessionTranscriptBlock{{Key: blocks[0].Key, Length: 1}}, blocks)
	require.Equal(t, []string{blocks[0].Key}, cyberSessionTranscriptKeysAt(77, []byte(`{"input":["hello","more"]}`), []int{1}))
	require.Empty(t, CyberSessionTranscriptBlocks(77, []byte(`{"input":"  "}`)))
}

func TestCyberTranscriptKeysAtRecomputesRecordedLengthsInOrder(t *testing.T) {
	messages := make([]map[string]string, 300)
	for i := range messages {
		messages[i] = map[string]string{"role": "user", "content": "message-" + strconv.Itoa(i)}
	}
	body, err := json.Marshal(map[string]any{"messages": messages})
	require.NoError(t, err)
	prefixKey := func(length int) string {
		prefix, err := json.Marshal(map[string]any{"messages": messages[:length]})
		require.NoError(t, err)
		return CyberSessionTranscriptBlocks(77, prefix)[0].Key
	}

	require.Equal(t, []string{prefixKey(1), prefixKey(45), prefixKey(300)},
		cyberSessionTranscriptKeysAt(77, body, []int{300, 45, 1, 45, 301, 0}))
	require.Empty(t, cyberSessionTranscriptKeysAt(77, body, nil))
	require.Equal(t, CyberSessionTranscriptBlocks(77, body)[0], CyberSessionTranscriptBlock{Key: prefixKey(300), Length: 300})
}

// --- fakes ---

type fakeCyberBlockStore struct {
	blocked  map[string]bool
	scopes   map[string]bool
	lengths  map[int64][]int
	findKeys [][]string
```

冲突块 2：

```text
build:
func (f *fakeCyberBlockStore) IsCyberSessionScopeActive(_ context.Context, scopeKey string) (bool, error) {
	f.scopeCalls++
	return f.scopes[scopeKey], nil
}

func (f *fakeCyberBlockStore) FindCyberSessionBlocked(_ context.Context, keys []string) (string, error) {
	f.findCalls++
	if f.findErr != nil {
		return "", f.findErr
	}
main:
func (f *fakeCyberBlockStore) CyberSessionTranscriptLengths(_ context.Context, apiKeyID int64, scopeKey string) ([]int, error) {
	if !f.scopes[scopeKey] {
		return nil, nil
	}
	return f.lengths[apiKeyID], nil
}

func (f *fakeCyberBlockStore) FindCyberSessionBlocked(_ context.Context, keys []string) (string, error) {
	f.findKeys = append(f.findKeys, keys)
```

冲突块 3：

```text
build:
func TestCyberSessionExplicitBlockKeyUsesSessionHeadersOnly(t *testing.T) {
	var expected string
	for _, header := range []string{"session-id", "session_id", "conversation_id", "X-Session-Id", "X-OpenCode-Session", "X-Conversation-ID"} {
		c, body := newCyberBlockTestCtx(map[string]string{header: " conversation-a "}, `{"prompt_cache_key":"shared-cache"}`)
		key := CyberSessionExplicitBlockKey(7, c, body)
		require.NotEmpty(t, key, header)
		if expected == "" {
			expected = key
		}
		require.Equal(t, expected, key, "header aliases must identify the same conversation")
		require.NotEqual(t, key, CyberSessionExplicitBlockKey(8, c, body))
		require.Empty(t, CyberSessionExplicitBlockKey(0, c, body))
		require.Empty(t, CyberSessionExplicitBlockKey(-1, c, body))
	}
	for _, body := range []string{`{}`, `{"prompt_cache_key":"conversation-a"}`, `{"type":"response.create","response":{"prompt_cache_key":"conversation-a","input":"hello"}}`} {
		c, b := newCyberBlockTestCtx(map[string]string{"X-Session-Affinity": "conversation-a"}, body)
		require.Empty(t, CyberSessionExplicitBlockKey(7, c, b))
	}
	require.Empty(t, CyberSessionExplicitBlockKey(7, nil, nil))
	require.Empty(t, CyberSessionExplicitBlockKey(7, &gin.Context{}, nil))
}

func TestCyberSessionIsolationAcrossSharedKeyAndLongHistory(t *testing.T) {
	ctx := context.Background()
	combo := &comboCacheAndStore{}
	svc := &OpenAIGatewayService{cache: combo, settingService: &SettingService{settingRepo: &fakeSettingRepo{vals: map[string]string{
		SettingKeyCyberSessionBlockEnabled: "true", SettingKeyCyberSessionBlockTTLSeconds: "60",
	}}}}
	// Same input, cache hint and source cannot merge distinct session IDs.
	items := make([]map[string]string, 600)
	for i := range items {
		items[i] = map[string]string{"type": "function_call_output", "call_id": "call-" + strconv.Itoa(i), "output": "ordinary tool result"}
	}
	payload, err := json.Marshal(map[string]any{"input": items, "prompt_cache_key": "shared-cache"})
	require.NoError(t, err)
	a, _ := newCyberBlockTestCtx(map[string]string{"session_id": "conversation-a"}, string(payload))
	aKey := CyberSessionExplicitBlockKey(7, a, payload)
	legacyScope := sha256.Sum256([]byte("cyber-scope:v1|api_key=7|ip=203.0.113.20|ua=" + NormalizeSessionUserAgent("Codex CLI 1.2.3")))
	svc.MarkCyberSessionBlocked(ctx, hex.EncodeToString(legacyScope[:]), []string{aKey})

	for _, tc := range []struct {
		name, session string
		apiKey        int64
		blocked       bool
	}{
		{"original session remains blocked", "conversation-a", 7, true},
		{"another session shares key and history", "conversation-b", 7, false},
		{"another authenticated key reuses session name", "conversation-a", 8, false},
		{"no explicit identity with shared cache", "", 7, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := newCyberBlockTestCtx(map[string]string{"session_id": tc.session}, string(payload))
			got := svc.FindCyberSessionBlockedForRequest(ctx, tc.apiKey, c, payload, "203.0.113.20", "Codex CLI 1.2.3")
			if tc.blocked {
				require.Equal(t, aKey, got)
			} else {
				require.Empty(t, got)
			}
		})
	}
	// A reconnect/model change and a different network must not evade an exact block.
	require.Equal(t, aKey, svc.FindCyberSessionBlockedForRequest(ctx, 7, a, []byte(`{"model":"another-model","input":"next turn"}`), "203.0.113.99", "AnotherClient/2"))
	require.Zero(t, combo.store.scopeCalls, "a shared source is never a blocking identity")
main:
func TestFindCyberSessionBlockedForRequestUsesScopeForTranscript(t *testing.T) {
	settingSvc := &SettingService{settingRepo: &fakeSettingRepo{vals: map[string]string{
		SettingKeyCyberSessionBlockEnabled:    "true",
		SettingKeyCyberSessionBlockTTLSeconds: "60",
	}}}
	combo := &comboCacheAndStore{}
	svc := &OpenAIGatewayService{cache: combo, settingService: settingSvc}
	ctx := context.Background()

	hitBody := []byte(`{"messages":[{"role":"user","content":"setup"},{"role":"assistant","content":"ready"},{"role":"user","content":"trigger"}]}`)
	nextBody := []byte(`{"messages":[{"role":"user","content":"setup"},{"role":"assistant","content":"ready"},{"role":"user","content":"different trigger"},{"role":"assistant","content":"blocked"},{"role":"user","content":"continue"}]}`)
	nextCtx, _ := newCyberBlockTestCtx(nil, string(nextBody))
	const clientIP = "203.0.113.20"
	const userAgent = "Codex CLI 1.2.3"
	block := CyberSessionTranscriptBlocks(9, hitBody)[1]

	// Without an active source scope, transcript candidates are never blocks.
	require.Empty(t, svc.FindCyberSessionBlockedForRequest(ctx, 9, nextCtx, nextBody, clientIP, userAgent))
	scopeKey := CyberSessionScopeKey(9, clientIP, userAgent)
	svc.MarkCyberSessionBlocked(ctx, CyberSessionBlock{APIKeyID: 9, ScopeKey: scopeKey, Keys: []string{block.Key}, TranscriptLengths: []int{block.Length}})
	require.Equal(t, block.Key, svc.FindCyberSessionBlockedForRequest(ctx, 9, nextCtx, nextBody, clientIP, "Codex CLI 1.2.4"))
}

// Issue #8519: once a source scope is active, unrelated sessions with long
// histories must pass, while a blocked transcript padded with any number of
// appended items must still match its recorded prefix.
func TestFindCyberSessionBlockedForRequestMatchesLongHistoriesExactly(t *testing.T) {
	settingSvc := &SettingService{settingRepo: &fakeSettingRepo{vals: map[string]string{
		SettingKeyCyberSessionBlockEnabled:    "true",
		SettingKeyCyberSessionBlockTTLSeconds: "60",
	}}}
	combo := &comboCacheAndStore{}
	svc := &OpenAIGatewayService{cache: combo, settingService: settingSvc}
	ctx := context.Background()
	const apiKeyID = int64(9)
	const clientIP = "203.0.113.20"
	const userAgent = "Codex CLI 1.2.3"

	history := func(prefix []map[string]string, padding int, tag string) []byte {
		items := append([]map[string]string{}, prefix...)
		for i := 0; i < padding; i++ {
			role := "user"
			if i%2 == 0 {
				role = "assistant"
			}
			items = append(items, map[string]string{"role": role, "content": tag + "-" + strconv.Itoa(i)})
		}
		body, err := json.Marshal(map[string]any{"input": items})
		require.NoError(t, err)
		return body
	}
	hitPrefix := []map[string]string{{"role": "user", "content": "setup"}, {"role": "assistant", "content": "ready"}, {"role": "user", "content": "trigger"}}
	hitBody := history(hitPrefix, 0, "")
	hitCtx, _ := newCyberBlockTestCtx(map[string]string{"session_id": "sess-hit"}, string(hitBody))
	block := CyberSessionBlock{
		APIKeyID: apiKeyID,
		ScopeKey: CyberSessionScopeKey(apiKeyID, clientIP, userAgent),
		Keys:     []string{CyberSessionExplicitBlockKey(apiKeyID, hitCtx, hitBody)},
	}
	transcripts := CyberSessionTranscriptBlocks(apiKeyID, hitBody)
	for _, transcript := range transcripts {
		block.Keys = append(block.Keys, transcript.Key)
		block.TranscriptLengths = append(block.TranscriptLengths, transcript.Length)
	}
	svc.MarkCyberSessionBlocked(ctx, block)

	for _, items := range []int{256, 257, 1000} {
		unrelated := history(nil, items, "unrelated")
		c, _ := newCyberBlockTestCtx(map[string]string{"session_id": "sess-other"}, string(unrelated))
		require.Empty(t, svc.FindCyberSessionBlockedForRequest(ctx, apiKeyID, c, unrelated, clientIP, userAgent), "items=%d", items)
	}

	padded := history(hitPrefix, 1000, "padding")
	c, _ := newCyberBlockTestCtx(nil, string(padded))
	// The shorter rewrite-tolerant prefix is checked first (earliest match).
	require.Equal(t, transcripts[1].Key, svc.FindCyberSessionBlockedForRequest(ctx, apiKeyID, c, padded, clientIP, userAgent))
	require.Len(t, combo.store.findKeys[len(combo.store.findKeys)-1], len(block.TranscriptLengths),
		"lookup must only recompute the recorded prefix lengths")
```


### backend/internal/service/openai_cyber_transcript.go


### backend/internal/service/openai_gateway_converted_reasoning_pricing_test.go

冲突块 1：

```text
build:
				body := []byte(fmt.Sprintf(`{"model":"glm-5.2","stream":%t,"max_tokens":32,"output_config":{"effort":%q},"messages":[{"role":"user","content":"hello"}]}`, stream, tc.effort))
				// Messages→Chat 回退始终向上游请求 SSE，非流式客户端由网关本地折叠，mock 统一返回 SSE。
				responseBody := "data: " + `{"id":"chatcmpl_glm","object":"chat.completion.chunk","model":"glm-5.2","choices":[{"index":0,"delta":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1000000,"completion_tokens":2,"total_tokens":1000002}}` + "\n\ndata: [DONE]\n\n"
				upstream := &httpUpstreamRecorder{resp: &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
					Body:       io.NopCloser(strings.NewReader(responseBody)),
				}}
				svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
				ctx := context.Background()
				if tc.maxPolicy != "" {
					ctx = WithOpenAIReasoningEffortPolicy(ctx, tc.maxPolicy, nil, "")
				}
				result, err := svc.ForwardAsAnthropic(ctx, adaptiveProtocolTestContext("/v1/messages", body), forceChatMessagesFallbackAccount(), body, "", "")
				require.NoError(t, err)
				require.Equal(t, "/v1/chat/completions", upstream.lastReq.URL.Path)
				require.True(t, gjson.GetBytes(upstream.lastBody, "stream").Bool())
				require.Equal(t, tc.wantEffort, gjson.GetBytes(upstream.lastBody, "reasoning_effort").String())
				require.Equal(t, tc.wantEffort, optionalStringValue(result.ReasoningEffort))
				assertConvertedReasoningPrice(t, result, tc.wantCost)
main:
				for _, overLimit := range []string{"", ReasoningEffortOverLimitDowngrade, ReasoningEffortOverLimitDeny} {
					t.Run("overLimit="+overLimit, func(t *testing.T) {
						body := []byte(fmt.Sprintf(`{"model":"glm-5.2","stream":%t,"max_tokens":32,"output_config":{"effort":%q},"messages":[{"role":"user","content":"hello"}]}`, stream, tc.effort))
						responseBody := `{"id":"chatcmpl_glm","object":"chat.completion","model":"glm-5.2","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1000000,"completion_tokens":2,"total_tokens":1000002}}`
						contentType := "application/json"
						if stream {
							contentType = "text/event-stream"
							responseBody = "data: " + `{"id":"chatcmpl_glm","object":"chat.completion.chunk","model":"glm-5.2","choices":[{"index":0,"delta":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1000000,"completion_tokens":2,"total_tokens":1000002}}` + "\n\ndata: [DONE]\n\n"
						}
						upstream := &httpUpstreamRecorder{resp: &http.Response{
							StatusCode: http.StatusOK,
							Header:     http.Header{"Content-Type": []string{contentType}},
							Body:       io.NopCloser(strings.NewReader(responseBody)),
						}}
						svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
						ctx := context.Background()
						if tc.maxPolicy != "" {
							ctx = WithOpenAIReasoningEffortPolicyForMessages(ctx, body, tc.maxPolicy, nil, overLimit)
						}
						requestContext := adaptiveProtocolTestContext("/v1/messages", body)
						account := forceChatMessagesFallbackAccount()
						require.Equal(t, PlatformOpenAI, account.Platform)
						result, err := svc.ForwardAsAnthropic(ctx, requestContext, account, body, "", "")
						if tc.maxPolicy != "" && overLimit == ReasoningEffortOverLimitDeny {
							require.True(t, IsReasoningEffortPolicyDenied(err), "expected policy denial, got %v", err)
							require.Equal(t, http.StatusForbidden, requestContext.Writer.Status())
							require.Empty(t, upstream.requests)
							require.Nil(t, result)
							return
						}
						require.NoError(t, err)
						require.Equal(t, http.StatusOK, requestContext.Writer.Status())
						require.Len(t, upstream.requests, 1)
						require.Equal(t, "/v1/chat/completions", upstream.lastReq.URL.Path)
						require.Equal(t, tc.wantEffort, gjson.GetBytes(upstream.lastBody, "reasoning_effort").String())
						require.Equal(t, tc.wantEffort, optionalStringValue(result.ReasoningEffort))
						assertConvertedReasoningPrice(t, result, tc.wantCost)
					})
				}
```


### backend/internal/service/openai_gateway_grok.go

冲突块 1：

```text
build:
		RequestID:       firstNonEmpty(resp.Header.Get("x-request-id"), resp.Header.Get("xai-request-id")),
		UpstreamHeaders: resp.Header,
		ResponseID:      responseID,
		Usage:           *usage,
		Model:           originalModel,
		UpstreamModel:   upstreamModel,
		ReasoningEffort: extractFinalOpenAIReasoningEffort(patchedBody),
		Stream:          reqStream,
		OpenAIWSMode:    false,
		ResponseHeaders: resp.Header.Clone(),
		Duration:        time.Since(startTime),
		FirstTokenMs:    firstTokenMs,
main:
		RequestID:                     firstNonEmpty(resp.Header.Get("x-request-id"), resp.Header.Get("xai-request-id")),
		UpstreamHeaders:               resp.Header,
		ResponseID:                    responseID,
		Usage:                         *usage,
		Model:                         originalModel,
		UpstreamModel:                 upstreamModel,
		UpstreamResponseModel:         observedUpstreamResponseModel(c),
		UpstreamResponseModelConflict: observedUpstreamResponseModelConflict(c),
		ReasoningEffort:               reasoningEffort,
		Stream:                        reqStream,
		OpenAIWSMode:                  false,
		ResponseHeaders:               resp.Header.Clone(),
		Duration:                      time.Since(startTime),
		FirstTokenMs:                  firstTokenMs,
```


### backend/internal/service/openai_gateway_messages_chat_fallback.go

冲突块 1：

```text
build:
	chatBody, err = s.applyOpenAIFastPolicyToBody(ctx, account, upstreamModel, chatBody)
	if err != nil {
		var blocked *OpenAIFastBlockedError
		if errors.As(err, &blocked) {
			writeOpenAIFastPolicyBlockedResponse(c, blocked)
		}
		return nil, err
	}
	chatBody = clampOllamaCloudUpstreamMaxTokens(account, chatBody)
	// provider 归一化、effort 策略与 fast 策略都可能改写 effort/service_tier，
	// 必须在最后一次改写后从最终出站 body 提取，保证用量日志与按档位计费倍率一致。
	reasoningEffort := extractOpenAIUpstreamReasoningEffort(chatBody, originalModel, upstreamModel, billingModel)
	serviceTier := extractOpenAIServiceTierFromBody(chatBody)
main:
	if err := validateGPT61CompatRequest(chatBody, upstreamModel); err != nil {
		writeAnthropicError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return nil, err
	}
	// Provider normalization and policy caps can both change the converted effort.
	// Use the final outbound value for usage logs and billing.
	reasoningEffort := extractOpenAIReasoningEffortFromBody(chatBody, upstreamModel, billingModel, originalModel)
	reasoningEffort = ApplyThinkingEnabledFallback(reasoningEffort, chatBody, upstreamModel)
	// Unlike forwardResponsesViaRawChatCompletions, applyOpenAIFastPolicyToBody
	// is intentionally skipped: Anthropic Messages bodies carry no service_tier,
	// so the converted Chat Completions body never contains one and the policy
	// would always be a no-op on this path.
```


### backend/internal/service/openai_gateway_passthrough.go

冲突块 1：

```text
build:
	responseBody := payload
main:
	if statusCode == http.StatusServiceUnavailable && (account == nil || account.Platform != PlatformOpenAI) {
		return statusCode, false
	}
```

冲突块 2：

```text
build:
	case http.StatusUnauthorized, http.StatusTooManyRequests, 529:
main:
	case http.StatusUnauthorized, http.StatusTooManyRequests, http.StatusServiceUnavailable, 529:
		ctx := context.Background()
		if c != nil && c.Request != nil {
			ctx = c.Request.Context()
		}
		model := firstNonEmpty(canonicalModel...)
		if model == "" {
			model = firstNonEmpty(gjson.GetBytes(payload, "model").String(), gjson.GetBytes(payload, "response.model").String())
		}
		accountHeaders := headers
		if statusCode == http.StatusTooManyRequests {
			// 普通模型的流式 429 不能继承外层 HTTP 200 的全局 quota 快照；
			// 只有 OAuth/SetupToken 的 Spark 配额 429 才需要保留 headers 读取明确的 5h/7d reset。
			accountHeaders = openAIWSSemantic429Headers(account, model, headers)
		}
		return statusCode, s.handleOpenAIAccountUpstreamError(ctx, account, statusCode, accountHeaders, payload, model)
```


### backend/internal/service/openai_gateway_service.go

冲突块 1：

```text
build:
	openaiCompatSessionResponses        sync.Map
	openaiCompatAnthropicDigestSessions sync.Map
	openAIWebSearchExecutor             func(context.Context, *Account, string, int) (*websearch.SearchResponse, string, error)
main:
	openaiCompatSessionResponses        sessionMemoryCache
	openaiCompatAnthropicDigestSessions sessionMemoryCache
```


### backend/internal/service/setting_service.go

冲突块 1：

```text
build:
	settingRepo                          SettingRepository
	defaultSubGroupReader                DefaultSubscriptionGroupReader
	proxyRepo                            ProxyRepository // for resolving websearch provider proxy URLs
	cfg                                  *config.Config
	onUpdate                             func() // Callback when settings are updated (for cache invalidation)
	version                              string // Application version
	webSearchManagerBuilder              WebSearchManagerBuilder
	antigravityUAVersionCache            atomic.Value // *cachedAntigravityUserAgentVersion
	antigravityUAVersionSF               singleflight.Group
	openAICodexUACache                   atomic.Value // *cachedOpenAICodexUserAgent
	openAICodexUASF                      singleflight.Group
	openAIResponsesLiteHeaderPolicyCache atomic.Value // *cachedOpenAIResponsesLiteHeaderPolicy
	openAIResponsesLiteHeaderPolicySF    singleflight.Group
	openAICodexVersionCache              atomic.Value // *cachedOpenAICodexClientVersion
	openAICodexVersionSF                 singleflight.Group
	claudeCodeVersionCache               atomic.Value // *cachedClaudeCodeClientVersion
	claudeCodeVersionSF                  singleflight.Group
	codexRestrictionPolicyCache          atomic.Value // *cachedCodexRestrictionPolicy
	codexRestrictionPolicySF             singleflight.Group
main:
	scopedAdminKeyRepo          ScopedAdminKeyRepository
	settingRepo                 SettingRepository
	defaultSubGroupReader       DefaultSubscriptionGroupReader
	proxyRepo                   ProxyRepository // for resolving websearch provider proxy URLs
	cfg                         *config.Config
	onUpdate                    func() // Callback when settings are updated (for cache invalidation)
	version                     string // Application version
	webSearchManagerBuilder     WebSearchManagerBuilder
	antigravityUAVersionCache   atomic.Value // *cachedAntigravityUserAgentVersion
	antigravityUAVersionSF      singleflight.Group
	openAICodexUACache          atomic.Value // *cachedOpenAICodexUserAgent
	openAICodexUASF             singleflight.Group
	openAICodexVersionCache     atomic.Value // *cachedOpenAICodexClientVersion
	openAICodexVersionSF        singleflight.Group
	claudeCodeVersionCache      atomic.Value // *cachedClaudeCodeClientVersion
	claudeCodeVersionSF         singleflight.Group
	codexRestrictionPolicyCache atomic.Value // *cachedCodexRestrictionPolicy
	codexRestrictionPolicySF    singleflight.Group
```


### backend/internal/service/settings_view.go

冲突块 1：

```text
build:
	OpenAITTFTMode                         string   // Responses first_token_ms 统计口径（默认 semantic）
	EnableFingerprintUnification           bool     // 是否统一 OAuth 账号的指纹头（默认 true）
	EnableMetadataPassthrough              bool     // 是否透传客户端原始 metadata（默认 false）
	EnableCCHSigning                       bool     // 已废弃 no-op：新版 CLI 取消 cch 签名后网关不再注入/签名 cch，开关无效果
	EnableClaudeOAuthSystemPromptInjection bool     // 是否对 Claude OAuth mimic 路径注入 Claude Code system blocks（默认 true）
	ClaudeOAuthSystemPrompt                string   // Claude OAuth mimic 路径注入的通用扩展 system prompt；空值使用内置默认
	ClaudeOAuthSystemPromptBlocks          string   // Claude OAuth mimic 路径注入的 system blocks JSON 配置；空值使用内置默认
	EnableAnthropicCacheTTL1hInjection     bool     // 是否对 Anthropic OAuth/SetupToken 请求体注入 1h cache_control ttl（默认 false）
	EnableClientDatelineNormalization      bool     // 是否对 Anthropic OAuth/SetupToken 请求体做客户端 dateline 归一化（默认 true）
	RewriteMessageCacheControl             bool     // 是否改写 messages[*].content[*].cache_control（默认 false）
	AntigravityUserAgentVersion            string   // Antigravity 上游 User-Agent 版本号；空值使用配置/默认值
	OpenAICodexUserAgent                   string   // OpenAI Codex 上游完整 User-Agent；空值使用内置默认
	OpenAIResponsesLiteHeaderBlockedModels []string // 按最终上游模型阻止 Responses Lite Header/metadata 透传的规则
	OpenAICodexClientVersion               string   // 出站声明的 Codex 客户端版本号（管理员覆写）；空值跟随自动同步值
	OpenAICodexClientVersionSynced         string   // 自动同步到的官方最新稳定版版本号（只读展示）
	OpenAICodexVersionAutoSyncEnabled      bool     // 是否启用 Codex 客户端版本号自动同步（默认 true）
	ClaudeCodeClientVersion                string   // 出站声明的 Claude Code CLI 客户端版本号（管理员覆写）；空值跟随自动同步值
	ClaudeCodeClientVersionSynced          string   // 自动同步到的官方最新版本号（只读展示）
	ClaudeCodeVersionAutoSyncEnabled       bool     // 是否启用 Claude Code 客户端版本号自动同步（默认 true）
	MinCodexVersion                        string   // codex_cli_only 最低 Codex 引擎版本；空=不检查
	MaxCodexVersion                        string   // codex_cli_only 最高 Codex 引擎版本；空=不检查
	CodexCLIOnlyBlacklist                  string   // codex_cli_only 全局黑名单 JSON（[]AllowedClientEntry，OR deny）
	CodexCLIOnlyWhitelist                  string   // codex_cli_only 全局白名单 JSON（[]AllowedClientEntry，AND allow）
	CodexCLIOnlyAllowAppServerClients      bool     // codex_cli_only App Server 开关：对未列名客户端开闸（默认 false）
	CodexCLIOnlyEngineFingerprintSignals   string   // codex_cli_only 引擎指纹门信号列表 JSON（[]EngineFingerprintSignal）
main:
	OpenAITTFTMode                         string // Responses first_token_ms 统计口径（默认 semantic）
	EnableFingerprintUnification           bool   // 是否统一 OAuth 账号的指纹头（默认 true）
	EnableMetadataPassthrough              bool   // 是否透传客户端原始 metadata（默认 false）
	EnableCCHSigning                       bool   // 已废弃 no-op：新版 CLI 取消 cch 签名后网关不再注入/签名 cch，开关无效果
	EnableClaudeOAuthSystemPromptInjection bool   // 是否对 Claude OAuth mimic 路径注入 Claude Code system blocks（默认 true）
	ClaudeOAuthSystemPrompt                string // Claude OAuth mimic 路径注入的通用扩展 system prompt；空值使用内置默认
	ClaudeOAuthSystemPromptBlocks          string // Claude OAuth mimic 路径注入的 system blocks JSON 配置；空值使用内置默认
	EnableAnthropicCacheTTL1hInjection     bool   // 是否对 Anthropic OAuth/SetupToken 请求体注入 1h cache_control ttl（默认 false）
	EnableClientDatelineNormalization      bool   // 是否对 Anthropic OAuth/SetupToken 请求体做客户端 dateline 归一化（默认 true）
	RewriteMessageCacheControl             bool   // 是否改写 messages[*].content[*].cache_control（默认 false）
	AntigravityUserAgentVersion            string // Antigravity 上游 User-Agent 版本号；空值使用配置/默认值
	OpenAICodexUserAgent                   string // OpenAI Codex 上游完整 User-Agent；空值由 Codex 客户端版本号拼出标准 CLI UA
	OpenAICodexClientVersion               string // 出站声明的 Codex 客户端版本号（管理员覆写）；空值跟随自动同步值
	OpenAICodexClientVersionSynced         string // 自动同步到的官方最新稳定版版本号（只读展示）
	OpenAICodexVersionAutoSyncEnabled      bool   // 是否启用 Codex 客户端版本号自动同步（默认 true）
	ClaudeCodeClientVersion                string // 出站声明的 Claude Code CLI 客户端版本号（管理员覆写）；空值跟随自动同步值
	ClaudeCodeClientVersionSynced          string // 自动同步到的官方最新版本号（只读展示）
	ClaudeCodeVersionAutoSyncEnabled       bool   // 是否启用 Claude Code 客户端版本号自动同步（默认 true）
	MinCodexVersion                        string // codex_cli_only 最低 Codex 引擎版本；空=不检查
	MaxCodexVersion                        string // codex_cli_only 最高 Codex 引擎版本；空=不检查
	CodexCLIOnlyBlacklist                  string // codex_cli_only 全局黑名单 JSON（[]AllowedClientEntry，OR deny）
	CodexCLIOnlyWhitelist                  string // codex_cli_only 全局白名单 JSON（[]AllowedClientEntry，AND allow）
	CodexCLIOnlyAllowAppServerClients      bool   // codex_cli_only App Server 开关：对未列名客户端开闸（默认 false）
	CodexCLIOnlyEngineFingerprintSignals   string // codex_cli_only 引擎指纹门信号列表 JSON（[]EngineFingerprintSignal）
```


### deploy/.env.example

冲突块 1：

```text
build:
# 图片生成和编辑请求等待上游响应头超时（秒）；0 表示不限制。
GATEWAY_OPENAI_IMAGES_RESPONSE_HEADER_TIMEOUT=600
# 非流式文本请求等待上游响应头超时（秒）；0 表示不限制。
GATEWAY_OPENAI_NONSTREAM_RESPONSE_HEADER_TIMEOUT=300
main:
# Disable only the fixed 10-minute account cooldown for persistent OpenAI DNS/proxy/network failures.
# YAML: gateway.disable_openai_transport_error_cooldown; false preserves the default protection.
# Ops/errors and bounded failover remain; other cooldowns, existing state and admin pauses are unchanged.
# Later requests retry broken shared egress; this is not a DNS/proxy repair or a model speed guarantee.
GATEWAY_DISABLE_OPENAI_TRANSPORT_ERROR_COOLDOWN=false
```


### deploy/config.example.yaml

冲突块 1：

```text
build:
  # 图片生成和编辑等待上游响应头的超时时间（秒，0=不限制）
  openai_images_response_header_timeout: 600
  # 非流式文本等待上游响应头的超时时间（秒，0=不限制）
  openai_nonstream_response_header_timeout: 300
main:
  # Opt out of the fixed 10-minute account cooldown for persistent OpenAI transport failures (DNS/proxy/network).
  # Env: GATEWAY_DISABLE_OPENAI_TRANSPORT_ERROR_COOLDOWN=true; default false retains protection.
  # Errors/Ops and bounded request failover remain; HTTP 401/403/429, quota/model limits,
  # stream timeouts and token-refresh cooldowns are unchanged. Existing cooldowns and admin pauses are not cleared.
  # With shared egress still broken, later requests retry it: this does not repair DNS/proxies or guarantee faster models.
  disable_openai_transport_error_cooldown: false
```


### deploy/docker-compose.dev.yml

冲突块 1：

```text
build:
      - GATEWAY_OPENAI_IMAGES_RESPONSE_HEADER_TIMEOUT=${GATEWAY_OPENAI_IMAGES_RESPONSE_HEADER_TIMEOUT:-600}
      - GATEWAY_OPENAI_NONSTREAM_RESPONSE_HEADER_TIMEOUT=${GATEWAY_OPENAI_NONSTREAM_RESPONSE_HEADER_TIMEOUT:-300}
main:
      - GATEWAY_DISABLE_OPENAI_TRANSPORT_ERROR_COOLDOWN=${GATEWAY_DISABLE_OPENAI_TRANSPORT_ERROR_COOLDOWN:-false}
```


### deploy/docker-compose.local.yml

冲突块 1：

```text
build:
      - GATEWAY_OPENAI_IMAGES_RESPONSE_HEADER_TIMEOUT=${GATEWAY_OPENAI_IMAGES_RESPONSE_HEADER_TIMEOUT:-600}
      - GATEWAY_OPENAI_NONSTREAM_RESPONSE_HEADER_TIMEOUT=${GATEWAY_OPENAI_NONSTREAM_RESPONSE_HEADER_TIMEOUT:-300}
main:
      - GATEWAY_DISABLE_OPENAI_TRANSPORT_ERROR_COOLDOWN=${GATEWAY_DISABLE_OPENAI_TRANSPORT_ERROR_COOLDOWN:-false}
```


### deploy/docker-compose.standalone.yml

冲突块 1：

```text
build:
      - GATEWAY_OPENAI_IMAGES_RESPONSE_HEADER_TIMEOUT=${GATEWAY_OPENAI_IMAGES_RESPONSE_HEADER_TIMEOUT:-600}
      - GATEWAY_OPENAI_NONSTREAM_RESPONSE_HEADER_TIMEOUT=${GATEWAY_OPENAI_NONSTREAM_RESPONSE_HEADER_TIMEOUT:-300}
main:
      - GATEWAY_DISABLE_OPENAI_TRANSPORT_ERROR_COOLDOWN=${GATEWAY_DISABLE_OPENAI_TRANSPORT_ERROR_COOLDOWN:-false}
```


### deploy/docker-compose.yml

冲突块 1：

```text
build:
      - GATEWAY_OPENAI_IMAGES_RESPONSE_HEADER_TIMEOUT=${GATEWAY_OPENAI_IMAGES_RESPONSE_HEADER_TIMEOUT:-600}
      - GATEWAY_OPENAI_NONSTREAM_RESPONSE_HEADER_TIMEOUT=${GATEWAY_OPENAI_NONSTREAM_RESPONSE_HEADER_TIMEOUT:-300}
main:
      - GATEWAY_DISABLE_OPENAI_TRANSPORT_ERROR_COOLDOWN=${GATEWAY_DISABLE_OPENAI_TRANSPORT_ERROR_COOLDOWN:-false}
```


### frontend/src/components/account/EditAccountModal.vue

冲突块 1：

```text
build:
      newExtra = applyOpenAICompatibilityExtra(newExtra, {
        responsesMode:
          props.account.type === 'apikey' && openAITextGenerationCapabilityEnabled.value
            ? openAIResponsesMode.value
            : null,
        jsonSchemaToJSONObject:
          props.account.type === 'apikey' ? openAIJSONSchemaDowngradeEnabled.value : null,
        webSearchEmulation:
          props.account.type === 'apikey' ? webSearchEmulationMode.value : null
      })
      if (props.account.type === 'apikey' && openAIImagesUrlToB64JsonEnabled.value) {
        newExtra.images_url_to_b64_json = true
      } else {
        delete newExtra.images_url_to_b64_json
      }
      newExtra = applyAccountCodexWebSearchBridgeExtra(
        newExtra,
        props.account.type === 'apikey' ? codexWebSearchBridgeMode.value : 'inherit'
      )
      if (autoPause5hThreshold.value != null && autoPause5hThreshold.value > 0) {
        newExtra.auto_pause_5h_threshold = autoPause5hThreshold.value / 100
      } else {
        delete newExtra.auto_pause_5h_threshold
      }
      if (autoPause7dThreshold.value != null && autoPause7dThreshold.value > 0) {
        newExtra.auto_pause_7d_threshold = autoPause7dThreshold.value / 100
      } else {
        delete newExtra.auto_pause_7d_threshold
      }
      if (autoPause5hDisabled.value) {
        newExtra.auto_pause_5h_disabled = true
      } else {
        delete newExtra.auto_pause_5h_disabled
      }
      if (autoPause7dDisabled.value) {
        newExtra.auto_pause_7d_disabled = true
      } else {
        delete newExtra.auto_pause_7d_disabled
      }
      if (props.account.type === 'oauth' && !isSparkShadow.value) {
        newExtra.auto_reset_credit_enabled = autoResetCreditEnabled.value
        newExtra.auto_reset_credit_5h_threshold = autoResetCredit5hThreshold.value / 100
        newExtra.auto_reset_credit_7d_threshold = autoResetCredit7dThreshold.value / 100
      }
      // 运行态只允许后端服务更新，账号编辑不得回写旧状态。
      delete newExtra.codex_auto_reset_credit_state
main:
		if (props.account.type === 'apikey') {
        if (!openAITextGenerationCapabilityEnabled.value || openAIResponsesMode.value === 'auto') {
          delete newExtra.openai_responses_mode
        } else {
          newExtra.openai_responses_mode = openAIResponsesMode.value
        }
        if (openAIImagesUrlToB64JsonEnabled.value) {
          newExtra.images_url_to_b64_json = true
        } else {
          delete newExtra.images_url_to_b64_json
        }
        if (webSearchEmulationMode.value === 'default') {
          delete newExtra.web_search_emulation
        } else {
          newExtra.web_search_emulation = webSearchEmulationMode.value
        }
		}
		applyOpenAIImageSizeTiersToExtra(newExtra, openAIImageSizeTiers.value)
		if (autoPause5hThreshold.value != null && autoPause5hThreshold.value > 0) {
			newExtra.auto_pause_5h_threshold = autoPause5hThreshold.value / 100
		} else {
			delete newExtra.auto_pause_5h_threshold
		}
		if (autoPause7dThreshold.value != null && autoPause7dThreshold.value > 0) {
			newExtra.auto_pause_7d_threshold = autoPause7dThreshold.value / 100
		} else {
			delete newExtra.auto_pause_7d_threshold
		}
		if (autoPause5hDisabled.value) {
			newExtra.auto_pause_5h_disabled = true
		} else {
			delete newExtra.auto_pause_5h_disabled
		}
		if (autoPause7dDisabled.value) {
			newExtra.auto_pause_7d_disabled = true
		} else {
			delete newExtra.auto_pause_7d_disabled
		}
		if (props.account.type === 'oauth' && !isSparkShadow.value) {
			newExtra.auto_reset_credit_enabled = autoResetCreditEnabled.value
			newExtra.auto_reset_credit_5h_threshold = autoResetCredit5hThreshold.value / 100
			newExtra.auto_reset_credit_7d_threshold = autoResetCredit7dThreshold.value / 100
		}
		// 运行态只允许后端服务更新，账号编辑不得回写旧状态。
		delete newExtra.codex_auto_reset_credit_state
```

冲突块 2：

```text
build:
    // Responses Lite 降级开关（build 私有）：API Key 且 openai / 国产供应商账号，
    // 在各平台 extra 块之后统一写入，避免被平台块从账号快照重建时覆盖。
    if (canConfigureResponsesLiteDowngrade.value) {
      const currentExtra = (updatePayload.extra as Record<string, unknown>) || (props.account.extra as Record<string, unknown>) || {}
      updatePayload.extra = applyResponsesLiteDowngradeExtra(currentExtra, responsesLiteDowngradeEnabled.value)
    }

    // Alpha Search 经上游 Responses 执行开关（build 私有）：仅 OpenAI API Key 账号。
    if (canConfigureAlphaSearchViaResponses.value) {
      const currentExtra = (updatePayload.extra as Record<string, unknown>) || (props.account.extra as Record<string, unknown>) || {}
      updatePayload.extra = applyAlphaSearchViaResponsesExtra(currentExtra, alphaSearchViaResponsesEnabled.value)
    }

    // Grok 复用 OpenAI Responses 路由覆盖键，仅增删该键，避免覆盖 OAuth 额度快照等 extra 信息。
    if (props.account.platform === 'grok' && (props.account.type === 'oauth' || props.account.type === 'apikey')) {
      const currentExtra = (updatePayload.extra as Record<string, unknown>) || (props.account.extra as Record<string, unknown>) || {}
      const newExtra = applyGrokForceChatExtra(currentExtra, openAIResponsesMode.value)
      updatePayload.extra = newExtra
    }

    // For apikey/bedrock accounts, handle quota_limit in extra
    if (props.account.type === 'apikey' || props.account.type === 'bedrock') {
main:
    // For apikey/bedrock/service_account accounts, handle quota_limit in extra
    if (supportsAccountQuota(props.account.type)) {
```


### frontend/src/i18n/locales/en/admin/settings.ts

冲突块 1：

```text
build:
		antigravityUserAgentVersionHint: 'Leave empty to use ANTIGRAVITY_USER_AGENT_VERSION or the built-in default 1.23.2; when set, the admin setting takes precedence.',
		openaiCodexUserAgent: 'OpenAI Codex UA',
		openaiCodexUserAgentPlaceholder: 'codex-tui/0.146.1 (Ubuntu 22.4.0; x86_64) WindowsTerminal (codex-tui; 0.146.1)',
        openaiCodexUserAgentHint: 'The full Codex User-Agent used for all outbound requests, for customizing the OS / arch / terminal fingerprint. Leave empty to build the standard codex-tui identity from the version below (recommended). If set, both the leading and trailing version declarations are synchronized to the version below, so the UA never stays pinned to the release entered here — under capacity pressure the upstream sheds load by client identity and drops stale or non-official identities first with server_is_overloaded.',
main:
        antigravityUserAgentVersionHint: 'Leave empty to use ANTIGRAVITY_USER_AGENT_VERSION or the built-in default 1.23.2; when set, the admin setting takes precedence.',
        openaiCodexUserAgent: 'OpenAI Codex UA',
        openaiCodexUserAgentPlaceholder: 'codex_cli_rs/0.146.1 (Ubuntu 22.4.0; x86_64) WindowsTerminal',
        openaiCodexUserAgentHint: 'The full Codex User-Agent used for all outbound requests, for customizing the OS / arch / terminal fingerprint. Leave empty to build the standard codex_cli_rs identity from the version below (recommended). If set, both the leading and trailing version declarations are synchronized to the version below, so the UA never stays pinned to the release entered here — upstream identity and version risk controls may return server_is_overloaded for stale or non-official identities.',
```


### frontend/src/i18n/locales/zh/admin/settings.ts

冲突块 1：

```text
build:
		antigravityUserAgentVersionHint: '留空时使用 ANTIGRAVITY_USER_AGENT_VERSION 或内置默认值 1.23.2；填写后后台设置优先。',
		openaiCodexUserAgent: 'OpenAI Codex UA',
		openaiCodexUserAgentPlaceholder: 'codex-tui/0.146.1 (Ubuntu 22.4.0; x86_64) WindowsTerminal (codex-tui; 0.146.1)',
        openaiCodexUserAgentHint: '出站统一使用的完整 Codex User-Agent，用于自定义 OS / 架构 / 终端指纹。留空则按下方版本号拼出标准 codex-tui 形态（推荐）。填写后首段和尾部的版本号仍会被下方版本号同步覆盖，避免这条 UA 停在填写时的旧版本——上游在容量紧张时按客户端身份分优先级降载，陈旧或非官方形态的身份会被优先丢弃并回 server_is_overloaded。',
main:
        antigravityUserAgentVersionHint: '留空时使用 ANTIGRAVITY_USER_AGENT_VERSION 或内置默认值 1.23.2；填写后后台设置优先。',
        openaiCodexUserAgent: 'OpenAI Codex UA',
        openaiCodexUserAgentPlaceholder: 'codex_cli_rs/0.146.1 (Ubuntu 22.4.0; x86_64) WindowsTerminal',
        openaiCodexUserAgentHint: '出站统一使用的完整 Codex User-Agent，用于自定义 OS / 架构 / 终端指纹。留空则按下方版本号拼出标准 codex_cli_rs 形态（推荐）。填写后首段和尾部的版本号仍会被下方版本号同步覆盖，避免这条 UA 停在填写时的旧版本——上游会根据客户端身份和版本进行风控分层，陈旧或非官方形态可能返回 server_is_overloaded。',
```


### frontend/src/views/admin/ChannelsView.vue

冲突块 1：

```text
build:
            <ChannelWebSearchEmulationToggle
              v-if="supportsChannelWebSearchEmulation(section.platform) && webSearchGlobalEnabled"
              v-model="section.web_search_emulation"
            />
main:
            <!-- Web Search Emulation (platform whitelist, hidden when global disabled) -->
            <div v-if="WEB_SEARCH_EMULATION_PLATFORMS.has(section.platform) && webSearchGlobalEnabled" class="border-t border-gray-200 pt-3 dark:border-dark-600">
              <div class="flex items-center justify-between">
                <div>
                  <label class="text-xs font-medium text-gray-700 dark:text-gray-300">
                    {{ t('admin.channels.form.webSearchEmulation') }}
                  </label>
                  <p class="mt-0.5 text-[11px] text-red-500 dark:text-red-400">
                    {{ t('admin.channels.form.webSearchEmulationHint') }}
                  </p>
                </div>
                <Toggle v-model="section.web_search_emulation" />
              </div>
            </div>
```

冲突块 2：

```text
build:
  applyChannelWebSearchFeatureConfig(featuresConfig, form.platforms)
  applyChannelCodexWebSearchBridgeConfig(featuresConfig, form.platforms)
  applyChannelCodexImageBridgeFeatureConfig(featuresConfig, form.platforms)
main:
  // Collect web_search_emulation（whitelist platforms only: the UI is the single
  // source of truth, so every whitelisted section is written explicitly — false
  // included — to avoid leaving a stale `true` behind for disabled sections）.
  const wsEmulation: Record<string, boolean> = {}
  for (const section of form.platforms) {
    if (!WEB_SEARCH_EMULATION_PLATFORMS.has(section.platform)) continue
    wsEmulation[section.platform] = section.enabled ? !!section.web_search_emulation : false
  }
  if (Object.keys(wsEmulation).length > 0) {
    featuresConfig.web_search_emulation = wsEmulation
  } else {
    delete featuresConfig.web_search_emulation
  }

  const codexImageGenerationBridge: Record<string, boolean> = {}
  for (const section of form.platforms) {
    if (!section.enabled) continue
    if (section.platform === 'openai') {
      codexImageGenerationBridge[section.platform] = !!section.codex_image_generation_bridge
    }
  }
  if (Object.keys(codexImageGenerationBridge).length > 0) {
    featuresConfig.codex_image_generation_bridge = codexImageGenerationBridge
  } else {
    delete featuresConfig.codex_image_generation_bridge
  }
```


## 语义复核结论

- cyber：handler 明确身份写计划、service v2 哈希与白名单沿用 build；存储边界投影到 main 新 CyberSessionBlock，保留有界 Redis 批次。请求路径保持 transcript 文件删除。
- 配置：超时分档和冷却开关并存，默认值、校验、环境变量和四份 Compose 已成对接续。
- 协议：新 Claude 模型保留搜索引用；buffer 化保留 Read 参数清洗与确定性工具 ID；拒绝内容、annotations 数组与最终 response 保持同一 wire 契约。
- 模型/计费：Terra 与 build 的 Sol/5.6 max 档位、实际出站 effort、Fast、cache-write TTL、Grok 响应模型观察一起保留；对映射与 disabled 的先后顺序追加实际出站断言。
- 自动合并 handler/DTO/server：Responses Lite 设置字段从保存、缓存、DTO 到页面接续；GIF 路由和 AnySearch provider 接线保留；main 的 scoped admin/MCP 认证和注册黑名单接线保留。
- 自动合并 service：GIF 初始与重试调用、模型提示词 owner、协议 resolver、Lite HTTP/WS 策略、流式停调、会话有界缓存与 main 搜索激活重连逐项核对。迁出的 debug 与模型映射逻辑仍由领域文件拥有。
- 自动合并 repository/Ent：Grok 配额 snapshot 中性更新保留；超时 profile 池隔离保留；Ent 与 Wire 重新生成，结果相对 main 仅保留 build 的 usage_log 字段说明。
- 自动合并 frontend：账号创建/编辑/批量更新保留新尺寸档位与 build extra；设置页领域组件与 main 保存浮标并存；所有 locale override 的有效语义已比较，渠道提示继续按当前平台描述。
- 其它交集：.gitignore 保留 cyber 文档及 Flower 记录规则；README 已有说明保留；go.sum 保留 Wire 校验和；定价 JSON 保留 build 的 max 档位声明；embed 测试保留实际 SVG 资源。
- 单边接入：main 新模型、MCP、scoped admin keys、图像计费和缓存 TTL 的现有专项测试纳入全量 unit；数据库集成测试覆盖迁移幂等、实际 schema、TTL 读写和 MCP 持久化。
- SQL：迁移器按完整 filename 排序及记录 checksum；两个 249 文件分别执行。本轮保持六个新 SQL 与 main 的 blob 完全一致。250 文件的尾部空行沿上游保留，避免改变已发布 SQL 的 checksum。

以上复选框表示代码与接线的静态复核完成。最终运行结果以 implement.md 中的验证记录为准。
