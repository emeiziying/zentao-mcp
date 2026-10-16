package mcp

import (
	"context"
	"github.com/merzzzl/openapi-mcp-server/internal/models"
	"github.com/merzzzl/openapi-mcp-server/internal/service/tool"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"testing"
)

func TestHistoryRegistrationRequiresSchemaAndPolicy(t *testing.T) {
	for _, kind := range []string{"task", "bug"} {
		listPath, detailPath := "/executions/{id}/tasks", "/tasks/{id}"
		if kind == "bug" {
			listPath, detailPath = "/products/{id}/bugs", "/bugs/{id}"
		}
		defs := []models.ToolDefinition{{Method: "GET", Path: "/user"}, {Method: "GET", Path: listPath}, {Method: "GET", Path: detailPath}}
		for _, tc := range []struct {
			name  string
			defs  []models.ToolDefinition
			allow bool
			count int
		}{{"correct", defs, true, 1}, {"unrelated", nil, true, 0}, {"missing route", defs[:1], true, 0}, {"denied", defs, false, 0}} {
			t.Run(kind+"/"+tc.name, func(t *testing.T) {
				server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "test"}, nil)
				svc := tool.New(nil, "")
				c := &Controller{tool: svc, operationChecker: func(string, string) bool { return tc.allow }}
				if kind == "task" {
					c.registerTaskHistoryTool(server, tc.defs)
				} else {
					c.registerBugHistoryTool(server, tc.defs)
				}
				st, ct := mcpsdk.NewInMemoryTransports()
				ctx := context.Background()
				ss, e := server.Connect(ctx, st, nil)
				if e != nil {
					t.Fatal(e)
				}
				defer ss.Close()
				client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test"}, nil)
				cs, e := client.Connect(ctx, ct, nil)
				if e != nil {
					t.Fatal(e)
				}
				defer cs.Close()
				listed, e := cs.ListTools(ctx, nil)
				if e != nil {
					t.Fatal(e)
				}
				if len(listed.Tools) != tc.count {
					t.Fatalf("unexpected tools: %+v", listed.Tools)
				}
				if tc.count == 1 && !listed.Tools[0].Annotations.ReadOnlyHint {
					t.Fatal("not read-only")
				}
			})
		}
	}

}
