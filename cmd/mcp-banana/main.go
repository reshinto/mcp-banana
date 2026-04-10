// Package main is the entry point for the mcp-banana server.
// It parses command-line flags and starts either the stdio or HTTP transport.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
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

func main() {
	transport := flag.String("transport", "stdio", "Transport mode: stdio or http")
	address := flag.String("addr", "0.0.0.0:8847", "Address to listen on (HTTP mode only)")
	healthcheck := flag.Bool("healthcheck", false, "Run a health check against the running server and exit")
	versionFlag := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("mcp-banana %s\n", version)
		os.Exit(0)
	}

	if *healthcheck {
		httpClient := &http.Client{Timeout: 5 * time.Second}
		healthResponse, fetchError := httpClient.Get(fmt.Sprintf("http://%s/healthz", *address))
		if fetchError != nil {
			fmt.Fprintf(os.Stderr, "health check failed: %s\n", fetchError)
			os.Exit(1)
		}
		defer func() { _ = healthResponse.Body.Close() }()
		if healthResponse.StatusCode != http.StatusOK {
			fmt.Fprintf(os.Stderr, "health check returned status %d\n", healthResponse.StatusCode)
			os.Exit(1)
		}
		os.Exit(0)
	}

	serverConfig, loadError := config.Load()
	if loadError != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %s\n", loadError)
		os.Exit(1)
	}

	if *transport == "http" && serverConfig.AuthToken == "" {
		fmt.Fprintf(os.Stderr, "MCP_AUTH_TOKEN is required for HTTP transport mode\n")
		os.Exit(1)
	}

	if registryError := gemini.ValidateRegistryAtStartup(); registryError != nil {
		fmt.Fprintf(os.Stderr, "registry validation failed: %s\n", registryError)
		os.Exit(1)
	}

	security.RegisterSecret(serverConfig.GeminiAPIKey)
	security.RegisterSecret(serverConfig.AuthToken)

	var logLevel slog.Level
	switch serverConfig.LogLevel {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel}))

	startupContext := context.Background()
	geminiClient, clientError := gemini.NewClient(
		startupContext,
		serverConfig.GeminiAPIKey,
		serverConfig.RequestTimeoutSecs,
		serverConfig.ProConcurrency,
	)
	if clientError != nil {
		fmt.Fprintf(os.Stderr, "failed to create Gemini client: %s\n", clientError)
		os.Exit(1)
	}

	mcpServer := internalserver.NewMCPServer(geminiClient, serverConfig.MaxImageBytes)

	switch *transport {
	case "stdio":
		logger.Info("starting mcp-banana in stdio mode", "version", version)
		if stdioError := server.ServeStdio(mcpServer); stdioError != nil {
			fmt.Fprintf(os.Stderr, "stdio server error: %s\n", stdioError)
			os.Exit(1)
		}
	case "http":
		handler := internalserver.NewHTTPHandler(mcpServer, serverConfig, logger)
		httpServer := &http.Server{
			Addr:        *address,
			Handler:     handler,
			ReadTimeout: 30 * time.Second,
			IdleTimeout: 120 * time.Second,
		}

		stopChan := make(chan os.Signal, 1)
		signal.Notify(stopChan, syscall.SIGTERM, syscall.SIGINT)

		go func() {
			sig := <-stopChan
			logger.Info("received shutdown signal", "signal", sig)
			shutdownContext, shutdownCancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer shutdownCancel()
			if shutdownError := httpServer.Shutdown(shutdownContext); shutdownError != nil {
				logger.Error("graceful shutdown failed", "error", shutdownError)
			}
		}()

		logger.Info("starting mcp-banana in HTTP mode", "addr", *address, "version", version)
		if serveError := httpServer.ListenAndServe(); serveError != nil && serveError != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "HTTP server error: %s\n", serveError)
			os.Exit(1)
		}
		logger.Info("server shutdown complete")
	default:
		fmt.Fprintf(os.Stderr, "unknown transport: %s (must be stdio or http)\n", *transport)
		os.Exit(1)
	}
}
