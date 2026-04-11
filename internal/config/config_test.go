// Package config_test validates that the configuration loading correctly
// reads environment variables, applies defaults, and rejects invalid values.
package config_test

import (
	"testing"

	"github.com/reshinto/mcp-banana/internal/config"
)

// All tests use test.Setenv() exclusively for environment manipulation.
// test.Setenv automatically restores the original value after the test,
// eliminating cross-test pollution.

func TestLoad_ValidConfig(test *testing.T) {
	test.Setenv("MCP_CREDENTIALS_FILE", "/tmp/test-creds.json")
	test.Setenv("MCP_LOG_LEVEL", "debug")

	serverConfig, loadError := config.Load()
	if loadError != nil {
		test.Fatalf("unexpected error: %v", loadError)
	}
	if serverConfig.CredentialsFile != "/tmp/test-creds.json" {
		test.Errorf("unexpected CredentialsFile: %s", serverConfig.CredentialsFile)
	}
	if serverConfig.LogLevel != "debug" {
		test.Errorf("unexpected log level: %s", serverConfig.LogLevel)
	}
}

func TestLoad_CredentialsFilePopulated(test *testing.T) {
	test.Setenv("MCP_CREDENTIALS_FILE", "/tmp/test-creds.json")

	serverConfig, loadError := config.Load()
	if loadError != nil {
		test.Fatalf("unexpected error: %v", loadError)
	}
	if serverConfig.CredentialsFile != "/tmp/test-creds.json" {
		test.Errorf("expected CredentialsFile '/tmp/test-creds.json', got: %s", serverConfig.CredentialsFile)
	}
}

func TestLoad_DefaultCredentialsFile(test *testing.T) {
	serverConfig, loadError := config.Load()
	if loadError != nil {
		test.Fatalf("unexpected error: %v", loadError)
	}
	if serverConfig.CredentialsFile != "credentials.json" {
		test.Errorf("expected default CredentialsFile 'credentials.json', got: %s", serverConfig.CredentialsFile)
	}
}

func TestLoad_DefaultLogLevel(test *testing.T) {
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
	test.Setenv("MCP_LOG_LEVEL", "GARBAGE")

	_, loadError := config.Load()
	if loadError == nil {
		test.Fatal("expected error for invalid log level")
	}
}

func TestLoad_MalformedIntegerEnvVar(test *testing.T) {
	test.Setenv("MCP_RATE_LIMIT", "abc")

	_, loadError := config.Load()
	if loadError == nil {
		test.Fatal("expected error for malformed integer env var")
	}
}

func TestLoad_ZeroRateLimit(test *testing.T) {
	test.Setenv("MCP_RATE_LIMIT", "0")

	_, loadError := config.Load()
	if loadError == nil {
		test.Fatal("expected error for zero rate limit")
	}
}

func TestLoad_NegativeConcurrency(test *testing.T) {
	test.Setenv("MCP_PRO_CONCURRENCY", "-1")

	_, loadError := config.Load()
	if loadError == nil {
		test.Fatal("expected error for negative concurrency")
	}
}

func TestLoad_ProConcurrencyExceedsGlobal(test *testing.T) {
	test.Setenv("MCP_GLOBAL_CONCURRENCY", "4")
	test.Setenv("MCP_PRO_CONCURRENCY", "10")

	_, loadError := config.Load()
	if loadError == nil {
		test.Fatal("expected error when pro concurrency exceeds global concurrency")
	}
}

func TestLoad_ZeroTimeout(test *testing.T) {
	test.Setenv("MCP_REQUEST_TIMEOUT_SECS", "0")

	_, loadError := config.Load()
	if loadError == nil {
		test.Fatal("expected error for zero timeout")
	}
}

func TestLoad_ZeroGlobalConcurrency(test *testing.T) {
	test.Setenv("MCP_GLOBAL_CONCURRENCY", "0")

	_, loadError := config.Load()
	if loadError == nil {
		test.Fatal("expected error for zero global concurrency")
	}
}

func TestLoad_ZeroMaxImageBytes(test *testing.T) {
	test.Setenv("MCP_MAX_IMAGE_BYTES", "0")

	_, loadError := config.Load()
	if loadError == nil {
		test.Fatal("expected error for zero max image bytes")
	}
}

func TestLoad_NegativeTimeout(test *testing.T) {
	test.Setenv("MCP_REQUEST_TIMEOUT_SECS", "-1")

	_, loadError := config.Load()
	if loadError == nil {
		test.Fatal("expected error for negative timeout")
	}
}

