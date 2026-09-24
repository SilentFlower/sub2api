package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAITextHandlersMarkNonstreamRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name string
		path string
		body string
		call func(*OpenAIGatewayHandler, *gin.Context)
	}{
		{
			name: "responses",
			path: "/v1/responses",
			body: `{"model":"gpt-5","previous_response_id":"msg_invalid"}`,
			call: (*OpenAIGatewayHandler).Responses,
		},
		{
			name: "responses_compact",
			path: "/v1/responses/compact",
			body: `{"model":"gpt-5","previous_response_id":"msg_invalid"}`,
			call: (*OpenAIGatewayHandler).Responses,
		},
		{
			name: "responses_body_compact",
			path: "/v1/responses",
			body: `{"model":"gpt-5","previous_response_id":"msg_invalid","input":[{"type":"compaction_trigger"}]}`,
			call: (*OpenAIGatewayHandler).Responses,
		},
		{
			name: "chat_completions",
			path: "/v1/chat/completions",
			body: `{"model":"gpt-image-2","messages":[{"role":"user","content":"draw"}]}`,
			call: (*OpenAIGatewayHandler).ChatCompletions,
		},
	} {
		for _, stream := range []struct {
			name  string
			field string
			want  bool
		}{
			{name: "omitted", field: "", want: true},
			{name: "false", field: `,"stream":false`, want: true},
			{name: "true", field: `,"stream":true`, want: false},
		} {
			t.Run(tc.name+"/"+stream.name, func(t *testing.T) {
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				body := []byte(tc.body[:len(tc.body)-1] + stream.field + "}")
				c.Request = httptest.NewRequest(http.MethodPost, tc.path, bytes.NewReader(body))
				c.Request.Header.Set("Content-Type", "application/json")
				setImageChatTestAuth(c)
				h := newOpenAIHandlerForPreviousResponseIDValidation(t, nil)
				tc.call(h, c)
				require.Equal(t, http.StatusBadRequest, recorder.Code)
				require.Equal(t, stream.want, service.OpenAINonstreamTextRequestFromContext(c.Request.Context()))
			})
		}
	}
}
