package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/merzzzl/openapi-mcp-server/internal/clients"
	"github.com/merzzzl/openapi-mcp-server/internal/repository/proxy"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func taskHistoryFixture(t *testing.T, mutate func(string, string) string) *Service {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Error("write")
		}
		body := ""
		switch r.URL.Path {
		case "/user":
			body = `{"profile":{"id":21,"account":"owner","realname":"本人","view":{"sprints":"2"}}}`
		case "/executions/2/tasks":
			body = `{"total":2,"tasks":[{"id":1,"openedDate":"2026-01-01"},{"id":2,"openedDate":"2025-01-01"}]}`
		case "/tasks/1":
			body = `{"id":1,"execution":2,"name":"done","status":"done","assignedTo":"other","finishedBy":{"account":"owner"},"finishedDate":"2026-02-02","deadline":"2026-02-01","closedDate":"","actions":[]}`
		case "/tasks/2":
			body = `{"id":2,"execution":2,"name":"reassigned","status":"doing","assignedTo":"other","deadline":"2025-01-01","finishedDate":"","closedDate":"","actions":[{"action":"assigned","date":"2026-01-01","history":[{"field":"assignedTo","new":"owner"}]}]}`
		default:
			t.Errorf("route %s", r.URL.Path)
			w.WriteHeader(404)
			return
		}
		if mutate != nil {
			body = mutate(r.URL.Path, body)
		}
		fmt.Fprint(w, body)
	}))
	t.Cleanup(server.Close)
	return New(proxy.New(clients.NewHTTPClient(server.Client())), server.URL)
}
func TestTaskHistoryAuditAndQuick(t *testing.T) {
	for _, mode := range []string{"all", "year"} {
		r, e := taskHistoryFixture(t, nil).myTaskHistoryAt(context.Background(), TaskHistoryInput{Year: 2026, Mode: mode, ScanAllVisible: true}, time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC))
		if e != nil {
			t.Fatal(e)
		}
		if mode == "all" {
			if !r.Complete || r.Count != 2 || r.CurrentOverdueCount != 1 || r.FinishedLateCount != 1 {
				t.Fatalf("%+v", r)
			}
		} else if r.Complete || r.Count != 1 || !r.ScanComplete {
			t.Fatalf("%+v", r)
		}
	}
}
func TestTaskHistoryExactIdentity(t *testing.T) {
	for _, h := range []string{`{"field":"assignedTo","new":"owner-other"}`, `{"field":"name","new":"owner"}`} {
		var m map[string]any
		json.Unmarshal([]byte(`{"actions":[{"action":"assigned","date":"2026-01-01","extra":"owner","desc":"owner","history":[`+h+`]}]}`), &m)
		r, _ := taskHistoryReasons(m, "owner", "本人", 2026)
		if len(r) > 0 {
			t.Fatal("unsafe match")
		}
	}
}
func TestTaskHistoryPartial(t *testing.T) {
	for _, change := range []string{"actions", "total", "name-only", "date"} {
		s := taskHistoryFixture(t, func(path, body string) string {
			if change == "total" && strings.Contains(path, "executions") {
				return strings.Replace(body, `"total":2,`, "", 1)
			}
			if path == "/tasks/2" {
				switch change {
				case "actions":
					return `{"id":2,"assignedTo":"other"}`
				case "name-only":
					return strings.Replace(body, `"new":"owner"`, `"new":"本人"`, 1)
				case "date":
					return strings.Replace(body, `"date":"2026-01-01",`, "", 1)
				}
			}
			return body
		})
		r, e := s.myTaskHistoryAt(context.Background(), TaskHistoryInput{Year: 2026, Mode: "all", ScanAllVisible: true}, time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC))
		if e != nil || r.Complete {
			t.Fatalf("%s %+v %v", change, r, e)
		}
	}
}
func TestTaskHistoryRevocation(t *testing.T) {
	calls := 0
	s := taskHistoryFixture(t, func(path, body string) string {
		if path == "/user" {
			calls++
			if calls > 1 {
				return strings.Replace(body, `"sprints":"2"`, `"sprints":""`, 1)
			}
		}
		return body
	})
	r, e := s.myTaskHistoryAt(context.Background(), TaskHistoryInput{Year: 2026, Mode: "all", ScanAllVisible: true}, time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC))
	if e == nil || len(r.Items) > 0 {
		t.Fatal("revoked results retained")
	}
}
func TestTaskHistoryScopeDenied(t *testing.T) {
	_, e := taskHistoryFixture(t, nil).myTaskHistoryAt(context.Background(), TaskHistoryInput{Year: 2026, ExecutionIDs: []int{99}}, time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC))
	if e == nil {
		t.Fatal("invisible execution")
	}
}
func TestTaskHistoryOverdueRules(t *testing.T) {
	for _, status := range []string{"done", "closed", "cancel", "cancelled"} {
		i := taskHistoryItem(map[string]any{"status": status, "deadline": "2026-01-01", "closedDate": "2026-01-02"}, nil, "2026-01-02")
		if i.CurrentOverdue || !i.FinishedLate || i.FinishedLateBasis != "closedDate_proxy" {
			t.Fatalf("%+v", i)
		}
	}
	if taskHistoryItem(map[string]any{"status": "doing", "deadline": "2026-01-02"}, nil, "2026-01-02").CurrentOverdue {
		t.Fatal("today overdue")
	}
}
func TestTaskHistoryPolicy(t *testing.T) {
	s := taskHistoryFixture(t, nil)
	r, e := s.MyTaskHistoryWithPolicy(context.Background(), TaskHistoryInput{Year: 2026, Mode: "all", ScanAllVisible: true}, func(method, path string) bool { return method == "GET" && !strings.HasPrefix(path, "/tasks/") })
	if e != nil || r.Complete || r.Count != 0 {
		t.Fatalf("%+v %v", r, e)
	}
}

