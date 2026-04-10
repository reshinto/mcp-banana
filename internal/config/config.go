// Package config handles loading and validating server configuration from
// environment variables. It provides a single Config struct that holds all
// runtime settings. Secrets (API keys, tokens) are stored here at startup
// and must NEVER be exposed in tool responses, logs, or error messages.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds all server configuration loaded from environment variables.
// This struct is created once at startup and passed to components that need it.
//
// SECURITY: GeminiAPIKey and AuthToken are secrets. They must never appear
// in any tool response, log output, error message, or health check.
type Config struct {
	GeminiAPIKey       string // The Google Gemini API key for authentication
	AuthToken          string // Single bearer token for HTTP auth (optional, legacy)
	AuthTokensFile     string // Path to a file with one bearer token per line (optional, hot-reloaded)
	LogLevel           string // Logging verbosity: "debug", "info", "warn", "error"
	RateLimit          int    // Maximum requests per minute (default: 30)
	GlobalConcurrency  int    // Maximum simultaneous requests across all models (default: 8)
	ProConcurrency     int    // Maximum simultaneous requests for Pro model (default: 3)
	MaxImageBytes      int    // Maximum decoded image size in bytes (default: 4MB)
	RequestTimeoutSecs int    // Timeout for each Gemini API call in seconds (default: 120)
}

// Load reads configuration from environment variables and validates it.
// Returns an error immediately if required values are missing or malformed.
// This "fail fast" approach ensures misconfiguration is caught at startup,
// not when the first request arrives.
func Load() (*Config, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return nil, errors.New("GEMINI_API_KEY is required")
	}

	authToken := os.Getenv("MCP_AUTH_TOKEN")
	authTokensFile := os.Getenv("MCP_AUTH_TOKENS_FILE")

	logLevel := strings.ToLower(os.Getenv("MCP_LOG_LEVEL"))
	if logLevel == "" {
		logLevel = "info"
	}
	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLevels[logLevel] {
		return nil, errors.New("MCP_LOG_LEVEL must be one of: debug, info, warn, error")
	}

	rateLimit, rateLimitError := getEnvInt("MCP_RATE_LIMIT", 30)
	if rateLimitError != nil {
		return nil, rateLimitError
	}
	globalConcurrency, concurrencyError := getEnvInt("MCP_GLOBAL_CONCURRENCY", 8)
	if concurrencyError != nil {
		return nil, concurrencyError
	}
	proConcurrency, proError := getEnvInt("MCP_PRO_CONCURRENCY", 3)
	if proError != nil {
		return nil, proError
	}
	maxImageBytes, imageBytesError := getEnvInt("MCP_MAX_IMAGE_BYTES", 4*1024*1024)
	if imageBytesError != nil {
		return nil, imageBytesError
	}
	requestTimeoutSecs, timeoutError := getEnvInt("MCP_REQUEST_TIMEOUT_SECS", 120)
	if timeoutError != nil {
		return nil, timeoutError
	}

	// Validate that all numeric limits are positive (zero or negative values
	// would cause broken rate limiting, deadlocked semaphores, or disabled timeouts).
	if rateLimit <= 0 {
		return nil, errors.New("MCP_RATE_LIMIT must be a positive integer")
	}
	if globalConcurrency <= 0 {
		return nil, errors.New("MCP_GLOBAL_CONCURRENCY must be a positive integer")
	}
	if proConcurrency <= 0 {
		return nil, errors.New("MCP_PRO_CONCURRENCY must be a positive integer")
	}
	if proConcurrency > globalConcurrency {
		return nil, fmt.Errorf("MCP_PRO_CONCURRENCY (%d) must be <= MCP_GLOBAL_CONCURRENCY (%d)", proConcurrency, globalConcurrency)
	}
	if maxImageBytes <= 0 {
		return nil, errors.New("MCP_MAX_IMAGE_BYTES must be a positive integer")
	}
	if requestTimeoutSecs <= 0 {
		return nil, errors.New("MCP_REQUEST_TIMEOUT_SECS must be a positive integer")
	}

	return &Config{
		GeminiAPIKey:       apiKey,
		AuthToken:          authToken,
		AuthTokensFile:     authTokensFile,
		LogLevel:           logLevel,
		RateLimit:          rateLimit,
		GlobalConcurrency:  globalConcurrency,
		ProConcurrency:     proConcurrency,
		MaxImageBytes:      maxImageBytes,
		RequestTimeoutSecs: requestTimeoutSecs,
	}, nil
}

// getEnvInt reads an integer from an environment variable, returning the
// provided default if the variable is unset. Returns an error if the
// variable is set but cannot be parsed as an integer (fail fast on typos).
func getEnvInt(name string, defaultValue int) (int, error) {
	rawValue := os.Getenv(name)
	if rawValue == "" {
		return defaultValue, nil
	}
	parsed, parseError := strconv.Atoi(rawValue)
	if parseError != nil {
		return 0, fmt.Errorf("%s must be a valid integer, got %q", name, rawValue)
	}
	return parsed, nil
}
