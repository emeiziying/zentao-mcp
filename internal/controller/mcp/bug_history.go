package mcp

import (
	"context"
	"github.com/merzzzl/openapi-mcp-server/internal/models"
	"github.com/merzzzl/openapi-mcp-server/internal/service/tool"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func (c *Controller) registerBugHistoryTool(server *mcpsdk.Server, definitions []models.ToolDefinition) {
	if c.tool == nil || c.operationChecker == nil {
		return
	}
	required := map[string]bool{"/user": false, "/products/{id}/bugs": false, "/bugs/{id}": false}
	for _, td := range definitions {
		if td.Method == "GET" {
			if _, ok := required[td.Path]; ok {
				required[td.Path] = true
			}
		}
	}
	for _, ok := range required {
		if !ok {
			return
		}
	}
	for _, path := range []string{"/user", "/products/1/bugs", "/bugs/1"} {
		if !c.operationChecker("GET", path) {
			return
		}
	}
	mcpsdk.AddTool(server, &mcpsdk.Tool{Name: "zentao_my_bug_history", Description: "Read authenticated user's Bugs across visible products, explicitly including resolved and closed statuses. Optional year filters exact relationship event dates. Current relationship fields only: not a complete past-assignment audit. Inspect complete, scope and limitations.", Annotations: &mcpsdk.ToolAnnotations{ReadOnlyHint: true}}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, in tool.BugHistoryInput) (*mcpsdk.CallToolResult, any, error) {
		out, err := c.tool.MyBugHistoryWithPolicy(ctx, in, c.operationChecker)
		return nil, out, err
	})
}
