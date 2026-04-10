package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/reshinto/mcp-banana/internal/policy"
	"github.com/reshinto/mcp-banana/internal/security"
)

// NewRecommendModelHandler returns an MCP tool handler for the recommend_model tool.
// It validates the task description and priority before calling the policy selector.
// The handler never returns a Go error; application errors are encoded in CallToolResult.
func NewRecommendModelHandler() func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		taskDescription := req.GetString("task_description", "")
		priority := req.GetString("priority", "")

		if err := security.ValidateTaskDescription(taskDescription); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid_task_description: %s", err.Error())), nil
		}
		if err := security.ValidatePriority(priority); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid_priority: %s", err.Error())), nil
		}

		recommendation := policy.Recommend(taskDescription, priority)
		jsonBytes, _ := json.Marshal(recommendation)
		return mcp.NewToolResultText(string(jsonBytes)), nil
	}
}
