package mcp

import (
	"context"
	"github.com/merzzzl/openapi-mcp-server/internal/models"
	"github.com/merzzzl/openapi-mcp-server/internal/service/tool"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func (c *Controller) registerTaskHistoryTool(server *mcpsdk.Server, definitions []models.ToolDefinition) {
	if c.tool == nil || c.operationChecker == nil {
		return
	}
	required := map[string]bool{"/user": false, "/executions/{id}/tasks": false, "/tasks/{id}": false}
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
	for _, path := range []string{"/user", "/executions/1/tasks", "/tasks/1"} {
		if !c.operationChecker("GET", path) {
			return
		}
	}
	mcpsdk.AddTool(server, &mcpsdk.Tool{Name: "zentao_my_task_history", Description: "Read authenticated user historical task participation and annual/overdue statistics. Requires execution_ids or explicit scan_all_visible. year mode is faster but incomplete for historical-only activity. all audits selected currently-visible executions within a bounded time. Check complete, failures and limitations. No selectable account or today.", Annotations: &mcpsdk.ToolAnnotations{ReadOnlyHint: true}}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, in tool.TaskHistoryInput) (*mcpsdk.CallToolResult, any, error) {
		out, err := c.tool.MyTaskHistoryWithPolicy(ctx, in, c.operationChecker)
		return nil, out, err
	})
}