func TestLoad_UppercaseLogLevel(test *testing.T) {
	test.Setenv("MCP_LOG_LEVEL", "DEBUG")

	serverConfig, loadError := config.Load()
	if loadError != nil {
		test.Fatalf("unexpected error: %v", loadError)
	}
	if serverConfig.LogLevel != "debug" {
		test.Errorf("expected log level to be lowercased to 'debug', got: %s", serverConfig.LogLevel)
	}
}

func TestLoad_ProConcurrencyEqualsGlobal(test *testing.T) {
	test.Setenv("MCP_GLOBAL_CONCURRENCY", "5")
	test.Setenv("MCP_PRO_CONCURRENCY", "5")

	serverConfig, loadError := config.Load()
	if loadError != nil {
		test.Fatalf("unexpected error: %v", loadError)
	}
	if serverConfig.ProConcurrency != 5 {
		test.Errorf("expected pro concurrency 5, got %d", serverConfig.ProConcurrency)
	}
}

func TestLoad_MalformedGlobalConcurrency(test *testing.T) {
	test.Setenv("MCP_GLOBAL_CONCURRENCY", "abc")

	_, loadError := config.Load()
	if loadError == nil {
		test.Fatal("expected error for malformed global concurrency")
	}
}

func TestLoad_MalformedMaxImageBytes(test *testing.T) {
	test.Setenv("MCP_MAX_IMAGE_BYTES", "abc")

	_, loadError := config.Load()
	if loadError == nil {
		test.Fatal("expected error for malformed max image bytes")
	}
}

func TestLoad_MalformedTimeout(test *testing.T) {
	test.Setenv("MCP_REQUEST_TIMEOUT_SECS", "abc")

	_, loadError := config.Load()
	if loadError == nil {
		test.Fatal("expected error for malformed timeout")
	}
}

func TestLoad_MalformedProConcurrency(test *testing.T) {
	test.Setenv("MCP_PRO_CONCURRENCY", "abc")

	_, loadError := config.Load()
	if loadError == nil {
		test.Fatal("expected error for malformed pro concurrency")
	}
}

func TestLoad_OAuthConfigFields(test *testing.T) {
	test.Setenv("OAUTH_GOOGLE_CLIENT_ID", "google-id")
	test.Setenv("OAUTH_GOOGLE_CLIENT_SECRET", "google-secret")
	test.Setenv("OAUTH_BASE_URL", "https://banana.example.com:8847")

	cfg, loadError := config.Load()
	if loadError != nil {
		test.Fatalf("unexpected error: %v", loadError)
	}
	if cfg.OAuthGoogleClientID != "google-id" {
		test.Errorf("expected OAuthGoogleClientID 'google-id', got '%s'", cfg.OAuthGoogleClientID)
	}
	if cfg.OAuthBaseURL != "https://banana.example.com:8847" {
		test.Errorf("expected OAuthBaseURL, got '%s'", cfg.OAuthBaseURL)
	}
}

func TestLoad_OAuthBaseURL_DerivedFromMCPDomain(test *testing.T) {
	test.Setenv("OAUTH_BASE_URL", "")
	test.Setenv("MCP_DOMAIN", "banana.example.com")

	cfg, loadError := config.Load()
	if loadError != nil {
		test.Fatalf("unexpected error: %v", loadError)
	}
	expected := "https://banana.example.com:8847"
	if cfg.OAuthBaseURL != expected {
		test.Errorf("expected OAuthBaseURL %q, got %q", expected, cfg.OAuthBaseURL)
	}
}

func TestLoad_TLSConfigFields(test *testing.T) {
	test.Setenv("MCP_TLS_CERT_FILE", "/certs/cert.pem")
	test.Setenv("MCP_TLS_KEY_FILE", "/certs/key.pem")

	cfg, loadError := config.Load()
	if loadError != nil {
		test.Fatalf("unexpected error: %v", loadError)
	}
	if cfg.TLSCertFile != "/certs/cert.pem" {
		test.Errorf("expected TLSCertFile '/certs/cert.pem', got '%s'", cfg.TLSCertFile)
	}
}

func TestLoad_TLSPartialConfig_ReturnsError(test *testing.T) {
	test.Setenv("MCP_TLS_CERT_FILE", "/certs/cert.pem")
	// MCP_TLS_KEY_FILE intentionally missing

	_, loadError := config.Load()
	if loadError == nil {
		test.Errorf("expected error when only one TLS file is set")
	}
}

func TestLoad_DefaultLimits(test *testing.T) {
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
