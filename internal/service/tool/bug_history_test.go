package tool

import (
	"context"
	"fmt"
	"github.com/merzzzl/openapi-mcp-server/internal/clients"
	"github.com/merzzzl/openapi-mcp-server/internal/repository/proxy"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func bugHistoryFixture(t *testing.T, mode string) *Service {
	t.Helper()
	users := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Error("write attempted")
		}
		switch r.URL.Path {
		case "/user":
			users++
			account := "owner"
			if mode == "identity" && users > 1 {
				account = "other"
			}
			products := "1"
			if mode == "revoked" && users > 1 {
				products = ""
			}
			fmt.Fprintf(w, `{"profile":{"id":21,"account":%q,"view":{"products":%q}}}`, account, products)
		case "/products/1/bugs":
			if mode == "missingtotal" {
				fmt.Fprint(w, `{"bugs":[{"id":1}]}`)
				return
			}
			if r.URL.Query().Get("status") != "all" || r.URL.Query().Get("browseType") != "all" {
				t.Error("missing history filters")
			}
			if mode == "page" {
				if r.URL.Query().Get("page") == "2" {
					w.WriteHeader(500)
					return
				}
				fmt.Fprint(w, `{"total":5,"bugs":[{"id":1}]}`)
				return
			}
			if mode == "duplicate" {
				fmt.Fprint(w, `{"total":5,"bugs":[{"id":1}]}`)
				return
			}
			fmt.Fprint(w, `{"total":4,"bugs":[{"id":1},{"id":2},{"id":3},{"id":4}]}`)
		case "/bugs/1":
			if mode == "nullresolver" {
				fmt.Fprint(w, `{"id":1,"product":1,"status":"resolved","openedBy":"other","resolvedBy":null,"closedBy":null,"assignedTo":null}`)
				return
			}
			if mode == "missingrelation" {
				fmt.Fprint(w, `{"id":1,"product":1,"title":"resolved","status":"resolved","resolvedBy":"owner","resolvedDate":"2026-01-01"}`)
				return
			}
			if mode == "malformedrelation" {
				fmt.Fprint(w, `{"id":1,"product":1,"status":"resolved","openedBy":[],"assignedTo":"","closedBy":"","resolvedBy":"owner","resolvedDate":"2026-01-01"}`)
				return
			}
			date := "2026-01-01"
			if mode == "missingdate" {
				date = ""
			}
			fmt.Fprintf(w, `{"id":1,"product":1,"title":"resolved","status":"resolved","openedBy":"other","closedBy":"","assignedTo":"","resolvedBy":"owner","resolvedDate":%q}`, date)
		case "/bugs/2":
			if mode == "nullcloser" {
				fmt.Fprint(w, `{"id":2,"product":1,"status":"closed","openedBy":"other","resolvedBy":null,"closedBy":null,"assignedTo":null}`)
				return
			}
			if mode == "nullassignee" || mode == "missingassignee" || mode == "malformedassignee" {
				assignee := `,"assignedTo":null`
				if mode == "missingassignee" {
					assignee = ""
				}
				if mode == "malformedassignee" {
					assignee = `,"assignedTo":{}`
				}
				fmt.Fprint(w, `{"id":2,"product":1,"status":"closed","openedBy":"other","resolvedBy":"","closedBy":{"account":"owner"},"closedDate":"2026-02-02"`+assignee+`}`)
				return
			}
			fmt.Fprint(w, `{"id":2,"product":1,"status":"closed","openedBy":"other","resolvedBy":"","assignedTo":"","closedBy":{"account":"owner"},"closedDate":"2026-02-02"}`)
		case "/bugs/3":
			if mode == "nullinactiveactors" || mode == "nullopener" || mode == "missinginactiveactor" || mode == "malformedinactiveactor" {
				opener, closer := `"other"`, `,"closedBy":null`
				if mode == "nullopener" {
					opener = `null`
				}
				if mode == "missinginactiveactor" {
					closer = ``
				}
				if mode == "malformedinactiveactor" {
					closer = `,"closedBy":{}`
				}
				fmt.Fprint(w, `{"id":3,"product":1,"status":"active","openedBy":`+opener+`,"resolvedBy":null,"assignedTo":null`+closer+`}`)
				return
			}
			fmt.Fprint(w, `{"id":3,"product":1,"status":"active","resolvedBy":"","closedBy":"","assignedTo":"owner2","openedBy":"other","openedDate":"2026-01-01"}`)
		case "/bugs/4":
			fmt.Fprint(w, `{"id":4,"product":1,"status":"closed","openedBy":"owner","openedDate":"2026-01-01","deleted":"1"}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(server.Close)
	return New(proxy.New(clients.NewHTTPClient(server.Client())), server.URL)
}
func TestBugHistoryResolvedClosedExactIdentity(t *testing.T) {
	s := bugHistoryFixture(t, "")
	r, e := s.MyBugHistoryWithPolicy(context.Background(), BugHistoryInput{ScanAllVisible: true, Year: 2026}, func(m, p string) bool { return m == "GET" })
	if e != nil || !r.Complete || len(r.Entries) != 2 {
		t.Fatalf("%+v %v", r, e)
	}
	if r.Entries[0].Status != "resolved" || r.Entries[1].Status != "closed" {
		t.Fatal(r)
	}
}
func TestBugHistoryPartial(t *testing.T) {
	for _, mode := range []string{"page", "duplicate", "missingdate", "missingrelation", "malformedrelation", "missingtotal"} {
		t.Run(mode, func(t *testing.T) {
			r, e := bugHistoryFixture(t, mode).MyBugHistoryWithPolicy(context.Background(), BugHistoryInput{ScanAllVisible: true, Year: 2026}, func(m, p string) bool { return true })
			if e != nil || r.Complete || len(r.Failures) == 0 {
				t.Fatalf("%+v %v", r, e)
			}
		})
	}
}
func TestBugHistoryIdentityChangeDiscards(t *testing.T) {
	r, e := bugHistoryFixture(t, "identity").MyBugHistoryWithPolicy(context.Background(), BugHistoryInput{ScanAllVisible: true}, func(m, p string) bool { return true })
	if e == nil || len(r.Entries) != 0 {
		t.Fatal(r, e)
	}
}
func TestBugHistoryScopeAndPolicy(t *testing.T) {
	for _, id := range []int{2, -1} {
		_, e := bugHistoryFixture(t, "").MyBugHistoryWithPolicy(context.Background(), BugHistoryInput{ProductID: id}, func(m, p string) bool { return true })
		if e == nil {
			t.Fatal("scope accepted")
		}
	}
	r, e := bugHistoryFixture(t, "").MyBugHistoryWithPolicy(context.Background(), BugHistoryInput{ScanAllVisible: true}, func(m, p string) bool { return !strings.HasPrefix(p, "/bugs/") })
	if e != nil || r.Complete || len(r.Entries) != 0 {
		t.Fatal(r, e)
	}
}
func TestBugHistoryEventYear(t *testing.T) {
	r, e := bugHistoryFixture(t, "").MyBugHistoryWithPolicy(context.Background(), BugHistoryInput{ScanAllVisible: true, Year: 2025}, func(m, p string) bool { return true })
	if e != nil || !r.Complete || len(r.Entries) != 0 {
		t.Fatal(r, e)
	}
}

func TestBugHistoryRequiresExplicitScope(t *testing.T) {
	_, e := bugHistoryFixture(t, "").MyBugHistoryWithPolicy(context.Background(), BugHistoryInput{}, func(m, p string) bool { t.Error("read before scope selected"); return true })
	if e == nil {
		t.Fatal("missing scope accepted")
	}
}

func TestBugHistoryVisibilityRevocationDiscards(t *testing.T) {
	r, e := bugHistoryFixture(t, "revoked").MyBugHistoryWithPolicy(context.Background(), BugHistoryInput{ScanAllVisible: true}, func(m, p string) bool { return true })
	if e == nil || len(r.Entries) != 0 {
		t.Fatal(r, e)
	}
}

func TestBugHistoryNullableAssignee(t *testing.T) {
	for _, mode := range []string{"nullassignee", "missingassignee", "malformedassignee"} {
		t.Run(mode, func(t *testing.T) {
			r, e := bugHistoryFixture(t, mode).MyBugHistoryWithPolicy(context.Background(), BugHistoryInput{ScanAllVisible: true, Year: 2026}, func(m, p string) bool { return m == "GET" })
			if e != nil || r.Complete != (mode == "nullassignee") || len(r.Entries) != 2 {
				t.Fatalf("%+v %v", r, e)
			}
		})
	}
}

func TestBugHistoryNullableActorsByStatus(t *testing.T) {
	for _, mode := range []string{"nullinactiveactors", "nullopener", "missinginactiveactor", "malformedinactiveactor", "nullresolver", "nullcloser"} {
		t.Run(mode, func(t *testing.T) {
			r, e := bugHistoryFixture(t, mode).MyBugHistoryWithPolicy(context.Background(), BugHistoryInput{ScanAllVisible: true, Year: 2026}, func(m, p string) bool { return m == "GET" })
			if e != nil || r.Complete != (mode == "nullinactiveactors") {
				t.Fatalf("%+v %v", r, e)
			}
		})
	}
}
