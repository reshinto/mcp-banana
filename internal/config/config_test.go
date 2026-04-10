// Package config_test validates that the configuration loading correctly
// reads environment variables, applies defaults, and rejects invalid values.
package config_test

import (
	"testing"

	"github.com/reshinto/mcp-banana/internal/config"
)

// All tests use test.Setenv() exclusively for environment manipulation.
// test.Setenv automatically restores the original value after the test,
// eliminating cross-test pollution. To simulate a missing env var, use
// test.Setenv("VAR_NAME", "") -- empty string is invalid for required
// vars (like GEMINI_API_KEY) and triggers defaults for optional vars.

func TestLoad_ValidConfig(test *testing.T) {
	test.Setenv("GEMINI_API_KEY", "AIzaSyTestKeyThatIsExactly39CharsLong01")
	test.Setenv("MCP_AUTH_TOKEN", "abcdef0123456789abcdef0123456789ab")
	test.Setenv("MCP_LOG_LEVEL", "debug")

	serverConfig, loadError := config.Load()
	if loadError != nil {
		test.Fatalf("unexpected error: %v", loadError)
	}
	if serverConfig.GeminiAPIKey != "AIzaSyTestKeyThatIsExactly39CharsLong01" {
		test.Errorf("unexpected API key: %s", serverConfig.GeminiAPIKey)
	}
	if serverConfig.LogLevel != "debug" {
		test.Errorf("unexpected log level: %s", serverConfig.LogLevel)
	}
}

func TestLoad_MissingAPIKey(test *testing.T) {
	test.Setenv("GEMINI_API_KEY", "")

	_, loadError := config.Load()
	if loadError == nil {
		test.Fatal("expected error for missing API key")
	}
}

func TestLoad_DefaultLogLevel(test *testing.T) {
	test.Setenv("GEMINI_API_KEY", "AIzaSyTestKeyThatIsExactly39CharsLong01")
	test.Setenv("MCP_LOG_LEVEL", "")

	serverConfig, loadError := config.Load()
	if loadError != nil {
		test.Fatalf("unexpected error: %v", loadError)
	}
	if serverConfig.LogLevel != "info" {
		test.Errorf("expected default log level 'info', got: %s", serverConfig.LogLevel)
	}
}

func TestLoad_InvalidLogLevel(test *testing.T) {
	test.Setenv("GEMINI_API_KEY", "AIzaSyTestKeyThatIsExactly39CharsLong01")
	test.Setenv("MCP_LOG_LEVEL", "GARBAGE")

	_, loadError := config.Load()
	if loadError == nil {
		test.Fatal("expected error for invalid log level")
	}
}

func TestLoad_MalformedIntegerEnvVar(test *testing.T) {
	test.Setenv("GEMINI_API_KEY", "AIzaSyTestKeyThatIsExactly39CharsLong01")
	test.Setenv("MCP_RATE_LIMIT", "abc")

	_, loadError := config.Load()
	if loadError == nil {
		test.Fatal("expected error for malformed integer env var")
	}
}

func TestLoad_ZeroRateLimit(test *testing.T) {
	test.Setenv("GEMINI_API_KEY", "AIzaSyTestKeyThatIsExactly39CharsLong01")
	test.Setenv("MCP_RATE_LIMIT", "0")

	_, loadError := config.Load()
	if loadError == nil {
		test.Fatal("expected error for zero rate limit")
	}
}

func TestLoad_NegativeConcurrency(test *testing.T) {
	test.Setenv("GEMINI_API_KEY", "AIzaSyTestKeyThatIsExactly39CharsLong01")
	test.Setenv("MCP_PRO_CONCURRENCY", "-1")

	_, loadError := config.Load()
	if loadError == nil {
		test.Fatal("expected error for negative concurrency")
	}
}

func TestLoad_ProConcurrencyExceedsGlobal(test *testing.T) {
	test.Setenv("GEMINI_API_KEY", "AIzaSyTestKeyThatIsExactly39CharsLong01")
	test.Setenv("MCP_GLOBAL_CONCURRENCY", "4")
	test.Setenv("MCP_PRO_CONCURRENCY", "10")

	_, loadError := config.Load()
	if loadError == nil {
		test.Fatal("expected error when pro concurrency exceeds global concurrency")
	}
}

func TestLoad_ZeroTimeout(test *testing.T) {
	test.Setenv("GEMINI_API_KEY", "AIzaSyTestKeyThatIsExactly39CharsLong01")
	test.Setenv("MCP_REQUEST_TIMEOUT_SECS", "0")

	_, loadError := config.Load()
	if loadError == nil {
		test.Fatal("expected error for zero timeout")
	}
}

func TestLoad_DefaultLimits(test *testing.T) {
	test.Setenv("GEMINI_API_KEY", "AIzaSyTestKeyThatIsExactly39CharsLong01")

	serverConfig, loadError := config.Load()
	if loadError != nil {
		test.Fatalf("unexpected error: %v", loadError)
	}
	if serverConfig.RateLimit != 30 {
		test.Errorf("expected default rate limit 30, got %d", serverConfig.RateLimit)
	}
	if serverConfig.GlobalConcurrency != 8 {
		test.Errorf("expected default global concurrency 8, got %d", serverConfig.GlobalConcurrency)
	}
	if serverConfig.RequestTimeoutSecs != 120 {
		test.Errorf("expected default timeout 120, got %d", serverConfig.RequestTimeoutSecs)
	}
}
