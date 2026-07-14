package websearch

import "context"

// Provider is the interface every search backend must implement.
type Provider interface {
	// Name 返回 provider 的稳定标识。
	Name() string
	// Search 执行一次网页搜索并返回归一化结果。
	Search(ctx context.Context, req SearchRequest) (*SearchResponse, error)
}
