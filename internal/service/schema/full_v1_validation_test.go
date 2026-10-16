package schema

import (
	"context"
	"testing"

	openapirepo "github.com/merzzzl/openapi-mcp-server/internal/repository/openapi"
)

func TestFullV1EstimateVersionPayloads(t *testing.T) {
	doc, err := openapirepo.New(nil, "../../../docs/zentao-v1.json").Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	path := doc.Paths.Value("/tasks/{id}/estimate")
	if path == nil || path.Post == nil {
		t.Fatal("estimate endpoint missing")
	}
	resolved := resolveInputJSONSchema(t, New(nil).buildToolInputSchema(context.Background(), doc, path.Post, path))
	for name, payload := range map[string]map[string]any{
		"before_20_7":    {"id": []any{float64(0)}, "objectID": []any{float64(42)}, "dates": []any{"2026-10-16"}, "work": []any{"fixture"}, "consumed": []any{float64(2)}, "left": []any{float64(3)}, "objectType": []any{"task"}},
		"20_7_and_later": {"date": []any{"2026-10-16"}, "work": []any{"fixture"}, "consumed": []any{float64(2)}, "left": []any{float64(3)}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := resolved.Validate(map[string]any{"id": float64(42), "payload": payload}); err != nil {
				t.Fatalf("documented version payload rejected: %v", err)
			}
		})
	}
	for name, input := range map[string]map[string]any{
		"empty":      {"id": float64(42), "payload": map[string]any{}},
		"missing":    {"id": float64(42)},
		"incomplete": {"id": float64(42), "payload": map[string]any{"work": []any{"fixture"}}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := resolved.Validate(input); err == nil {
				t.Fatal("invalid estimate payload accepted")
			}
		})
	}
}

func TestFullV1CreateTaskRequiredFields(t *testing.T) {
	doc, err := openapirepo.New(nil, "../../../docs/zentao-v1.json").Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	path := doc.Paths.Value("/executions/{id}/tasks")
	if path == nil || path.Post == nil {
		t.Fatal("create task endpoint missing")
	}
	resolved := resolveInputJSONSchema(t, New(nil).buildToolInputSchema(context.Background(), doc, path.Post, path))
	full := map[string]any{"name": "fixture", "type": "devel", "assignedTo": "fixture-user", "estStarted": "2026-10-16", "deadline": "2026-10-17"}
	if err := resolved.Validate(map[string]any{"id": float64(42), "payload": full}); err != nil {
		t.Fatalf("valid creation rejected: %v", err)
	}
	for key := range full {
		t.Run("missing_"+key, func(t *testing.T) {
			partial := map[string]any{}
			for k, v := range full {
				if k != key {
					partial[k] = v
				}
			}
			if err := resolved.Validate(map[string]any{"id": float64(42), "payload": partial}); err == nil {
				t.Fatalf("missing %s accepted", key)
			}
		})
	}
}
