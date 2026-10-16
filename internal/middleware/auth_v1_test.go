package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestV1RedirectCannotLeakTokenOrReplayMutation(t *testing.T) {
	for _, status := range []int{301, 302, 303, 307, 308} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var targetCalls, sourceCalls atomic.Int32
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				targetCalls.Add(1)
				t.Error("redirect target must not receive any request or credentials")
			}))
			defer target.Close()
			source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				sourceCalls.Add(1)
				if r.Method != "POST" || r.Header.Get("Token") != "fake-private-token" {
					t.Error("incorrect source request")
				}
				w.Header().Set("Location", target.URL+"/tasks")
				w.WriteHeader(status)
			}))
			defer source.Close()
			ctx := WithAuthorizationSource(WithAuthorization(context.Background(), "fake-private-token"), true)
			req, err := http.NewRequestWithContext(ctx, "POST", source.URL+"/tasks", strings.NewReader(`{"name":"fixture"}`))
			if err != nil {
				t.Fatal(err)
			}
			response, err := WithBearerAuth(source.Client()).Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != status || sourceCalls.Load() != 1 || targetCalls.Load() != 0 {
				t.Fatalf("unexpected redirect/retry behavior: status=%d source=%d target=%d", response.StatusCode, sourceCalls.Load(), targetCalls.Load())
			}
			if req.Header.Get("Token") != "" {
				t.Error("transport mutated input request headers")
			}
		})
	}
}

func TestV1AuthTransportTimeout(t *testing.T) {
	if got := WithBearerAuth(&http.Client{}).Timeout; got != 30*time.Second {
		t.Fatalf("default timeout: %s", got)
	}
	if got := WithBearerAuth(&http.Client{Timeout: 7 * time.Second}).Timeout; got != 7*time.Second {
		t.Fatalf("configured timeout lost: %s", got)
	}
}
