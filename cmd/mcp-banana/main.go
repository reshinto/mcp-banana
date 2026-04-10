// Package main is the entry point for the mcp-banana server.
// It parses command-line flags and starts either the stdio or HTTP transport.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mark3labs/mcp-go/server"
	"github.com/reshinto/mcp-banana/internal/config"
	"github.com/reshinto/mcp-banana/internal/gemini"
	"github.com/reshinto/mcp-banana/internal/security"
	internalserver "github.com/reshinto/mcp-banana/internal/server"
)

// version is set at build time via -ldflags="-X main.version=1.0.0".
var version = "dev"

// osExit is os.Exit, overridden in tests to prevent process termination.
var osExit = os.Exit

func main() {
	osExit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// registryValidator validates the model registry at startup. Overridden in tests.
var registryValidator = func() error {
	return gemini.ValidateRegistryAtStartup()
}

// clientFactory creates a Gemini API client. Overridden in tests to avoid real SDK calls.
var clientFactory = func(ctx context.Context, apiKey string, timeoutSecs int, proConcurrency int) (*gemini.Client, error) {
	return gemini.NewClient(ctx, apiKey, timeoutSecs, proConcurrency)
}

// stdioServe runs the MCP server in stdio mode. Overridden in tests.
var stdioServe = func(mcpServer *server.MCPServer) error {
	return server.ServeStdio(mcpServer)
}

// run contains all the main logic and returns an exit code.
// It is extracted from main() to enable comprehensive testing.
func run(args []string, stdout io.Writer, stderr io.Writer) int {
	flagSet := flag.NewFlagSet("mcp-banana", flag.ContinueOnError)
	flagSet.SetOutput(stderr)
	transport := flagSet.String("transport", "stdio", "Transport mode: stdio or http")
	address := flagSet.String("addr", "0.0.0.0:8847", "Address to listen on (HTTP mode only)")
	healthcheck := flagSet.Bool("healthcheck", false, "Run a health check against the running server and exit")
	versionFlag := flagSet.Bool("version", false, "Print version and exit")

	if parseError := flagSet.Parse(args); parseError != nil {
		return 1
	}

	if *versionFlag {
		_, _ = fmt.Fprintf(stdout, "mcp-banana %s\n", version)
		return 0
	}

	if *healthcheck {
		return runHealthCheck(*address, stderr)
	}

	serverConfig, loadError := config.Load()
	if loadError != nil {
		_, _ = fmt.Fprintf(stderr, "failed to load config: %s\n", loadError)
		return 1
	}

	if registryError := registryValidator(); registryError != nil {
		_, _ = fmt.Fprintf(stderr, "registry validation failed: %s\n", registryError)
		return 1
	}

	security.RegisterSecret(serverConfig.GeminiAPIKey)
	if serverConfig.AuthToken != "" {
		security.RegisterSecret(serverConfig.AuthToken)
	}

	logLevel := resolveLogLevel(serverConfig.LogLevel)
	logger := slog.New(slog.NewJSONHandler(stderr, &slog.HandlerOptions{Level: logLevel}))

	if *transport == "http" && serverConfig.AuthToken == "" && serverConfig.AuthTokensFile == "" {
		logger.Warn("HTTP mode: no MCP_AUTH_TOKEN or MCP_AUTH_TOKENS_FILE configured -- auth is disabled, relying on network-level security (SSH tunnel)")
	}

	startupContext := context.Background()
	geminiClient, clientError := clientFactory(
		startupContext,
		serverConfig.GeminiAPIKey,
		serverConfig.RequestTimeoutSecs,
		serverConfig.ProConcurrency,
	)
	if clientError != nil {
		_, _ = fmt.Fprintf(stderr, "failed to create Gemini client: %s\n", clientError)
		return 1
	}

	mcpServer := internalserver.NewMCPServer(geminiClient, serverConfig.MaxImageBytes)

	switch *transport {
	case "stdio":
		logger.Info("starting mcp-banana in stdio mode", "version", version)
		if stdioError := stdioServe(mcpServer); stdioError != nil {
			_, _ = fmt.Fprintf(stderr, "stdio server error: %s\n", stdioError)
			return 1
		}
	case "http":
		return runHTTPServer(mcpServer, serverConfig, logger, *address)
	default:
		_, _ = fmt.Fprintf(stderr, "unknown transport: %s (must be stdio or http)\n", *transport)
		return 1
	}

	return 0
}

// resolveLogLevel maps a log level string to the corresponding slog.Level.
func resolveLogLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// runHealthCheck performs a health check against the running server.
func runHealthCheck(address string, stderr io.Writer) int {
	httpClient := &http.Client{Timeout: 5 * time.Second}
	healthResponse, fetchError := httpClient.Get(fmt.Sprintf("http://%s/healthz", address))
	if fetchError != nil {
		_, _ = fmt.Fprintf(stderr, "health check failed: %s\n", fetchError)
		return 1
	}
	defer func() { _ = healthResponse.Body.Close() }()
	if healthResponse.StatusCode != http.StatusOK {
		_, _ = fmt.Fprintf(stderr, "health check returned status %d\n", healthResponse.StatusCode)
		return 1
	}
	return 0
}

// listenFunc creates a network listener. Overridden in tests.
var listenFunc = func(network, address string) (net.Listener, error) {
	return net.Listen(network, address)
}

// shutdownTimeout is the graceful shutdown duration. Overridden in tests.
var shutdownTimeout = 120 * time.Second

// runHTTPServer starts the HTTP server with graceful shutdown.
func runHTTPServer(mcpServer *server.MCPServer, serverConfig *config.Config, logger *slog.Logger, address string) int {
	handler := internalserver.NewHTTPHandler(mcpServer, serverConfig, logger)
	httpServer := &http.Server{
		Handler:     handler,
		ReadTimeout: 30 * time.Second,
		IdleTimeout: 120 * time.Second,
	}

	listener, listenError := listenFunc("tcp", address)
	if listenError != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to listen on %s: %s\n", address, listenError)
		return 1
	}

	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		sig := <-stopChan
		logger.Info("received shutdown signal", "signal", sig)
		shutdownContext, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer shutdownCancel()
		if shutdownError := httpServer.Shutdown(shutdownContext); shutdownError != nil {
			logger.Error("graceful shutdown failed", "error", shutdownError)
		}
	}()

	logger.Info("starting mcp-banana in HTTP mode", "addr", address, "version", version)
	if serveError := httpServer.Serve(listener); serveError != nil && serveError != http.ErrServerClosed {
		_, _ = fmt.Fprintf(os.Stderr, "HTTP server error: %s\n", serveError)
		return 1
	}
	logger.Info("server shutdown complete")
	return 0
}
