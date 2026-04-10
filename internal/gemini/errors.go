// Package gemini provides the Gemini API client and model registry.
//
// SECURITY: This file contains the safe error mapping boundary. Raw SDK error
// text (which can contain API keys and request headers) is never forwarded
// to Claude Code. All errors are mapped to predefined safe codes and messages.
package gemini

import (
	"errors"
	"net/http"
	"strings"

	"google.golang.org/genai"
)

// Error codes returned to Claude Code. These are the ONLY error strings
// that should ever reach the client. Never forward raw SDK error text.
//
// SECURITY: These constants form a strict allowlist. No other error text
// may be returned to callers. This prevents API keys, internal URLs,
// and SDK diagnostics from leaking through error responses.
const (
	ErrContentPolicy  = "content_policy_violation"
	ErrQuotaExceeded  = "quota_exceeded"
	ErrModelUnavail   = "model_unavailable"
	ErrGenerationFail = "generation_failed"
	ErrServerError    = "server_error"
)

// safeMessages maps error codes to safe, human-readable messages.
// These messages contain NO internal details, no SDK error text, no headers.
var safeMessages = map[string]string{
	ErrContentPolicy:  "The prompt was blocked by content safety policy. Rephrase and try again.",
	ErrQuotaExceeded:  "API quota exceeded. Try again later.",
	ErrModelUnavail:   "The requested model is currently unavailable.",
	ErrGenerationFail: "Image generation failed. This may be transient -- retry is safe.",
	ErrServerError:    "An internal server error occurred.",
}

// MapError takes a raw error from the Gemini genai SDK and returns a safe
// (code, message) pair. The raw error text is NEVER included in the output.
//
// SECURITY: This is a critical security boundary. The genai SDK's APIError
// type can contain request metadata. We extract ONLY the HTTP status code
// and map it to a predefined safe message. The raw Message field is discarded.
//
// NOTE: This uses genai.APIError (from google.golang.org/genai), NOT the
// legacy googleapi.Error (from google.golang.org/api).
func MapError(inputError error) (code string, message string) {
	if inputError == nil {
		return "", ""
	}

	// Try to unwrap as a genai API error for HTTP status-based classification.
	var apiErrorPointer *genai.APIError
	if errors.As(inputError, &apiErrorPointer) {
		safeCode := mapHTTPStatus(apiErrorPointer.Code)
		return safeCode, safeMessages[safeCode]
	}
	var apiErrorValue genai.APIError
	if errors.As(inputError, &apiErrorValue) {
		safeCode := mapHTTPStatus(apiErrorValue.Code)
		return safeCode, safeMessages[safeCode]
	}

	// Fallback: classify by substring patterns in the error message.
	// SECURITY: We read the error text for classification only. We never
	// include any part of this text in the returned message.
	lowercaseError := strings.ToLower(inputError.Error())
	switch {
	case strings.Contains(lowercaseError, "safety") || strings.Contains(lowercaseError, "blocked"):
		return ErrContentPolicy, safeMessages[ErrContentPolicy]
	case strings.Contains(lowercaseError, "quota") || strings.Contains(lowercaseError, "rate"):
		return ErrQuotaExceeded, safeMessages[ErrQuotaExceeded]
	case strings.Contains(lowercaseError, "not found") || strings.Contains(lowercaseError, "deprecated"):
		return ErrModelUnavail, safeMessages[ErrModelUnavail]
	default:
		return ErrGenerationFail, safeMessages[ErrGenerationFail]
	}
}

// mapHTTPStatus converts an HTTP status code into a safe error code string.
func mapHTTPStatus(status int) string {
	switch {
	case status == http.StatusBadRequest:
		return ErrContentPolicy
	case status == http.StatusForbidden:
		return ErrContentPolicy
	case status == http.StatusNotFound:
		return ErrModelUnavail
	case status == http.StatusTooManyRequests:
		return ErrQuotaExceeded
	case status >= 500:
		return ErrGenerationFail
	default:
		return ErrGenerationFail
	}
}
