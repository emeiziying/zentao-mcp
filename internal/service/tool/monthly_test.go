package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/merzzzl/openapi-mcp-server/internal/clients"
	"github.com/merzzzl/openapi-mcp-server/internal/middleware"
	"github.com/merzzzl/openapi-mcp-server/internal/repository/proxy"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

var monthlyNow = time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)

func fixtureMonthly(t *testing.T, mutate func(string, string) string) *Service {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("mutation %s", r.Method)
		}
		var body string
		switch r.URL.Path {
		case "/user":
			body = `{"profile":{"id":21,"account":"owner","realname":"本人"}}`
		case "/dataViewData":
			if r.Header.Get("Token") != "" || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
				t.Error("report credential leak")
			}
			if r.URL.Query().Get("name") != "本人" || r.URL.Query().Get("data") != "yesterdayEff" {
				t.Error("wrong report identity")
			}
			if r.URL.Query().Get("ex") == "10-01" {
				body = `{"success":true,"data":[{"id":1,"taskid":1,"taskname":"reassigned\nwork","consumed":0.1}]}`
			} else {
				body = `{"success":true,"data":[{"id":2,"taskid":2,"taskname":"completed","consumed":0.2}]}`
			}
		case "/tasks/1/estimate":
			body = `{"effort":[{"id":101,"objectID":1,"objectType":"task","account":"owner","date":"2026-10-01","consumed":0.1,"deleted":"0","work":"work"},{"id":102,"account":"other","date":"2026-10-01","consumed":50},{"id":103,"account":"owner","date":"2026-09-30","consumed":50},{"id":104,"account":"owner","date":"2026-10-01","consumed":50,"deleted":"1"}]}`
		case "/tasks/2/estimate":
			body = `{"effort":{"105":{"id":105,"objectID":2,"objectType":"task","account":"owner","date":"2026-10-02","consumed":0.2}}}`
		default:
			t.Errorf("unexpected route (full-scan regression): %s", r.URL.Path)
			w.WriteHeader(404)
			return
		}
		if mutate != nil {
			body = mutate(r.URL.Path, body)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, body)
	}))
	t.Cleanup(server.Close)
	s := New(proxy.New(clients.NewHTTPClient(server.Client())), server.URL)
	if e := s.ConfigureMonthlyReport(MonthlyReportConfig{URL: server.URL + "/dataViewData"}); e != nil {
		t.Fatal(e)
	}
	return s
}
func TestMonthlyCompletedReassignedExactTotal(t *testing.T) {
	s := fixtureMonthly(t, nil)
	ctx := middleware.WithAuthorizationSource(middleware.WithAuthorization(context.Background(), "fake-user-token"), true)
	r, e := s.myMonthlyWorklogsAt(ctx, MonthlyInput{2026, 10}, monthlyNow)
	if e != nil {
		t.Fatal(e)
	}
	if !r.Complete || r.Count != 2 || r.TotalHours.String() != "0.3" || r.Coverage.TasksScanned != 2 || r.Coverage.DaysScanned != 2 || r.Entries[0].ID != 101 {
		t.Fatalf("unexpected %+v", r)
	}
	b, _ := json.Marshal(r)
	if strings.Contains(string(b), "other") {
		t.Fatal("other account leaked")
	}
}
func TestMonthlyPartialCases(t *testing.T) {
	for _, tc := range []struct{ name, path, body string }{
		{"missing report task id", "/dataViewData", `{"success":true,"data":[{"id":1,"taskname":"unknown","consumed":1}]}`},
		{"same name extra report row", "/dataViewData", `{"success":true,"data":[{"id":1,"taskid":1,"taskname":"same name","consumed":50}]}`},
		{"report failed", "/dataViewData", `{"success":false,"data":[]}`},
		{"missing account", "/tasks/2/estimate", `{"effort":[{"id":105,"date":"2026-10-02","consumed":0.2}]}`},
		{"bad date", "/tasks/2/estimate", `{"effort":[{"id":105,"account":"owner","date":"","consumed":0.2}]}`},
		{"bad amount", "/tasks/2/estimate", `{"effort":[{"id":105,"account":"owner","date":"2026-10-02","consumed":-2}]}`},
		{"wrong object", "/tasks/2/estimate", `{"effort":[{"id":105,"objectID":99,"account":"owner","date":"2026-10-02","consumed":0.2}]}`},
		{"invalid deleted", "/tasks/2/estimate", `{"effort":[{"id":105,"account":"owner","date":"2026-10-02","consumed":0.2,"deleted":"unknown"}]}`},
		{"duplicate log", "/tasks/2/estimate", `{"effort":[{"id":105,"account":"owner","date":"2026-10-02","consumed":0.2},{"id":105,"account":"owner","date":"2026-10-02","consumed":0.2}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := fixtureMonthly(t, func(p, b string) string {
				if p == tc.path {
					return tc.body
				}
				return b
			})
			r, e := s.myMonthlyWorklogsAt(context.Background(), MonthlyInput{2026, 10}, monthlyNow)
			if e != nil || r.Complete || r.Coverage.Complete || len(r.Coverage.Failures) == 0 {
				t.Fatalf("partial lost %+v %v", r, e)
			}
		})
	}
}
func TestMonthlyIdentityChange(t *testing.T) {
	for _, field := range []string{"owner", "本人"} {
		t.Run(field, func(t *testing.T) {
			calls := 0
			s := fixtureMonthly(t, func(p, b string) string {
				if p == "/user" {
					calls++
					if calls > 1 {
						return strings.ReplaceAll(b, field, "changed")
					}
				}
				return b
			})
			r, e := s.myMonthlyWorklogsAt(context.Background(), MonthlyInput{2026, 10}, monthlyNow)
			if e == nil || len(r.Entries) > 0 {
				t.Fatalf("identity data leak %+v %v", r, e)
			}
		})
	}
}
func TestMonthlyCalendarAndConfig(t *testing.T) {
	for _, in := range []MonthlyInput{{2025, 10}, {2026, 11}, {2026, 13}, {2026, 0}} {
		if _, e := reportDays(in, monthlyNow); e == nil {
			t.Fatal("unsupported period accepted")
		}
	}
	days, e := reportDays(MonthlyInput{2026, 9}, monthlyNow)
	if e != nil || len(days) != 30 || days[29] != "2026-09-30" {
		t.Fatal("past month incomplete")
	}
	days, e = reportDays(MonthlyInput{2026, 10}, time.Date(2026, 10, 1, 17, 0, 0, 0, time.UTC))
	if e != nil || len(days) != 2 {
		t.Fatal("Shanghai boundary wrong")
	}
	s := New(nil, "")
	for _, u := range []string{"file:///tmp/x", "http://user:secret@host/x", "http://host/x?name=other", "http://host/x#fragment"} {
		if s.ConfigureMonthlyReport(MonthlyReportConfig{URL: u}) == nil {
			t.Fatal("unsafe URL accepted")
		}
	}
	if s.MonthlyReportEnabled() {
		t.Fatal("default unexpectedly enabled")
	}
}
func TestMonthlyPolicyAndEmptyReport(t *testing.T) {
	s := fixtureMonthly(t, func(p, b string) string {
		if p == "/dataViewData" {
			return `{"success":true,"data":[]}`
		}
		return b
	})
	r, e := s.myMonthlyWorklogsAt(context.Background(), MonthlyInput{2026, 10}, monthlyNow)
	if e != nil || !r.Complete || r.Count != 0 || r.TotalHours != "0" {
		t.Fatalf("empty report %+v %v", r, e)
	}
	s = fixtureMonthly(t, nil)
	ctx := context.WithValue(context.Background(), monthlyPolicyKey{}, func(_, path string) bool { return path != "/tasks/2/estimate" })
	r, e = s.myMonthlyWorklogsAt(ctx, MonthlyInput{2026, 10}, monthlyNow)
	if e != nil || r.Complete || r.Coverage.TasksScanned != 1 {
		t.Fatal("policy bypass")
	}
}

func TestMonthlyAmountMultiset(t *testing.T) {
	if sameAmounts([]json.Number{"1", "3"}, []json.Number{"2", "2"}) {
		t.Fatal("same sum mismatch accepted")
	}
	if !sameAmounts([]json.Number{"0.10", "2"}, []json.Number{"2.0", "0.1"}) {
		t.Fatal("equivalent decimals rejected")
	}
}
