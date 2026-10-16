package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/big"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/merzzzl/openapi-mcp-server/internal/models"
)

// MonthlyInput deliberately cannot select a different account or upstream URL.
type MonthlyInput struct {
	Year  int `json:"year"`
	Month int `json:"month"`
}
type Worklog struct {
	ID       int         `json:"id"`
	TaskID   int         `json:"task_id"`
	TaskName string      `json:"task_name"`
	Account  string      `json:"account"`
	Date     string      `json:"date"`
	Consumed json.Number `json:"consumed"`
	Work     string      `json:"work"`
}
type Coverage struct {
	Complete      bool     `json:"complete"`
	Scope         string   `json:"scope"`
	Tasks         int      `json:"tasks"`
	TasksScanned  int      `json:"tasks_scanned"`
	DaysRequested int      `json:"days_requested"`
	DaysScanned   int      `json:"days_scanned"`
	FromDate      string   `json:"from_date"`
	ToDate        string   `json:"to_date"`
	FailedDays    []string `json:"failed_days"`
	Failures      []string `json:"failures"`
	Truncated     bool     `json:"truncated"`
}
type MonthlyResult struct {
	Realname    string      `json:"realname"`
	Account     string      `json:"account"`
	UserID      int         `json:"user_id"`
	Year        int         `json:"year"`
	Month       int         `json:"month"`
	Timezone    string      `json:"timezone"`
	Complete    bool        `json:"complete"`
	Coverage    Coverage    `json:"coverage"`
	Entries     []Worklog   `json:"entries"`
	Count       int         `json:"count"`
	TotalHours  json.Number `json:"total_hours"`
	Limitations []string    `json:"limitations"`
}

func number(v any) (float64, bool) {
	f, e := strconv.ParseFloat(fmt.Sprint(v), 64)
	return f, e == nil && !math.IsNaN(f) && !math.IsInf(f, 0)
}
func integer(v any) (int, bool) {
	n, ok := number(v)
	return int(n), ok && n >= 0 && n == math.Trunc(n) && n < 1e12
}

type monthlyPolicyKey struct{}

func (s *Service) MyMonthlyWorklogsWithPolicy(ctx context.Context, in MonthlyInput, checker func(string, string) bool) (MonthlyResult, error) {
	return s.MyMonthlyWorklogs(context.WithValue(ctx, monthlyPolicyKey{}, checker), in)
}
func (s *Service) monthlyGet(ctx context.Context, path string, query map[string]any) (any, error) {
	if checker, ok := ctx.Value(monthlyPolicyKey{}).(func(string, string) bool); ok && !checker("GET", path) {
		return nil, fmt.Errorf("read not allowed: %s", path)
	}
	td := &models.ToolDefinition{Method: http.MethodGet, Path: path}
	for k := range query {
		td.Params = append(td.Params, models.ToolParam{Name: k, In: "query"})
	}
	status, b, e := s.Execute(ctx, td, query)
	if e != nil || status != 200 {
		return nil, fmt.Errorf("read failed: %s", path)
	}
	var v any
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.UseNumber()
	if decoder.Decode(&v) != nil {
		return nil, fmt.Errorf("invalid response: %s", path)
	}
	if m, ok := v.(map[string]any); ok {
		if m["error"] != nil || m["status"] == "fail" || m["status"] == "failed" {
			return nil, fmt.Errorf("API failure: %s", path)
		}
	}
	return v, nil
}
func (s *Service) monthlyIdentity(ctx context.Context) (map[string]any, error) {
	v, e := s.monthlyGet(ctx, "/user", nil)
	if e != nil {
		return nil, e
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid identity")
	}
	if p, ok := m["profile"].(map[string]any); ok {
		m = p
	}
	id, ok := integer(m["id"])
	account, _ := m["account"].(string)
	if !ok || id < 1 || account == "" {
		return nil, fmt.Errorf("invalid identity")
	}
	return m, nil
}

