package dto

// AntigravityGIFCompatibilitySettings 是反重力 GIF 多帧兼容的管理端设置 DTO。
type AntigravityGIFCompatibilitySettings struct {
	Enabled         bool `json:"enabled"`
	MaxFramesPerGIF int  `json:"max_frames_per_gif"`
}
