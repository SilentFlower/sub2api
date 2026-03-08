package antigravityls

import (
	"encoding/json"
	"fmt"
	"strings"
)

// translatedToolCall 表示转换后返回给 Claude 客户端的工具调用。
// 某些 AG 工具会收敛到同一个客户端工具，因此这里允许一对多转换。
type translatedToolCall struct {
	Name  string
	Input any
	ID    string
}

var clientBuiltinToAG = map[string]string{
	"read":  "view_file",
	"write": "write_to_file",
	"edit":  "replace_file_content",
	"glob":  "find_by_name",
	"grep":  "grep_search",
	"bash":  "run_command",
	"lsp":   "view_file_outline",
}

func translateClientToolNameToAG(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return ""
	}
	if mapped, ok := clientBuiltinToAG[trimmed]; ok {
		return mapped
	}
	if strings.Contains(trimmed, "__") || strings.Contains(trimmed, "_") {
		return "mcp_tool"
	}
	return trimmed
}

// TranslateClientToolNameToAGForPrompt 将客户端工具名转换回 AG 工具名，供提示词转录使用。
func TranslateClientToolNameToAGForPrompt(name string) string {
	return translateClientToolNameToAG(name)
}

func translateAGToolCall(tc ToolCall) []translatedToolCall {
	args := decodeToolArgs(tc.ArgumentsJSON)
	switch tc.Name {
	case "view_file":
		return []translatedToolCall{{Name: "read", ID: tc.ID, Input: map[string]any{
			"filePath": pickString(args, "AbsolutePath", "filePath", "file_path", "path"),
			"offset":   pickInt(args, 1, "StartLine", "startLine", "offset"),
			"limit":    pickReadLimit(args),
		}}}
	case "view_file_outline":
		return []translatedToolCall{{Name: "lsp", ID: tc.ID, Input: map[string]any{
			"filePath":  pickString(args, "AbsolutePath", "filePath", "file_path", "path"),
			"operation": "documentSymbol",
			"line":      1,
			"character": 0,
		}}}
	case "write_to_file":
		return []translatedToolCall{{Name: "write", ID: tc.ID, Input: map[string]any{
			"filePath": pickString(args, "TargetFile", "filePath", "file_path", "path"),
			"content":  pickString(args, "CodeContent", "content", "code"),
		}}}
	case "replace_file_content":
		return []translatedToolCall{{Name: "edit", ID: tc.ID, Input: map[string]any{
			"filePath":  pickString(args, "TargetFile", "filePath", "file_path", "path"),
			"oldString": pickString(args, "TargetContent", "oldString", "old_string", "target"),
			"newString": pickString(args, "ReplacementContent", "newString", "new_string", "replacement"),
		}}}
	case "multi_replace_file_content":
		return []translatedToolCall{{Name: "edit", ID: tc.ID, Input: map[string]any{
			"filePath":  pickString(args, "TargetFile", "filePath", "file_path", "path"),
			"oldString": pickNestedChunkString(args, "ReplacementChunks", "TargetContent"),
			"newString": pickNestedChunkString(args, "ReplacementChunks", "ReplacementContent"),
		}}}
	case "find_by_name":
		return []translatedToolCall{{Name: "glob", ID: tc.ID, Input: map[string]any{
			"pattern": pickString(args, "Pattern", "pattern"),
			"path":    pickString(args, "SearchDirectory", "path", "directory"),
		}}}
	case "grep_search":
		return []translatedToolCall{{Name: "grep", ID: tc.ID, Input: buildGrepInput(args)}}
	case "list_dir":
		return []translatedToolCall{{Name: "read", ID: tc.ID, Input: map[string]any{
			"filePath": pickString(args, "DirectoryPath", "filePath", "path", "directory"),
		}}}
	case "run_command":
		input := map[string]any{
			"command":     pickString(args, "CommandLine", "command", "cmd"),
			"description": "Run command",
		}
		if cwd := pickString(args, "Cwd", "cwd", "workdir"); cwd != "" {
			input["workdir"] = cwd
		}
		return []translatedToolCall{{Name: "bash", ID: tc.ID, Input: input}}
	case "command_status":
		return []translatedToolCall{{Name: "bash", ID: tc.ID, Input: map[string]any{
			"command":     "echo 'Command completed'",
			"description": "Check command status",
		}}}
	case "send_command_input":
		cmd := strings.TrimRight(pickString(args, "Input", "input"), "\n")
		if cmd == "" {
			cmd = "echo done"
		}
		return []translatedToolCall{{Name: "bash", ID: tc.ID, Input: map[string]any{
			"command":     cmd,
			"description": "Send input to command",
		}}}
	case "read_terminal":
		return []translatedToolCall{{Name: "bash", ID: tc.ID, Input: map[string]any{
			"command":     "echo 'Terminal output unavailable in this environment'",
			"description": "Read terminal output",
		}}}
	case "mcp_tool":
		server := pickString(args, "ServerName", "serverName")
		tool := pickString(args, "ToolName", "toolName")
		toolName := strings.Trim(strings.Join([]string{server, tool}, "__"), "_")
		if toolName == "" {
			toolName = "mcp_tool"
		}
		var input any = map[string]any{}
		if rawArgs, ok := args["Arguments"]; ok {
			switch typed := rawArgs.(type) {
			case string:
				if err := json.Unmarshal([]byte(typed), &input); err != nil {
					input = map[string]any{"arguments": typed}
				}
			default:
				input = typed
			}
		}
		return []translatedToolCall{{Name: toolName, ID: tc.ID, Input: input}}
	default:
		return []translatedToolCall{{Name: tc.Name, ID: tc.ID, Input: args}}
	}
}

