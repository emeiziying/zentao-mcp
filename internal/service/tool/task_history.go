package tool

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
)

type TaskHistoryInput struct {
	Year           int    `json:"year"`
	Mode           string `json:"mode,omitempty"`
	ExecutionIDs   []int  `json:"execution_ids,omitempty"`
	ScanAllVisible bool   `json:"scan_all_visible,omitempty"`
}
type TaskHistoryItem struct {
	ID                int      `json:"id"`
	Name              string   `json:"name"`
	Status            string   `json:"status"`
	Deadline          string   `json:"deadline,omitempty"`
	FinishedDate      string   `json:"finished_date,omitempty"`
	ClosedDate        string   `json:"closed_date,omitempty"`
	Reasons           []string `json:"reasons"`
	CurrentOverdue    bool     `json:"current_overdue"`
	FinishedLate      bool     `json:"finished_late"`
	FinishedLateBasis string   `json:"finished_late_basis,omitempty"`
}
type TaskHistoryResult struct {
	Account             string            `json:"account"`
	UserID              int               `json:"user_id"`
	Year                int               `json:"year"`
	Today               string            `json:"today"`
	Timezone            string            `json:"timezone"`
	Mode                string            `json:"mode"`
	Scope               string            `json:"scope"`
	Complete            bool              `json:"complete"`
	ScanComplete        bool              `json:"scan_complete"`
	Truncated           bool              `json:"truncated"`
	ExecutionCount      int               `json:"execution_count"`
	RowsScanned         int               `json:"rows_scanned"`
	DetailLoaded        int               `json:"detail_loaded"`
	Count               int               `json:"count"`
	OpenCount           int               `json:"open_count"`
	CurrentOverdueCount int               `json:"current_overdue_count"`
	FinishedLateCount   int               `json:"finished_late_count"`
	NoDeadlineCount     int               `json:"no_deadline_count"`
	StatusCounts        map[string]int    `json:"status_counts"`
	RelationCounts      map[string]int    `json:"relation_counts"`
	Items               []TaskHistoryItem `json:"items"`
	Failures            []string          `json:"failures"`
	Limitations         []string          `json:"limitations"`
}

