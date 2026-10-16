package mcp

import "testing"

func TestResponseFailed(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		json   bool
		failed bool
	}{
		{"v1 profile", 200, `{"profile":{"id":1}}`, true, false},
		{"empty collection", 200, `{"tasks":[]}`, true, false},
		{"unsupported API empty 200", 200, "", true, true},
		{"HTML login page", 200, "<html>Login</html>", true, true},
		{"null response", 200, "null", true, true},
		{"unauthorized", 401, `{"error":"unauthorized"}`, true, true},
		{"server error", 500, "failure", false, true},
		{"legitimate no content", 204, "", true, false},
		{"plain text endpoint", 200, "OK", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := responseFailed(tc.status, []byte(tc.body), tc.json); got != tc.failed {
				t.Fatalf("got %v want %v", got, tc.failed)
			}
		})
	}
}
