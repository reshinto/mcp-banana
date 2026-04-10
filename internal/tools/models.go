package tools

import (
	"context"
	"encoding/json"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/reshinto/mcp-banana/internal/gemini"
)

// NewListModelsHandler returns an MCP tool handler for the list_models tool.
// It returns all registered models as SafeModelInfo (no GeminiID).
// The handler never returns a Go error; internal errors are encoded in CallToolResult.
func NewListModelsHandler() func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		models := gemini.AllModelsSafe()
		jsonBytes, marshalErr := json.Marshal(models)
		if marshalErr != nil {
			return mcp.NewToolResultError("server_error: internal error"), nil
		}
		return mcp.NewToolResultText(string(jsonBytes)), nil
	}
}