func decodeToolArgs(raw string) map[string]any {
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return map[string]any{"raw": raw}
	}
	return parsed
}

func pickString(args map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := args[key]; ok {
			switch typed := value.(type) {
			case string:
				return typed
			case fmt.Stringer:
				return typed.String()
			}
		}
	}
	return ""
}

func pickInt(args map[string]any, fallback int, keys ...string) int {
	for _, key := range keys {
		if value, ok := args[key]; ok {
			switch typed := value.(type) {
			case float64:
				return int(typed)
			case int:
				return typed
			case int64:
				return int(typed)
			}
		}
	}
	return fallback
}

func pickReadLimit(args map[string]any) int {
	start := pickInt(args, 1, "StartLine", "startLine", "offset")
	end := pickInt(args, 0, "EndLine", "endLine")
	if end >= start && end > 0 {
		return end - start + 1
	}
	if limit := pickInt(args, 0, "limit", "Limit"); limit > 0 {
		return limit
	}
	return 800
}

func pickNestedChunkString(args map[string]any, key string, chunkField string) string {
	chunks, ok := args[key].([]any)
	if !ok || len(chunks) == 0 {
		return ""
	}
	first, ok := chunks[0].(map[string]any)
	if !ok {
		return ""
	}
	return pickString(first, chunkField)
}

func buildGrepInput(args map[string]any) map[string]any {
	pattern := pickString(args, "Query", "query", "pattern", "search")
	out := map[string]any{"pattern": pattern}
	if path := pickString(args, "SearchPath", "path"); path != "" {
		out["path"] = path
	}
	if include := pickFirstString(args, "Includes", "include"); include != "" {
		out["include"] = include
	}
	if mpl := pickInt(args, 0, "MatchPerLine", "matchPerLine"); mpl > 0 {
		out["output_mode"] = "content"
	}
	return out
}

func pickFirstString(args map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := args[key]; ok {
			switch typed := value.(type) {
			case string:
				return typed
			case []any:
				if len(typed) == 0 {
					continue
				}
				if first, ok := typed[0].(string); ok {
					return first
				}
			}
		}
	}
	return ""
}