// monthlyList requires upstream total metadata. Missing totals, duplicate pages,
// inconsistent totals and early empty pages never count as proof of completeness.
func (s *Service) monthlyList(ctx context.Context, path string, keys ...string) ([]map[string]any, error) {
	out := []map[string]any{}
	seen := map[int]bool{}
	total := -1
	for page := 1; page <= 1000; page++ {
		v, e := s.monthlyGet(ctx, path, map[string]any{"page": page, "limit": 100, "status": "all", "browseType": "all"})
		if e != nil {
			return out, e
		}
		m, ok := v.(map[string]any)
		if !ok {
			return out, fmt.Errorf("missing pagination: %s", path)
		}
		var rows []any
		for _, key := range keys {
			switch a := m[key].(type) {
			case []any:
				rows = a
			case map[string]any:
				rows = []any{}
				for _, row := range a {
					rows = append(rows, row)
				}
			}
			if rows != nil {
				break
			}
		}
		if rows == nil {
			return out, fmt.Errorf("missing rows: %s", path)
		}
		for _, raw := range rows {
			row, ok := raw.(map[string]any)
			if !ok {
				return out, fmt.Errorf("invalid row: %s", path)
			}
			id, ok := integer(row["id"])
			if !ok || id < 1 || seen[id] {
				return out, fmt.Errorf("invalid or repeated row: %s", path)
			}
			seen[id] = true
			out = append(out, row)
		}
		n, valid := integer(m["total"])
		// Official REST v1 /tasks/id/estimate (api/1199) is an unpaginated
		// effort collection. If a server supplies total, validate its pagination.
		if _, present := m["total"]; !present && strings.HasSuffix(path, "/estimate") {
			return out, nil
		}
		if !valid {
			return out, fmt.Errorf("missing total: %s", path)
		}
		if total >= 0 && n != total {
			return out, fmt.Errorf("changed total: %s", path)
		}
		total = n
		if len(out) == total {
			return out, nil
		}
		if len(out) > total || len(rows) == 0 {
			return out, fmt.Errorf("incomplete pagination: %s", path)
		}
	}
	return out, fmt.Errorf("page cap: %s", path)
}

// MonthlyReportConfig is administrator-owned configuration, never model input.
type MonthlyReportConfig struct {
	URL          string            `yaml:"url"`
	AccountNames map[string]string `yaml:"account_names"`
}

func (s *Service) ConfigureMonthlyReport(config MonthlyReportConfig) error {
	if config.URL == "" {
		s.monthlyReport = config
		return nil
	}
	u, e := url.Parse(config.URL)
	if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("invalid monthly report URL")
	}
	s.monthlyReport = config
	return nil
}
func (s *Service) MonthlyReportEnabled() bool { return s.monthlyReport.URL != "" }
func reportDays(in MonthlyInput, now time.Time) ([]string, error) {
	loc, e := time.LoadLocation("Asia/Shanghai")
	if e != nil {
		return nil, e
	}
	now = now.In(loc)
	if in.Year != now.Year() || in.Month < 1 || in.Month > 12 || in.Month > int(now.Month()) {
		return nil, fmt.Errorf("daily report supports only current-year elapsed months")
	}
	start := time.Date(in.Year, time.Month(in.Month), 1, 0, 0, 0, 0, loc)
	end := start.AddDate(0, 1, -1)
	if in.Month == int(now.Month()) {
		end = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	}
	days := []string{}
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		days = append(days, d.Format("2006-01-02"))
	}
	return days, nil
}
func decimal(v any) (json.Number, error) {
	raw := fmt.Sprint(v)
	if len(raw) > 64 {
		return "", fmt.Errorf("invalid decimal")
	}
	n, valid := number(v)
	if !valid || n < 0 {
		return "", fmt.Errorf("invalid decimal")
	}
	if pos := strings.IndexAny(raw, "eE"); pos >= 0 {
		exp, e := strconv.Atoi(raw[pos+1:])
		if e != nil || exp < -308 || exp > 308 {
			return "", fmt.Errorf("invalid decimal exponent")
		}
	}
	if _, ok := new(big.Rat).SetString(raw); !ok {
		return "", fmt.Errorf("invalid decimal")
	}
	return json.Number(raw), nil
}
func exactTotal(values []json.Number) json.Number {
	total := new(big.Rat)
	for _, v := range values {
		n, _ := new(big.Rat).SetString(string(v))
		total.Add(total, n)
	}
	den := new(big.Int).Set(total.Denom())
	two, five := big.NewInt(2), big.NewInt(5)
	a, b := 0, 0
	for new(big.Int).Mod(den, two).Sign() == 0 {
		den.Div(den, two)
		a++
	}
	for new(big.Int).Mod(den, five).Sign() == 0 {
		den.Div(den, five)
		b++
	}
	if b > a {
		a = b
	}
	return json.Number(total.FloatString(a))
}

type reportRow struct {
	TaskID   int
	Name     string
	Consumed json.Number
}

