// Package tools implements MCP tool handlers for mcp-banana.
// Each handler is returned by a factory function that accepts dependencies via parameters,
// enabling dependency injection and testability without global state.
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/reshinto/mcp-banana/internal/gemini"
	"github.com/reshinto/mcp-banana/internal/security"
)

const defaultModel = "nano-banana-2"

// NewGenerateImageHandler returns an MCP tool handler for the generate_image tool.
// It validates all inputs via the security package before forwarding to the Gemini service.
// The handler never returns a Go error; application errors are encoded in CallToolResult.
func NewGenerateImageHandler(service gemini.GeminiService, maxImageBytes int) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(requestContext context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		prompt := req.GetString("prompt", "")
		modelAlias := req.GetString("model", defaultModel)
		aspectRatio := req.GetString("aspect_ratio", "")

		if err := security.ValidatePrompt(prompt); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid_prompt: %s", err.Error())), nil
		}
		if err := security.ValidateModelAlias(modelAlias); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid_model: %s", err.Error())), nil
		}
		if err := security.ValidateAspectRatio(aspectRatio); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid_aspect_ratio: %s", err.Error())), nil
		}

		result, err := service.GenerateImage(requestContext, modelAlias, prompt, gemini.GenerateOptions{
			AspectRatio: aspectRatio,
		})
		if err != nil {
			code, message := gemini.MapError(err)
			return mcp.NewToolResultError(fmt.Sprintf("%s: %s", code, message)), nil
		}

		jsonBytes, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(jsonBytes)), nil
	}
}
