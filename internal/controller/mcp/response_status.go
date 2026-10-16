package mcp

import (
	"bytes"
	"encoding/json"
	"strings"
)

// JSON API endpoints may return an empty HTML page with HTTP 200 when an API
// version is unavailable. Do not present that response as a successful query.
// Preserve legitimate empty 204 and non-JSON endpoints without output schemas.
func responseFailed(status int, body []byte, expectsJSON bool) bool {
	if status < 200 || status >= 300 {
		return true
	}
	if status == 204 || !expectsJSON {
		return false
	}
	body = bytes.TrimSpace(body)
	if len(body) == 0 || !json.Valid(body) || bytes.Equal(body, []byte("null")) {
		return true
	}
	var envelope map[string]any
	if json.Unmarshal(body, &envelope) != nil {
		return false
	}
	for _, key := range []string{"status", "result"} {
		if value, ok := envelope[key].(string); ok {
			switch strings.ToLower(value) {
			case "fail", "failed", "error":
				return true
			}
		}
	}
	if value, ok := envelope["result"].(bool); ok && !value {
		return true
	}
	if value, ok := envelope["error"]; ok && value != nil {
		switch v := value.(type) {
		case string:
			return v != ""
		case bool:
			return v
		case float64:
			return v != 0
		default:
			return true
		}
	}
	return false
}
