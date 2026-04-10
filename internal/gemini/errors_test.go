// Package gemini_test validates the safe error mapping that prevents
// Gemini SDK error details from leaking to Claude Code.
package gemini_test

import (
	"errors"
	"testing"

	"github.com/reshinto/mcp-banana/internal/gemini"
	"google.golang.org/genai"
)

func TestMapError_NilError(test *testing.T) {
	code, message := gemini.MapError(nil)
	if code != "" || message != "" {
		test.Errorf("expected empty code/message for nil error, got %q/%q", code, message)
	}
}

func TestMapError_GenericError(test *testing.T) {
	code, message := gemini.MapError(errors.New("something broke"))
	if code != "generation_failed" {
		test.Errorf("expected 'generation_failed', got %q", code)
	}
	if message == "" {
		test.Error("expected non-empty message")
	}
}

func TestMapError_NeverLeaksRawText(test *testing.T) {
	// SECURITY: Verify that raw error text (which could contain API keys
	// or request headers) is never included in the safe output.
	sensitiveError := errors.New("Authorization: Bearer AIzaSySecretKeyHere12345678901234567")
	code, message := gemini.MapError(sensitiveError)
	if code == "" {
		test.Error("expected non-empty code")
	}
	if containsSubstring(message, "AIzaSy") {
		test.Fatal("SECURITY: API key pattern leaked into error message")
	}
	if containsSubstring(message, "Authorization") {
		test.Fatal("SECURITY: auth header leaked into error message")
	}
}

func TestMapError_BadRequest(test *testing.T) {
	apiError := &genai.APIError{Code: 400, Message: "blocked by safety"}
	code, _ := gemini.MapError(apiError)
	if code != "content_policy_violation" {
		test.Errorf("expected content_policy_violation for 400, got %q", code)
	}
}

func TestMapError_Forbidden(test *testing.T) {
	apiError := &genai.APIError{Code: 403, Message: "forbidden"}
	code, _ := gemini.MapError(apiError)
	if code != "content_policy_violation" {
		test.Errorf("expected content_policy_violation for 403, got %q", code)
	}
}

func TestMapError_NotFound(test *testing.T) {
	apiError := &genai.APIError{Code: 404, Message: "model not found"}
	code, _ := gemini.MapError(apiError)
	if code != "model_unavailable" {
		test.Errorf("expected model_unavailable for 404, got %q", code)
	}
}

func TestMapError_TooManyRequests(test *testing.T) {
	apiError := &genai.APIError{Code: 429, Message: "quota exceeded"}
	code, _ := gemini.MapError(apiError)
	if code != "quota_exceeded" {
		test.Errorf("expected quota_exceeded for 429, got %q", code)
	}
}

func TestMapError_ServerError(test *testing.T) {
	apiError := &genai.APIError{Code: 500, Message: "internal error with key=AIzaSySecret"}
	code, message := gemini.MapError(apiError)
	if code != "generation_failed" {
		test.Errorf("expected generation_failed for 500, got %q", code)
	}
	// SECURITY: verify the raw Message with the key pattern is NOT returned
	if containsSubstring(message, "AIzaSy") {
		test.Fatal("SECURITY: API key pattern leaked from 500 error")
	}
}

func containsSubstring(haystack, needle string) bool {
	return len(haystack) >= len(needle) && searchSubstring(haystack, needle)
}

func searchSubstring(haystack, needle string) bool {
	for position := 0; position <= len(haystack)-len(needle); position++ {
		if haystack[position:position+len(needle)] == needle {
			return true
		}
	}
	return false
}
