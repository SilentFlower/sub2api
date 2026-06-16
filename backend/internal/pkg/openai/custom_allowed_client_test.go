package openai

import "testing"

func TestMatchCustomUserAgentPrefixes(t *testing.T) {
	tests := []struct {
		name     string
		ua       string
		patterns []string
		want     bool
	}{
		{name: "前缀命中", ua: "my-client/1.2.3", patterns: []string{"my-client/"}, want: true},
		{name: "大小写不敏感", ua: "My-Client/1.2.3", patterns: []string{"my-client/"}, want: true},
		{name: "pattern 两侧空白被裁剪", ua: "my-client/1.2.3", patterns: []string{"  my-client/  "}, want: true},
		{name: "空白 pattern 不放行", ua: "my-client/1.2.3", patterns: []string{"", "  "}, want: false},
		{name: "空 UA 不放行", ua: "", patterns: []string{"my-client/"}, want: false},
		{name: "单星号匹配任意非空 UA", ua: "curl/8.0", patterns: []string{"*"}, want: true},
		{name: "星号后缀仍要求固定前缀", ua: "my-client/1.2.3", patterns: []string{"my-client/*"}, want: true},
		{name: "星号中段匹配", ua: "my-client beta/1.2.3", patterns: []string{"my-client*1.2"}, want: true},
		{name: "星号前缀允许任意前导字符", ua: "Mozilla/5.0 my-client/1.2.3", patterns: []string{"*my-client/1.2"}, want: true},
		{name: "多个 pattern 任一命中", ua: "my-client/1.2.3", patterns: []string{"other/", "my-client/"}, want: true},
		{name: "无匹配", ua: "curl/8.0", patterns: []string{"my-client/"}, want: false},
		{name: "固定片段顺序必须一致", ua: "my-client bar foo", patterns: []string{"my-client*foo*bar"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MatchCustomUserAgentPrefixes(tt.ua, tt.patterns); got != tt.want {
				t.Fatalf("MatchCustomUserAgentPrefixes(%q, %v) = %v, want %v", tt.ua, tt.patterns, got, tt.want)
			}
		})
	}
}