func taskHistoryVisibleSprints(profile map[string]any) ([]int, error) {
	view, ok := profile["view"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("missing execution visibility")
	}
	raw, ok := view["sprints"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid execution visibility")
	}
	ids := []int{}
	seen := map[int]bool{}
	for _, v := range strings.Split(raw, ",") {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		id, e := strconv.Atoi(v)
		if e != nil || id < 1 {
			return nil, fmt.Errorf("invalid execution visibility")
		}
		if !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	sort.Ints(ids)
	return ids, nil
}
func taskHistoryDate(v any) string {
	s, _ := v.(string)
	if len(s) < 10 {
		return ""
	}
	s = s[:10]
	if _, e := time.Parse("2006-01-02", s); e != nil {
		return ""
	}
	return s
}
func taskHistoryYear(v any, year int) bool {
	d := taskHistoryDate(v)
	return d != "" && strings.HasPrefix(d, fmt.Sprintf("%04d-", year))
}
func taskHistoryMarker(m map[string]any, year int) bool {
	for _, k := range []string{"openedDate", "assignedDate", "estStarted", "realStarted", "deadline", "finishedDate", "closedDate", "lastEditedDate"} {
		if taskHistoryYear(m[k], year) {
			return true
		}
	}
	return false
}
func taskHistoryAccount(v any) string {
	if m, ok := v.(map[string]any); ok {
		s, _ := m["account"].(string)
		return s
	}
	s, _ := v.(string)
	return s
}
func taskHistoryRows(v any) ([]map[string]any, bool) {
	out := []map[string]any{}
	if a, ok := v.([]any); ok {
		for _, x := range a {
			m, ok := x.(map[string]any)
			if !ok {
				return out, false
			}
			out = append(out, m)
		}
		return out, true
	}
	if a, ok := v.(map[string]any); ok {
		for _, x := range a {
			m, ok := x.(map[string]any)
			if !ok {
				return out, false
			}
			out = append(out, m)
		}
		return out, true
	}
	return out, false
}
func taskHistoryReasons(m map[string]any, account, realname string, year int) ([]string, bool) {
	reasons := map[string]bool{}
	ambiguous := false
	match := func(v any) bool {
		a := taskHistoryAccount(v)
		if a == account {
			return true
		}
		if a != "" && a == realname {
			ambiguous = true
		}
		if obj, ok := v.(map[string]any); ok && a == "" && obj["realname"] == realname {
			ambiguous = true
		}
		return false
	}
	if value, exists := m["assignedTo"]; !exists || (value != nil && value != "" && taskHistoryAccount(value) == "") {
		ambiguous = true
	}
	if match(m["assignedTo"]) && taskHistoryMarker(m, year) {
		reasons["current_assignee_in_year"] = true
	}
	for _, pair := range [][3]string{{"finishedBy", "finishedDate", "finished_in_year"}, {"closedBy", "closedDate", "closed_in_year"}} {
		if taskHistoryYear(m[pair[1]], year) && taskHistoryAccount(m[pair[0]]) == "" {
			ambiguous = true
		}
		if taskHistoryYear(m[pair[1]], year) && match(m[pair[0]]) {
			reasons[pair[2]] = true
		}
	}
	actions, valid := taskHistoryRows(m["actions"])
	if !valid {
		ambiguous = true
	}
	for _, a := range actions {
		if taskHistoryDate(a["date"]) == "" {
			ambiguous = true
		}
		if !taskHistoryYear(a["date"], year) {
			continue
		}
		name, _ := a["action"].(string)
		name = strings.ToLower(name)
		if (name == "finished" || name == "closed") && taskHistoryAccount(a["actor"]) == "" {
			ambiguous = true
		}
		if (name == "finished" || name == "closed") && match(a["actor"]) {
			reasons[name+"_in_year"] = true
		}
		if name == "assigned" {
			rows, ok := taskHistoryRows(a["history"])
			if !ok {
				ambiguous = true
			}
			targetFound := false
			for _, h := range rows {
				if h["field"] != "assignedTo" {
					continue
				}
				targetFound = true
				value, ok := h["new"]
				if !ok {
					value = h["newValue"]
				}
				if taskHistoryAccount(value) == "" {
					ambiguous = true
				}
				if match(value) {
					reasons["assigned_in_year"] = true
				}
			}
			if !targetFound {
				ambiguous = true
			}
		}
	}
	out := []string{}
	for reason := range reasons {
		out = append(out, reason)
	}
	sort.Strings(out)
	return out, ambiguous
}
func taskHistoryItem(m map[string]any, reasons []string, today string) TaskHistoryItem {
	id, _ := integer(m["id"])
	name, _ := m["name"].(string)
	status, _ := m["status"].(string)
	item := TaskHistoryItem{ID: id, Name: name, Status: status, Deadline: taskHistoryDate(m["deadline"]), FinishedDate: taskHistoryDate(m["finishedDate"]), ClosedDate: taskHistoryDate(m["closedDate"]), Reasons: reasons}
	open := taskHistoryOpen(status)
	item.CurrentOverdue = open && item.Deadline != "" && item.Deadline < today
	end := item.FinishedDate
	basis := "finishedDate"
	if end == "" {
		end = item.ClosedDate
		basis = "closedDate_proxy"
	}
	item.FinishedLate = item.Deadline != "" && end != "" && end > item.Deadline
	if item.FinishedLate {
		item.FinishedLateBasis = basis
	}
	return item
}
func taskHistoryOpen(status string) bool {
	switch strings.ToLower(status) {
	case "wait", "doing", "pause":
		return true
	}
	return false
}
func (s *Service) MyTaskHistoryWithPolicy(ctx context.Context, in TaskHistoryInput, checker func(string, string) bool) (TaskHistoryResult, error) {
	if checker == nil {
		return TaskHistoryResult{}, fmt.Errorf("missing read policy")
	}
	return s.myTaskHistoryAt(context.WithValue(ctx, monthlyPolicyKey{}, checker), in, time.Now())
}
func (s *Service) myTaskHistoryAt(parent context.Context, in TaskHistoryInput, now time.Time) (TaskHistoryResult, error) {
	loc, e := time.LoadLocation("Asia/Shanghai")
	if e != nil {
		return TaskHistoryResult{}, e
	}
	now = now.In(loc)
	if in.Year == 0 {
		in.Year = now.Year()
	}
	if in.Mode == "" {
		in.Mode = "year"
	}
	r := TaskHistoryResult{Year: in.Year, Today: now.Format("2006-01-02"), Timezone: "Asia/Shanghai", Mode: in.Mode, Scope: "authenticated_user_relations_in_selected_visible_executions", Items: []TaskHistoryItem{}, Failures: []string{}, StatusCounts: map[string]int{}, RelationCounts: map[string]int{}, Limitations: []string{"Completeness is limited to currently visible executions and retained structured account history; inaccessible or deleted history is not covered. closedDate_proxy means closure date, not proven completion time."}}
	if in.Year < 1970 || in.Year > now.Year() || (in.Mode != "year" && in.Mode != "all") {
		return r, fmt.Errorf("invalid year or mode")
	}
	if len(in.ExecutionIDs) == 0 && !in.ScanAllVisible {
		return r, fmt.Errorf("execution_ids or explicit scan_all_visible required")
	}
	if len(in.ExecutionIDs) > 0 && in.ScanAllVisible {
		return r, fmt.Errorf("choose execution_ids or scan_all_visible")
	}
	ctx, cancel := context.WithTimeout(parent, 50*time.Second)
	defer cancel()
	profile, e := s.monthlyIdentity(ctx)
	if e != nil {
		return r, e
	}
	visible, e := taskHistoryVisibleSprints(profile)
	if e != nil {
		return r, e
	}
	r.Account = profile["account"].(string)
	r.UserID, _ = integer(profile["id"])
	realname, _ := profile["realname"].(string)
	selected := visible
	if len(in.ExecutionIDs) > 0 {
		selected = []int{}
		seen := map[int]bool{}
		for _, id := range in.ExecutionIDs {
			pos := sort.SearchInts(visible, id)
			if pos == len(visible) || visible[pos] != id {
				return TaskHistoryResult{}, fmt.Errorf("execution outside authenticated visibility")
			}
			if !seen[id] {
				selected = append(selected, id)
				seen[id] = true
			}
		}
	}
	r.ExecutionCount = len(selected)
	candidates := map[int]map[string]any{}
	candidateExecutions := map[int]int{}
	for _, eid := range selected {
		if ctx.Err() != nil {
			break
		}
		rows, err := s.monthlyList(ctx, fmt.Sprintf("/executions/%d/tasks", eid), "tasks")
		if err != nil {
			r.Failures = append(r.Failures, fmt.Sprintf("execution %d incomplete", eid))
		}
		r.RowsScanned += len(rows)
		for _, m := range rows {
			if in.Mode == "year" && !taskHistoryMarker(m, in.Year) {
				continue
			}
			id, _ := integer(m["id"])
			if previous, exists := candidateExecutions[id]; exists && previous != eid {
				r.Failures = append(r.Failures, fmt.Sprintf("task %d repeated across executions", id))
			}
			candidates[id] = m
			candidateExecutions[id] = eid
		}
	}
	ids := []int{}
	for id := range candidates {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		if ctx.Err() != nil {
			break
		}
		v, err := s.monthlyGet(ctx, fmt.Sprintf("/tasks/%d", id), nil)
		m, ok := v.(map[string]any)
		got, _ := integer(m["id"])
		if err != nil || !ok || got != id {
			r.Failures = append(r.Failures, fmt.Sprintf("task %d detail incomplete", id))
			continue
		}
		r.DetailLoaded++
		eid, validExecution := integer(m["execution"])
		if !validExecution || eid != candidateExecutions[id] {
			r.Failures = append(r.Failures, fmt.Sprintf("task %d execution mismatch", id))
			continue
		}
		if deleted, exists := m["deleted"]; exists {
			if fmt.Sprint(deleted) == "1" || fmt.Sprint(deleted) == "true" {
				r.Failures = append(r.Failures, fmt.Sprintf("task %d deleted", id))
				continue
			}
		}
		status, _ := m["status"].(string)
		switch strings.ToLower(status) {
		case "wait", "doing", "pause", "done", "closed", "cancel", "cancelled":
		default:
			r.Failures = append(r.Failures, fmt.Sprintf("task %d unknown status", id))
		}
		for _, field := range []string{"deadline", "finishedDate", "closedDate"} {
			if _, exists := m[field]; !exists {
				r.Failures = append(r.Failures, fmt.Sprintf("task %d missing %s", id, field))
			}
			if raw, exists := m[field]; exists && raw != nil && raw != "" && raw != "0000-00-00" && raw != "0000-00-00 00:00:00" && taskHistoryDate(raw) == "" {
				r.Failures = append(r.Failures, fmt.Sprintf("task %d invalid %s", id, field))
			}
		}
		if (strings.ToLower(status) == "done" && taskHistoryDate(m["finishedDate"]) == "") || (strings.ToLower(status) == "closed" && taskHistoryDate(m["closedDate"]) == "") {
			r.Failures = append(r.Failures, fmt.Sprintf("task %d missing status-required date", id))
		}
		reasons, ambiguous := taskHistoryReasons(m, r.Account, realname, in.Year)
		if ambiguous {
			r.Failures = append(r.Failures, fmt.Sprintf("task %d history missing or identity ambiguous", id))
		}
		if len(reasons) > 0 {
			item := taskHistoryItem(m, reasons, r.Today)
			r.Items = append(r.Items, item)
			r.StatusCounts[item.Status]++
			for _, reason := range reasons {
				r.RelationCounts[reason]++
			}
			if taskHistoryOpen(item.Status) {
				r.OpenCount++
			}
			if item.CurrentOverdue {
				r.CurrentOverdueCount++
			}
			if item.FinishedLate {
				r.FinishedLateCount++
			}
			if item.Deadline == "" {
				r.NoDeadlineCount++
			}
		}
	}
	if ctx.Err() != nil {
		r.Truncated = true
		r.Failures = append(r.Failures, "query deadline exceeded")
	}
	verify, done := context.WithTimeout(parent, 3*time.Second)
	defer done()
	after, e := s.monthlyIdentity(verify)
	if e != nil {
		return TaskHistoryResult{}, fmt.Errorf("identity recheck failed")
	}
	afterID, _ := integer(after["id"])
	afterVisible, e := taskHistoryVisibleSprints(after)
	if e != nil || afterID != r.UserID || after["account"] != r.Account || !reflect.DeepEqual(visible, afterVisible) {
		return TaskHistoryResult{}, fmt.Errorf("identity or execution visibility changed")
	}
	r.Count = len(r.Items)
	r.ScanComplete = len(r.Failures) == 0 && !r.Truncated
	r.Complete = r.ScanComplete && in.Mode == "all"
	if in.Mode == "year" {
		r.Limitations = append(r.Limitations, "Year mode filters task date fields before reading actions; it can omit tasks with only historical activity in the year and never proves full annual coverage.")
	}
	if len(r.Items) > 200 {
		r.Items = r.Items[:200]
		r.Truncated = true
		r.Complete = false
		r.Limitations = append(r.Limitations, "Only first 200 matched items returned; counts describe scanned matches.")
	}
	return r, nil
}
