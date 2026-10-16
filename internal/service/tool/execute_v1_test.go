package tool

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/merzzzl/openapi-mcp-server/internal/middleware"
	"github.com/merzzzl/openapi-mcp-server/internal/models"
	"github.com/merzzzl/openapi-mcp-server/internal/repository/proxy"
)

// These operations use only a local fake upstream: no real business writes.
func TestV1ExecuteHTTPMethods(t *testing.T) {
	for _, method := range []string{"GET", "POST", "PUT", "DELETE"} {
		t.Run(method, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != method || r.URL.Path != "/zentao/api.php/v1/tasks/42" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				if r.URL.Query().Get("page") != "2" || r.URL.Query().Get("limit") != "20" || r.URL.Query().Get("search") != "待办 & review" {
					t.Errorf("incorrect query: %v", r.URL.Query())
				}
				if r.URL.Query().Get("taskID") != "" || r.URL.Query().Get("payload") != "" {
					t.Error("non-query arguments leaked into query")
				}
				if r.Header.Get("Token") != "fake-user-token" {
					t.Error("per-request token missing")
				}
				if method == "POST" || method == "PUT" {
					if r.Header.Get("Content-Type") != "application/json" {
						t.Error("missing JSON content type")
					}
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					if body["name"] != "中文任务" || body["left"] != float64(0) {
						t.Errorf("incorrect payload: %v", body)
					}
				} else {
					body, _ := io.ReadAll(r.Body)
					if len(body) != 0 {
						t.Errorf("unexpected body: %s", body)
					}
				}
				if method == "DELETE" {
					w.WriteHeader(http.StatusNoContent)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":42,"name":"中文任务"}`))
			}))
			defer server.Close()
			service := New(proxy.New(middleware.WithBearerAuth(server.Client())), server.URL+"/zentao/api.php/v1")
			definition := &models.ToolDefinition{Method: method, Path: "/tasks/{taskID}", HasBody: method == "POST" || method == "PUT", Params: []models.ToolParam{{Name: "taskID", In: "path"}, {Name: "page", In: "query"}, {Name: "limit", In: "query"}, {Name: "search", In: "query"}}}
			ctx := middleware.WithAuthorizationSource(middleware.WithAuthorization(context.Background(), "fake-user-token"), true)
			status, body, err := service.Execute(ctx, definition, map[string]any{"taskID": 42, "page": 2, "limit": 20, "search": "待办 & review", "payload": map[string]any{"name": "中文任务", "left": 0}})
			if err != nil {
				t.Fatal(err)
			}
			if method == "DELETE" {
				if status != 204 || len(body) != 0 {
					t.Fatalf("unexpected delete response: %d %s", status, body)
				}
			} else if status != 200 || !json.Valid(body) {
				t.Fatalf("unexpected response: %d %s", status, body)
			}
		})
	}
}

func TestV1ExecutePreservesUpstreamErrors(t *testing.T) {
	for _, status := range []int{400, 401, 403, 404, 409, 422, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"error":"fixture failure"}`))
			}))
			defer server.Close()
			service := New(proxy.New(server.Client()), server.URL+"/api.php/v1")
			got, body, err := service.Execute(context.Background(), &models.ToolDefinition{Method: "GET", Path: "/tasks/42"}, nil)
			if err != nil || got != status || string(body) != `{"error":"fixture failure"}` {
				t.Fatalf("error lost: %d %s %v", got, body, err)
			}
		})
	}
}

func TestV1ConcurrentCredentialIsolation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expected := r.URL.Query().Get("owner")
		if r.Header.Get("Token") != expected {
			t.Errorf("credential crossed request boundary for %q", expected)
		}
		if r.Header.Get("Authorization") != "" {
			t.Error("unexpected bearer header for token request")
		}
		_, _ = w.Write([]byte(`{"tasks":[]}`))
	}))
	defer server.Close()
	service := New(proxy.New(middleware.WithBearerAuth(server.Client())), server.URL+"/api.php/v1")
	definition := &models.ToolDefinition{Method: "GET", Path: "/user", Params: []models.ToolParam{{Name: "owner", In: "query"}}}
	var wg sync.WaitGroup
	for _, token := range []string{"fake-alice", "fake-bob", ""} {
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func(token string) {
				defer wg.Done()
				ctx := middleware.WithAuthorizationSource(middleware.WithAuthorization(context.Background(), token), true)
				if _, _, err := service.Execute(ctx, definition, map[string]any{"owner": token}); err != nil {
					t.Error(err)
				}
			}(token)
		}
	}
	wg.Wait()
}
