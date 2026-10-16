package mcp

import (
	"context"
	"github.com/merzzzl/openapi-mcp-server/internal/models"
	"github.com/merzzzl/openapi-mcp-server/internal/service/tool"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func (c *Controller) registerMonthlyTool(server *mcpsdk.Server, definitions []models.ToolDefinition) {
	if c.tool == nil || !c.tool.MonthlyReportEnabled() {
		return
	}
	required := map[string]bool{"/user": false, "/tasks/{id}/estimate": false}
	for _, td := range definitions {
		if td.Method == "GET" {
			if _, ok := required[td.Path]; ok {
				required[td.Path] = true
			}
		}
	}
	for _, found := range required {
		if !found {
			return
		}
	}
	// All concrete reads used by the aggregate must be permitted. Runtime also
	// checks each dynamically discovered path, preventing allowlist bypass.
	for _, p := range []string{"/user", "/tasks/1/estimate"} {
		if !c.operationChecker("GET", p) {
			return
		}
	}
	mcpsdk.AddTool(server, &mcpsdk.Tool{Name: "zentao_my_monthly_worklogs", Description: "Read authenticated user monthly task worklogs discovered from the fixed daily report and independently verified with user token. Includes completed and reassigned tasks. Current year only. Inspect coverage and limitations.", Annotations: &mcpsdk.ToolAnnotations{ReadOnlyHint: true}}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, in tool.MonthlyInput) (*mcpsdk.CallToolResult, any, error) {
		out, err := c.tool.MyMonthlyWorklogsWithPolicy(ctx, in, c.operationChecker)
		return nil, out, err
	})
}
