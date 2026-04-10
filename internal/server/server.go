// Package server wires together the MCP protocol server, HTTP routing, and
// middleware into a single http.Handler suitable for use in main.
package server

import (
	"log/slog"
	"net/http"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/reshinto/mcp-banana/internal/config"
	"github.com/reshinto/mcp-banana/internal/gemini"
	"github.com/reshinto/mcp-banana/internal/tools"
)

// NewMCPServer creates and configures an MCP server with all four tool handlers
// registered. Both HTTP and stdio transports share the returned instance.
func NewMCPServer(service gemini.GeminiService, maxImageBytes int) *mcpserver.MCPServer {
	srv := mcpserver.NewMCPServer("mcp-banana", "1.0.0")

	generateImageTool := mcp.NewTool("generate_image",
		mcp.WithDescription("Generate an image from a text prompt using the Gemini image generation API."),
		mcp.WithString("prompt",
			mcp.Required(),
			mcp.Description("Text description of the image to generate."),
		),
		mcp.WithString("model",
			mcp.Description("Optional model alias to use for generation."),
		),
		mcp.WithString("aspect_ratio",
			mcp.Description("Optional aspect ratio for the generated image (e.g. '16:9', '1:1')."),
		),
	)
	srv.AddTool(generateImageTool, tools.NewGenerateImageHandler(service, maxImageBytes))

	editImageTool := mcp.NewTool("edit_image",
		mcp.WithDescription("Edit an existing image using text instructions and the Gemini image editing API."),
		mcp.WithString("instructions",
			mcp.Required(),
			mcp.Description("Text instructions describing how to edit the image."),
		),
		mcp.WithString("model",
			mcp.Description("Optional model alias to use for editing."),
		),
		mcp.WithString("image",
			mcp.Required(),
			mcp.Description("Base64-encoded image data to edit."),
		),
		mcp.WithString("mime_type",
			mcp.Required(),
			mcp.Description("MIME type of the image (e.g. 'image/png', 'image/jpeg')."),
		),
	)
	srv.AddTool(editImageTool, tools.NewEditImageHandler(service, maxImageBytes))

	listModelsTool := mcp.NewTool("list_models",
		mcp.WithDescription("List all available model aliases and their capabilities."),
	)
	srv.AddTool(listModelsTool, tools.NewListModelsHandler())

	recommendModelTool := mcp.NewTool("recommend_model",
		mcp.WithDescription("Recommend a model alias based on a task description and optional priority."),
		mcp.WithString("task_description",
			mcp.Required(),
			mcp.Description("Description of the task you want to perform."),
		),
		mcp.WithString("priority",
			mcp.Description("Optional optimization priority: 'speed', 'quality', or 'cost'."),
		),
	)
	srv.AddTool(recommendModelTool, tools.NewRecommendModelHandler())

	return srv
}

// WrapWithMiddleware applies the full middleware chain (panic recovery, auth,
// rate limiting, concurrency semaphore, body size limit) to an arbitrary
// http.Handler. Exported primarily for testing middleware in isolation.
func WrapWithMiddleware(cfg *config.Config, logger *slog.Logger, inner http.Handler) http.Handler {
	mw := newMiddleware(cfg, logger)
	return mw.WrapHTTP(inner)
}

// NewHTTPHandler wraps an MCP server with HTTP routing and middleware.
// It mounts /healthz directly on the mux and /mcp via the streamable HTTP
// transport. All routes except /healthz pass through the full middleware chain.
func NewHTTPHandler(mcpSrv *mcpserver.MCPServer, serverConfig *config.Config, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusOK)
		writer.Write([]byte(`{"status":"ok"}`)) //nolint:errcheck
	})

	streamableHandler := mcpserver.NewStreamableHTTPServer(mcpSrv,
		mcpserver.WithEndpointPath("/mcp"),
		mcpserver.WithStateLess(true),
	)
	mux.Handle("/mcp", streamableHandler)

	mw := newMiddleware(serverConfig, logger)
	return mw.WrapHTTP(mux)
}