func (s *Service) reportDay(ctx context.Context, name, date string) ([]reportRow, error) {
	u, _ := url.Parse(s.monthlyReport.URL)
	q := u.Query()
	q.Set("data", "yesterdayEff")
	q.Set("name", name)
	q.Set("ex", date[5:])
	u.RawQuery = q.Encode()
	// Independent client intentionally has no authentication transport, headers or cookies.
	client := &http.Client{Timeout: 12 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, e := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if e != nil {
		return nil, fmt.Errorf("invalid report request")
	}
	resp, e := client.Do(req)
	if e != nil {
		return nil, fmt.Errorf("report request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("report HTTP failure")
	}
	raw, e := io.ReadAll(io.LimitReader(resp.Body, (2<<20)+1))
	if e != nil || len(raw) > 2<<20 {
		return nil, fmt.Errorf("invalid report size")
	}
	var payload struct {
		Success bool             `json:"success"`
		Data    []map[string]any `json:"data"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&payload) != nil || !payload.Success || payload.Data == nil {
		return nil, fmt.Errorf("invalid report response")
	}
	rows := []reportRow{}
	for _, row := range payload.Data {
		id, ok := integer(row["taskid"])
		if !ok || id < 1 {
			return nil, fmt.Errorf("invalid report task")
		}
		if other, exists := row["id"]; exists {
			oid, valid := integer(other)
			if !valid || oid != id {
				return nil, fmt.Errorf("report task identity mismatch")
			}
		}
		hours, e := decimal(row["consumed"])
		name, valid := row["taskname"].(string)
		if e != nil || !valid || name == "" {
			return nil, fmt.Errorf("invalid report row")
		}
		rows = append(rows, reportRow{id, strings.SplitN(name, "\n", 2)[0], hours})
	}
	return rows, nil
}
func (s *Service) MyMonthlyWorklogs(ctx context.Context, in MonthlyInput) (MonthlyResult, error) {
	return s.myMonthlyWorklogsAt(ctx, in, time.Now())
}
func (s *Service) myMonthlyWorklogsAt(ctx context.Context, in MonthlyInput, now time.Time) (MonthlyResult, error) {
	r := MonthlyResult{Year: in.Year, Month: in.Month, Timezone: "Asia/Shanghai", Entries: []Worklog{}, Coverage: Coverage{Scope: "daily_report_verified_tasks", FailedDays: []string{}, Failures: []string{}}, Limitations: []string{"Daily report discovers candidate tasks for the authenticated profile name. Returned entries are original task worklogs independently verified using the authenticated account token. Completeness is limited to daily-report-discovered task worklogs, not a proof that the report includes every effort module or inaccessible/deleted task."}}
	if !s.MonthlyReportEnabled() {
		return r, fmt.Errorf("monthly report not configured")
	}
	days, e := reportDays(in, now)
	if e != nil {
		return r, e
	}
	r.Coverage.DaysRequested = len(days)
	r.Coverage.FromDate = days[0]
	r.Coverage.ToDate = days[len(days)-1]
	parent := ctx
	ctx, cancel := context.WithTimeout(ctx, 50*time.Second)
	defer cancel()
	profile, e := s.monthlyIdentity(ctx)
	if e != nil {
		return r, e
	}
	r.Account = profile["account"].(string)
	r.UserID, _ = integer(profile["id"])
	realname, ok := profile["realname"].(string)
	if !ok || strings.TrimSpace(realname) == "" {
		return r, fmt.Errorf("missing authenticated real name")
	}
	r.Realname = realname
	reportName := realname
	if mapped, exists := s.monthlyReport.AccountNames[r.Account]; exists {
		if strings.TrimSpace(mapped) == "" {
			return r, fmt.Errorf("invalid report name mapping")
		}
		reportName = mapped
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	jobs := make(chan string)
	reports := map[string][]reportRow{}
	tasks := map[int]string{}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for day := range jobs {
				rows, e := s.reportDay(ctx, reportName, day)
				mu.Lock()
				if e != nil {
					r.Coverage.FailedDays = append(r.Coverage.FailedDays, day)
					r.Coverage.Failures = append(r.Coverage.Failures, day+": "+e.Error())
				} else {
					r.Coverage.DaysScanned++
					reports[day] = rows
					for _, row := range rows {
						tasks[row.TaskID] = row.Name
					}
				}
				mu.Unlock()
			}
		}()
	}
	for _, day := range days {
		jobs <- day
	}
	close(jobs)
	wg.Wait()
	r.Coverage.Tasks = len(tasks)
	taskJobs := make(chan int)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for tid := range taskJobs {
				rows, e := s.monthlyList(ctx, fmt.Sprintf("/tasks/%d/estimate", tid), "effort", "efforts", "estimates", "estimate", "logs")
				entries := []Worklog{}
				for _, row := range rows {
					account, ok := row["account"].(string)
					if !ok || account == "" {
						e = fmt.Errorf("invalid log account")
						continue
					}
					if account != r.Account {
						continue
					}
					if deleted, exists := row["deleted"]; exists {
						switch fmt.Sprint(deleted) {
						case "1", "true":
							continue
						case "0", "false":
						default:
							e = fmt.Errorf("invalid deleted flag")
							continue
						}
					}
					day, ok := row["date"].(string)
					if _, err := time.Parse("2006-01-02", day); !ok || err != nil {
						e = fmt.Errorf("invalid log date")
						continue
					}
					if day < r.Coverage.FromDate || day > r.Coverage.ToDate {
						continue
					}
					id, valid := integer(row["id"])
					hours, err := decimal(row["consumed"])
					if !valid || id < 1 || err != nil {
						e = fmt.Errorf("invalid worklog")
						continue
					}
					if obj, exists := row["objectID"]; exists {
						oid, valid := integer(obj)
						if !valid || oid != tid {
							e = fmt.Errorf("wrong worklog task")
							continue
						}
					}
					if typ, exists := row["objectType"]; exists && typ != "task" {
						e = fmt.Errorf("wrong worklog type")
						continue
					}
					work, _ := row["work"].(string)
					entries = append(entries, Worklog{id, tid, tasks[tid], r.Account, day, hours, work})
				}
				mu.Lock()
				if e != nil {
					r.Coverage.Failures = append(r.Coverage.Failures, fmt.Sprintf("task %d: %s", tid, e))
				} else {
					r.Coverage.TasksScanned++
				}
				r.Entries = append(r.Entries, entries...)
				mu.Unlock()
			}
		}()
	}
	for tid := range tasks {
		taskJobs <- tid
	}
	close(taskJobs)
	wg.Wait()
	// Compare per-day/task row count AND exact amount, detecting same-name mixing
	// or report omissions within discovered tasks. Report rows never become logs.
	for day, reported := range reports {
		byTask := map[int][]json.Number{}
		actual := map[int][]json.Number{}
		for _, row := range reported {
			byTask[row.TaskID] = append(byTask[row.TaskID], row.Consumed)
		}
		for _, row := range r.Entries {
			if row.Date == day {
				actual[row.TaskID] = append(actual[row.TaskID], row.Consumed)
			}
		}
		keys := map[int]bool{}
		for id := range byTask {
			keys[id] = true
		}
		for id := range actual {
			keys[id] = true
		}
		for id := range keys {
			a, b := byTask[id], actual[id]
			if !sameAmounts(a, b) {
				r.Coverage.Failures = append(r.Coverage.Failures, fmt.Sprintf("report/log mismatch %s task %d", day, id))
			}
		}
	}
	if ctx.Err() != nil {
		r.Coverage.Truncated = true
		r.Coverage.Failures = append(r.Coverage.Failures, "query deadline exceeded")
	}
	verify, cancelVerify := context.WithTimeout(parent, 3*time.Second)
	defer cancelVerify()
	after, e := s.monthlyIdentity(verify)
	if e != nil {
		return MonthlyResult{}, fmt.Errorf("identity recheck failed")
	}
	uid, _ := integer(after["id"])
	if uid != r.UserID || after["account"] != r.Account || after["realname"] != r.Realname {
		return MonthlyResult{}, fmt.Errorf("identity changed")
	}
	sort.Slice(r.Entries, func(i, j int) bool {
		a, b := r.Entries[i], r.Entries[j]
		if a.Date != b.Date {
			return a.Date < b.Date
		}
		if a.TaskID != b.TaskID {
			return a.TaskID < b.TaskID
		}
		return a.ID < b.ID
	})
	amounts := []json.Number{}
	for _, row := range r.Entries {
		amounts = append(amounts, row.Consumed)
	}
	r.TotalHours = exactTotal(amounts)
	r.Count = len(r.Entries)
	sort.Strings(r.Coverage.FailedDays)
	sort.Strings(r.Coverage.Failures)
	r.Complete = len(r.Coverage.Failures) == 0 && !r.Coverage.Truncated && r.Coverage.DaysScanned == r.Coverage.DaysRequested && r.Coverage.TasksScanned == r.Coverage.Tasks
	r.Coverage.Complete = r.Complete
	return r, nil
}

func sameAmounts(a, b []json.Number) bool {
	if len(a) != len(b) {
		return false
	}
	left, right := []string{}, []string{}
	for _, v := range a {
		left = append(left, string(exactTotal([]json.Number{v})))
	}
	for _, v := range b {
		right = append(right, string(exactTotal([]json.Number{v})))
	}
	sort.Strings(left)
	sort.Strings(right)
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
