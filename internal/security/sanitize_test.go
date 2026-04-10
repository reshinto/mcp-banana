package security_test

import (
	"testing"

	"github.com/reshinto/mcp-banana/internal/security"
)

func TestRegisterSecret_EmptyString(test *testing.T) {
	test.Cleanup(security.ClearSecrets)

	// Registering an empty secret should be a no-op.
	security.RegisterSecret("")
	// If empty string were registered, it would replace every character boundary.
	// Verify that sanitization still works normally.
	input := "no secrets here"
	result := security.SanitizeString(input)
	if result != input {
		test.Errorf("expected %q unchanged after registering empty secret, got %q", input, result)
	}
}

func TestSanitizeString_NoSecretsRegistered(test *testing.T) {
	test.Cleanup(security.ClearSecrets)

	input := "hello world, nothing sensitive here"
	result := security.SanitizeString(input)
	if result != input {
		test.Errorf("expected %q, got %q", input, result)
	}
}

func TestSanitizeString_RedactsGeminiAPIKeyPattern(test *testing.T) {
	test.Cleanup(security.ClearSecrets)

	// The suffix after "AIza" must be exactly 35 alphanumeric/dash/underscore characters.
	// SECURITY: Build the test key via concatenation so no single source literal matches
	// the Google API key pattern that secret scanners detect.
	testKey := "AIza" + "SyABCDEFGHIJKLMNOPQRSTUVWXYZ0123456"
	input := "key=" + testKey + " is set"
	result := security.SanitizeString(input)
	expected := "key=[REDACTED] is set"
	if result != expected {
		test.Errorf("expected %q, got %q", expected, result)
	}
}

func TestSanitizeString_RedactsRegisteredSecret(test *testing.T) {
	test.Cleanup(security.ClearSecrets)

	security.RegisterSecret("supersecrettoken")
	input := "Authorization: Bearer supersecrettoken"
	result := security.SanitizeString(input)
	expected := "Authorization: Bearer [REDACTED]"
	if result != expected {
		test.Errorf("expected %q, got %q", expected, result)
	}
}

func TestSanitizeString_MultipleSecretsInOneString(test *testing.T) {
	test.Cleanup(security.ClearSecrets)

	security.RegisterSecret("secretone")
	security.RegisterSecret("secrettwo")
	input := "first secretone then secrettwo"
	result := security.SanitizeString(input)
	expected := "first [REDACTED] then [REDACTED]"
	if result != expected {
		test.Errorf("expected %q, got %q", expected, result)
	}
}

func TestSanitizeString_EmptyInput(test *testing.T) {
	test.Cleanup(security.ClearSecrets)

	result := security.SanitizeString("")
	if result != "" {
		test.Errorf("expected empty string, got %q", result)
	}
}

func TestSanitizeString_StripsNewline(test *testing.T) {
	test.Cleanup(security.ClearSecrets)

	input := "line one\nline two"
	result := security.SanitizeString(input)
	expected := "line oneline two"
	if result != expected {
		test.Errorf("expected %q, got %q", expected, result)
	}
}

func TestSanitizeString_StripsCarriageReturn(test *testing.T) {
	test.Cleanup(security.ClearSecrets)

	input := "line one\rline two"
	result := security.SanitizeString(input)
	expected := "line oneline two"
	if result != expected {
		test.Errorf("expected %q, got %q", expected, result)
	}
}
