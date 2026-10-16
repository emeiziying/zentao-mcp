package tool

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Identity and upstream are deliberately not selectable by the caller.
type BugHistoryInput struct {
	ScanAllVisible bool `json:"scan_all_visible,omitempty"`
	Year           int  `json:"year,omitempty"`
	ProductID      int  `json:"product_id,omitempty"`
}
type BugHistoryEntry struct {
	ID        int               `json:"id"`
	ProductID int               `json:"product_id"`
	Title     string            `json:"title"`
	Status    string            `json:"status"`
	Relations []string          `json:"relations"`
	Dates     map[string]string `json:"event_dates"`
}
type BugHistoryResult struct {
	Account         string            `json:"account"`
	UserID          int               `json:"user_id"`
	Year            int               `json:"year"`
	Complete        bool              `json:"complete"`
	Scope           string            `json:"scope"`
	Entries         []BugHistoryEntry `json:"entries"`
	Failures        []string          `json:"failures"`
	Limitations     []string          `json:"limitations"`
	Truncated       bool              `json:"truncated"`
	ProductsScanned int               `json:"products_scanned"`
}

func (s *Service) bugHistoryIdentity(ctx context.Context) (map[string]any, []int, error) {
	raw, e := s.monthlyGet(ctx, "/user", nil)
	if e != nil {
		return nil, nil, e
	}
	root, ok := raw.(map[string]any)
	if !ok {
		return nil, nil, fmt.Errorf("invalid identity")
	}
	profile := root
	if p, ok := root["profile"].(map[string]any); ok {
		profile = p
	}
	id, ok := integer(profile["id"])
	account, _ := profile["account"].(string)
	if !ok || id < 1 || account == "" {
		return nil, nil, fmt.Errorf("invalid identity")
	}
	view, ok := profile["view"].(map[string]any)
	if !ok {
		view, ok = root["view"].(map[string]any)
	}
	if !ok {
		return nil, nil, fmt.Errorf("missing visible product scope")
	}
	rawIDs, exists := view["products"]
	if !exists {
		return nil, nil, fmt.Errorf("missing visible product scope")
	}
	ids := []int{}
	seen := map[int]bool{}
	add := func(v any) error {
		n, valid := integer(v)
		if !valid || n < 1 {
			return fmt.Errorf("invalid visible product scope")
		}
		if !seen[n] {
			ids = append(ids, n)
			seen[n] = true
		}
		return nil
	}
	switch v := rawIDs.(type) {
	case string:
		for _, part := range strings.Split(v, ",") {
			if strings.TrimSpace(part) == "" {
				continue
			}
			if e := add(strings.TrimSpace(part)); e != nil {
				return nil, nil, e
			}
		}
	case []any:
		for _, part := range v {
			if e := add(part); e != nil {
				return nil, nil, e
			}
		}
	default:
		return nil, nil, fmt.Errorf("invalid visible product scope")
	}
	sort.Ints(ids)
	return profile, ids, nil
}
func (s *Service) MyBugHistoryWithPolicy(parent context.Context, in BugHistoryInput, checker func(string, string) bool) (BugHistoryResult, error) {
	if in.ProductID == 0 && !in.ScanAllVisible {
		return BugHistoryResult{}, fmt.Errorf("select product_id or explicitly scan_all_visible")
	}
	if checker == nil {
		return BugHistoryResult{}, fmt.Errorf("missing read policy")
	}
	parent = context.WithValue(parent, monthlyPolicyKey{}, checker)
	loc, locErr := time.LoadLocation("Asia/Shanghai")
	if locErr != nil {
		return BugHistoryResult{}, fmt.Errorf("timezone unavailable")
	}
	if in.Year != 0 && (in.Year < 1970 || in.Year > time.Now().In(loc).Year()) || in.ProductID < 0 {
		return BugHistoryResult{}, fmt.Errorf("invalid year or product")
	}
	ctx, cancel := context.WithTimeout(parent, 50*time.Second)
	defer cancel()
	identity, products, e := s.bugHistoryIdentity(ctx)
	if e != nil {
		return BugHistoryResult{}, e
	}
	originalProducts := append([]int(nil), products...)
	if in.ProductID != 0 {
		found := false
		for _, id := range products {
			if id == in.ProductID {
				found = true
			}
		}
		if !found {
			return BugHistoryResult{}, fmt.Errorf("product outside personal visibility")
		}
		products = []int{in.ProductID}
	}
	uid, _ := integer(identity["id"])
	account := identity["account"].(string)
	r := BugHistoryResult{Account: account, UserID: uid, Year: in.Year, Complete: true, Scope: "Visible products; exact current openedBy, resolvedBy, closedBy or assignedTo account. Year filters corresponding event dates only.", Entries: []BugHistoryEntry{}, Failures: []string{}, Limitations: []string{"Not a complete history of all past assignments or overwritten resolver/closer fields; no inferred relationship from names, comments or update timestamps."}}
	seen := map[int]bool{}
	if len(products) > 20 {
		products = products[:20]
		r.Truncated = true
		r.Failures = append(r.Failures, "visible product cap exceeded; narrow product_id")
	}
	for _, pid := range products {
		if ctx.Err() != nil || len(seen) >= 2000 {
			r.Truncated = true
			r.Failures = append(r.Failures, "query budget exceeded")
			break
		}
		rows, err := s.bugHistoryList(ctx, fmt.Sprintf("/products/%d/bugs", pid), "bugs")
		if err != nil {
			r.Failures = append(r.Failures, fmt.Sprintf("product %d pagination incomplete", pid))
		} else {
			r.ProductsScanned++
		}
		for _, row := range rows {
			if ctx.Err() != nil || len(seen) >= 2000 {
				r.Truncated = true
				r.Failures = append(r.Failures, "bug scan budget exceeded")
				break
			}
			id, _ := integer(row["id"])
			if seen[id] {
				r.Failures = append(r.Failures, fmt.Sprintf("duplicate bug %d", id))
				continue
			}
			seen[id] = true
			raw, err := s.monthlyGet(ctx, fmt.Sprintf("/bugs/%d", id), nil)
			if err != nil {
				r.Failures = append(r.Failures, fmt.Sprintf("bug %d unavailable", id))
				continue
			}
			b, ok := raw.(map[string]any)
			if !ok {
				r.Failures = append(r.Failures, fmt.Sprintf("bug %d invalid", id))
				continue
			}
			bid, _ := integer(b["id"])
			product, _ := integer(b["product"])
			if bid != id || product != pid {
				r.Failures = append(r.Failures, fmt.Sprintf("bug %d identity mismatch", id))
				continue
			}
			if fmt.Sprint(b["deleted"]) == "1" || b["deleted"] == true {
				continue
			}
			entry := BugHistoryEntry{ID: id, ProductID: pid, Dates: map[string]string{}, Relations: []string{}}
			entry.Title, _ = b["title"].(string)
			entry.Status, _ = b["status"].(string)
			if entry.Status != "active" && entry.Status != "resolved" && entry.Status != "closed" {
				r.Failures = append(r.Failures, fmt.Sprintf("bug %d invalid status", id))
				continue
			}

			for _, field := range []string{"openedBy", "resolvedBy", "closedBy", "assignedTo"} {
				value, present := b[field]
				nullable := field == "assignedTo" || (field == "resolvedBy" && entry.Status != "resolved") || (field == "closedBy" && entry.Status != "closed")
				explicitNull := present && value == nil
				if obj, ok := value.(map[string]any); ok {
					value = obj["account"]
				}
				actor, valid := value.(string)
				if !present || (!valid && !(nullable && explicitNull)) || (!nullable && actor == "") {
					r.Failures = append(r.Failures, fmt.Sprintf("bug %d missing or malformed %s", id, field))
				}
			}
			if len([]rune(entry.Title)) > 500 {
				entry.Title = string([]rune(entry.Title)[:500])
			}

			for _, pair := range [][2]string{{"openedBy", "openedDate"}, {"resolvedBy", "resolvedDate"}, {"closedBy", "closedDate"}, {"assignedTo", "assignedDate"}} {
				value := b[pair[0]]
				if m, ok := value.(map[string]any); ok {
					value = m["account"]
				}
				if value != account {
					continue
				}
				date, _ := b[pair[1]].(string)
				valid := false
				if len(date) >= 10 {
					_, err := time.Parse("2006-01-02", date[:10])
					valid = err == nil
				}
				if in.Year != 0 {
					if !valid {
						r.Failures = append(r.Failures, fmt.Sprintf("bug %d missing %s", id, pair[1]))
						continue
					}
					if date[:4] != strconv.Itoa(in.Year) {
						continue
					}
				}
				entry.Relations = append(entry.Relations, pair[0])
				if valid {
					entry.Dates[pair[1]] = date
				}
			}
			if len(entry.Relations) > 0 {
				r.Entries = append(r.Entries, entry)
			}
		}
	}
	verify, stop := context.WithTimeout(parent, 3*time.Second)
	defer stop()
	after, afterProducts, err := s.bugHistoryIdentity(verify)
	if err != nil {
		return BugHistoryResult{}, fmt.Errorf("identity recheck failed")
	}
	afterID, _ := integer(after["id"])
	if afterID != uid || after["account"] != account {
		return BugHistoryResult{}, fmt.Errorf("identity changed")
	}
	if in.ProductID == 0 && fmt.Sprint(originalProducts) != fmt.Sprint(afterProducts) {
		return BugHistoryResult{}, fmt.Errorf("product visibility changed")
	}
	for _, pid := range products {
		found := false
		for _, p := range afterProducts {
			if p == pid {
				found = true
			}
		}
		if !found {
			return BugHistoryResult{}, fmt.Errorf("product visibility changed")
		}
	}
	r.Complete = len(r.Failures) == 0
	sort.Slice(r.Entries, func(i, j int) bool { return r.Entries[i].ID < r.Entries[j].ID })
	return r, nil
}

func (s *Service) bugHistoryList(ctx context.Context, path string, keys ...string) ([]map[string]any, error) {
	out := []map[string]any{}
	seen := map[int]bool{}
	total := -1
	for page := 1; page <= 20; page++ {
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
