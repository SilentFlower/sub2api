//go:build unit

package websearch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAnySearchProviderSearchStructuredResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer secret", r.Header.Get("Authorization"))
		var request map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		require.Equal(t, "tools/call", request["method"])
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"structuredContent":{"results":[{"url":"https://example.com","title":"Example","snippet":"Result"}]}}}`))
	}))
	defer server.Close()

	oldEndpoint := anySearchEndpoint
	anySearchEndpoint = server.URL
	defer func() { anySearchEndpoint = oldEndpoint }()

	provider := NewAnySearchProvider("secret", server.Client())
	response, err := provider.Search(context.Background(), SearchRequest{Query: "query", MaxResults: 3})
	require.NoError(t, err)
	require.Len(t, response.Results, 1)
	require.Equal(t, "https://example.com", response.Results[0].URL)
}

func TestAnySearchProviderSearchMCPTextJSONWithoutAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Empty(t, r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"{\"items\":[{\"link\":\"https://example.org\",\"name\":\"Example\",\"description\":\"Text\"}]}"}]}}`))
	}))
	defer server.Close()

	oldEndpoint := anySearchEndpoint
	anySearchEndpoint = server.URL
	defer func() { anySearchEndpoint = oldEndpoint }()

	response, err := NewAnySearchProvider("", server.Client()).Search(context.Background(), SearchRequest{Query: "query"})
	require.NoError(t, err)
	require.Len(t, response.Results, 1)
	require.Equal(t, "Example", response.Results[0].Title)
}

func TestAnySearchProviderRedactsSecretAndQueryFromError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`secret query failed`))
	}))
	defer server.Close()

	oldEndpoint := anySearchEndpoint
	anySearchEndpoint = server.URL
	defer func() { anySearchEndpoint = oldEndpoint }()

	_, err := NewAnySearchProvider("secret", server.Client()).Search(context.Background(), SearchRequest{Query: "query"})
	require.Error(t, err)
	require.NotContains(t, err.Error(), "secret")
	require.NotContains(t, err.Error(), "query")
}

func TestAnySearchProviderRedactsLongSecretBeforeTruncation(t *testing.T) {
	apiKey := strings.Repeat("secret-key-", 35)
	query := strings.Repeat("private-query-", 30)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(apiKey + " " + query + " failed"))
	}))
	defer server.Close()

	oldEndpoint := anySearchEndpoint
	anySearchEndpoint = server.URL
	defer func() { anySearchEndpoint = oldEndpoint }()

	_, err := NewAnySearchProvider(apiKey, server.Client()).Search(context.Background(), SearchRequest{Query: query})
	require.Error(t, err)
	require.NotContains(t, err.Error(), apiKey[:80])
	require.NotContains(t, err.Error(), query[:80])
	require.Contains(t, err.Error(), "[REDACTED]")
}

func TestAnySearchProviderRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", maxResponseSize+1)))
	}))
	defer server.Close()

	oldEndpoint := anySearchEndpoint
	anySearchEndpoint = server.URL
	defer func() { anySearchEndpoint = oldEndpoint }()

	_, err := NewAnySearchProvider("", server.Client()).Search(context.Background(), SearchRequest{Query: "query"})
	require.ErrorContains(t, err, "response body exceeds")
	require.NotContains(t, err.Error(), strings.Repeat("x", 100))
}