func TestTaskHistoryExplicitScope(t *testing.T) {
	s := taskHistoryFixture(t, func(path, body string) string { t.Error("unspecified scope performed read"); return body })
	_, e := s.myTaskHistoryAt(context.Background(), TaskHistoryInput{Year: 2026}, time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC))
	if e == nil {
		t.Fatal("scope required")
	}
}
func TestTaskHistoryMissingStructuredTarget(t *testing.T) {
	for _, body := range []string{`{"actions":[{"action":"assigned","date":"2026-01-01","history":[]}]}`, `{"actions":[{"action":"finished","date":"2026-01-01","actor":"本人"}]}`, `{"actions":[{"action":"closed","date":"2026-01-01"}]}`} {
		var m map[string]any
		json.Unmarshal([]byte(body), &m)
		r, partial := taskHistoryReasons(m, "owner", "本人", 2026)
		if !partial || len(r) != 0 {
			t.Fatalf("%s %v %v", body, r, partial)
		}
	}
}

func TestTaskHistoryNullableDates(t *testing.T) {
	for _, tc := range []struct {
		name, path, old, replacement string
		complete                     bool
	}{
		{"open-null-dates", "/tasks/2", `"finishedDate":"","closedDate":""`, `"finishedDate":null,"closedDate":null`, true},
		{"open-null-assignee", "/tasks/2", `"assignedTo":"other"`, `"assignedTo":null`, true},
		{"done-null-finish", "/tasks/1", `"finishedDate":"2026-02-02"`, `"finishedDate":null`, false},
		{"closed-null-close", "/tasks/1", `"status":"done"`, `"status":"closed"`, false},
		{"missing-finish", "/tasks/2", `"finishedDate":"",`, ``, false},
		{"invalid-finish", "/tasks/2", `"finishedDate":""`, `"finishedDate":42`, false},
		{"null-dates-name-only", "/tasks/2", `"new":"owner"`, `"new":"本人"`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := taskHistoryFixture(t, func(path, body string) string {
				if path == tc.path {
					body = strings.Replace(body, tc.old, tc.replacement, 1)
					if tc.name == "closed-null-close" {
						body = strings.Replace(body, `"closedDate":""`, `"closedDate":null`, 1)
					}
					if tc.name == "null-dates-name-only" {
						body = strings.Replace(body, `"finishedDate":"","closedDate":""`, `"finishedDate":null,"closedDate":null`, 1)
					}
					return body
				}
				return body
			})
			r, e := s.myTaskHistoryAt(context.Background(), TaskHistoryInput{Year: 2026, Mode: "all", ScanAllVisible: true}, time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC))
			if e != nil || r.Complete != tc.complete {
				t.Fatalf("%+v %v", r, e)
			}
		})
	}
}
