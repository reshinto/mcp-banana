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
// SECURITY: CredentialsFile contains paths to secrets and OAuth client secrets are secrets.
// They must never appear in any tool response, log output, error message, or health check.
type Config struct {
	CredentialsFile    string // Path to unified credentials JSON file (optional, hot-reloaded)
	LogLevel           string // Logging verbosity: "debug", "info", "warn", "error"
	RateLimit          int    // Maximum requests per minute (default: 30)
	GlobalConcurrency  int    // Maximum simultaneous requests across all models (default: 8)
	ProConcurrency     int    // Maximum simultaneous requests for Pro model (default: 3)
	MaxImageBytes      int    // Maximum decoded image size in bytes (default: 4MB)
	RequestTimeoutSecs int    // Timeout for each Gemini API call in seconds (default: 120)

	// OAuth provider credentials (all optional; omit a provider by leaving its ID or secret empty)
	OAuthGoogleClientID     string // Google OAuth 2.0 client ID
	OAuthGoogleClientSecret string // Google OAuth 2.0 client secret (SECURITY: never log)
	OAuthGitHubClientID     string // GitHub OAuth 2.0 client ID
	OAuthGitHubClientSecret string // GitHub OAuth 2.0 client secret (SECURITY: never log)
	OAuthAppleClientID      string // Apple Sign-In client ID
	OAuthAppleClientSecret  string // Apple Sign-In client secret (SECURITY: never log)
	OAuthBaseURL            string // Public base URL for OAuth redirect and metadata endpoints

	// TLS configuration (both MCP_TLS_CERT_FILE and MCP_TLS_KEY_FILE must be set together, or neither)
	TLSCertFile string // Path to TLS certificate PEM file
	TLSKeyFile  string // Path to TLS private key PEM file
}

// Load reads configuration from environment variables and validates it.
// Returns an error immediately if required values are missing or malformed.
// This "fail fast" approach ensures misconfiguration is caught at startup,
// not when the first request arrives.
func Load() (*Config, error) {
	credentialsFile := os.Getenv("MCP_CREDENTIALS_FILE")
	if credentialsFile == "" {
		credentialsFile = "credentials.json"
	}

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

	cfg := &Config{
		CredentialsFile:    credentialsFile,
		LogLevel:           logLevel,
		RateLimit:          rateLimit,
		GlobalConcurrency:  globalConcurrency,
		ProConcurrency:     proConcurrency,
		MaxImageBytes:      maxImageBytes,
		RequestTimeoutSecs: requestTimeoutSecs,
	}

	// OAuth provider configuration (all optional)
	cfg.OAuthGoogleClientID = os.Getenv("OAUTH_GOOGLE_CLIENT_ID")
	cfg.OAuthGoogleClientSecret = os.Getenv("OAUTH_GOOGLE_CLIENT_SECRET")
	cfg.OAuthGitHubClientID = os.Getenv("OAUTH_GITHUB_CLIENT_ID")
	cfg.OAuthGitHubClientSecret = os.Getenv("OAUTH_GITHUB_CLIENT_SECRET")
	cfg.OAuthAppleClientID = os.Getenv("OAUTH_APPLE_CLIENT_ID")
	cfg.OAuthAppleClientSecret = os.Getenv("OAUTH_APPLE_CLIENT_SECRET")
	cfg.OAuthBaseURL = os.Getenv("OAUTH_BASE_URL")
	// Auto-derive OAUTH_BASE_URL from MCP_DOMAIN if not explicitly set
	if cfg.OAuthBaseURL == "" {
		mcpDomain := os.Getenv("MCP_DOMAIN")
		if mcpDomain != "" {
			cfg.OAuthBaseURL = "https://" + mcpDomain + ":8847"
		}
	}

	// TLS configuration (both required together, or neither)
	cfg.TLSCertFile = os.Getenv("MCP_TLS_CERT_FILE")
	cfg.TLSKeyFile = os.Getenv("MCP_TLS_KEY_FILE")
	if (cfg.TLSCertFile != "" && cfg.TLSKeyFile == "") || (cfg.TLSCertFile == "" && cfg.TLSKeyFile != "") {
		return nil, fmt.Errorf("both MCP_TLS_CERT_FILE and MCP_TLS_KEY_FILE must be set together")
	}

	return cfg, nil
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
