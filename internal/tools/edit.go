package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/reshinto/mcp-banana/internal/gemini"
	"github.com/reshinto/mcp-banana/internal/security"
)

// NewEditImageHandler returns an MCP tool handler for the edit_image tool.
// It validates the instructions, model alias, and image data before forwarding
// to the Gemini service. When clientCache is non-nil and the request context
// contains a per-request API key (set by middleware from the X-Gemini-API-Key
// header), that client is used instead of the default service. The handler never
// returns a Go error; application errors are encoded in CallToolResult.
func NewEditImageHandler(service gemini.GeminiService, clientCache *gemini.ClientCache, maxImageBytes int) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(requestContext context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		resolvedService := service
		if clientCache != nil {
			perRequestKey := gemini.APIKeyFromContext(requestContext)
			if perRequestKey != "" {
				perRequestClient, clientError := clientCache.GetClient(requestContext, perRequestKey)
				if clientError != nil {
					return mcp.NewToolResultError("failed to initialize client for provided API key"), nil
				}
				resolvedService = perRequestClient
			}
		}

		instructions := req.GetString("instructions", "")
		modelAlias := req.GetString("model", defaultModel)
		imageBase64 := req.GetString("image", "")
		mimeType := req.GetString("mime_type", "")

		if err := security.ValidatePrompt(instructions); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid_prompt: %s", err.Error())), nil
		}
		if err := security.ValidateModelAlias(modelAlias); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid_model: %s", err.Error())), nil
		}

		imageBytes, err := security.ValidateAndDecodeImage(imageBase64, mimeType, maxImageBytes)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid_image: %s", err.Error())), nil
		}

		result, err := resolvedService.EditImage(requestContext, modelAlias, imageBytes, mimeType, instructions)
		if err != nil {
			code, message := gemini.MapError(err)
			return mcp.NewToolResultError(fmt.Sprintf("%s: %s", code, message)), nil
		}

		jsonBytes, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(jsonBytes)), nil
	}
}
