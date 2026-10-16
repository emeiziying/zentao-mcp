package mcp

import (
	"bytes"
	"encoding/json"
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
	return len(body) == 0 || !json.Valid(body) || bytes.Equal(body, []byte("null"))
}
