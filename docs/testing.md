# Testing

## Overview

Tests are written using the standard Go `testing` package. The test runner is `go test`. All test files end in `_test.go` and are co-located with the packages they test.

## Running Tests

### Full Test Suite

```bash
make test
```

Equivalent to:

```bash
go test -coverprofile=coverage.out -race ./... -v
```

Flags:
- `-coverprofile=coverage.out` writes coverage data to `coverage.out`
- `-race` enables the race detector (finds concurrent access bugs)
- `-v` prints each test name and pass/fail status

### Full Quality Gate (Required Before Committing)

```bash
make quality-gate
```

Runs lint, format check, vet, and tests in order. All steps must pass.

### Single Package

```bash
go test ./internal/security/...
```

### With Coverage Report

```bash
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
go tool cover -html=coverage.out  # opens in browser
```

## Coverage Threshold

The CI pipeline enforces a minimum of **80% total line coverage**. Builds that fall below this threshold fail.

Coverage is measured with:

```bash
go test -coverprofile=coverage.out -race ./... -v
COVERAGE=$(go tool cover -func=coverage.out | grep total | awk '{print $3}' | tr -d '%')
awk -v cov="$COVERAGE" 'BEGIN { if (cov < 80.0) { print "Coverage below 80%"; exit 1 } }'
```

## Test Inventory

| File | Package | Tests | What It Covers |
|---|---|---|---|
| `internal/config/config_test.go` | `config` | 10 | `Load()`: required variable enforcement, default values, positive-integer validation, `MCP_PRO_CONCURRENCY <= MCP_GLOBAL_CONCURRENCY` constraint, invalid log level rejection |
| `internal/gemini/client_test.go` | `gemini` | 4 | `buildGenerateInputs`, `buildEditInputs`, `extractImage` (nil response, empty candidates, text-only, nil content, valid image, disallowed MIME, JPEG/WebP acceptance), pro semaphore cancellation |
| `internal/gemini/errors_test.go` | `gemini` | 8 | `MapError`: nil input, `genai.APIError` with various HTTP status codes (400, 403, 404, 429, 500+), string-based fallback classification |
| `internal/gemini/registry_test.go` | `gemini` | 9 | `LookupModel`, `AllModelsSafe` (no GeminiID, sorted order), `ValidAliases`, `ValidateRegistryAtStartup` (passes with real IDs, fails with sentinel) |
| `internal/policy/selector_test.go` | `policy` | 16 | `Recommend`: speed priority, quality priority, balanced with pro keywords, balanced with speed keywords, balanced with no keywords, empty priority treated as balanced, unknown priority normalized to balanced, case-insensitivity |
| `internal/security/sanitize_test.go` | `security` | 7 | `RegisterSecret` and `SanitizeString`: exact secret match, Gemini API key pattern match, log injection prevention (`\n`, `\r`), multiple registered secrets, empty secret ignored, `ClearSecrets` cleanup |
| `internal/security/validate_test.go` | `security` | 26 | All validators: `ValidatePrompt` (empty, max length, null bytes), `ValidateModelAlias` (empty, valid, invalid), `ValidateAspectRatio` (all five valid values, invalid), `ValidatePriority` (all three valid values, invalid), `ValidateAndDecodeImage` (empty, invalid base64, oversized, unsupported MIME, too small, magic byte mismatch for PNG/JPEG/WebP, valid cases), `ValidateTaskDescription` (empty, max length) |
| `internal/server/server_test.go` | `server_test` | 8 | HTTP middleware integration: correct bearer token, wrong token (401), missing token (401), rate limit exhaustion (429 + Retry-After), health check bypasses auth and rate limit, panic recovery (500), `tools/list` integration, oversized body (413) |
| `internal/tools/tools_test.go` | `tools` | 9 | Tool handlers: `generate_image` success, empty prompt, invalid model, Gemini error (no raw text leak, safe code returned), `list_models` (no GeminiID in output, valid JSON array), `recommend_model` success, empty task description, `edit_image` success, invalid image data |

**Total: 97 test functions** across 9 test files.

## Testing Patterns

### Dependency Injection via Interfaces

All tool handlers accept `gemini.GeminiService` (an interface), not `*gemini.Client` (the concrete type). Tests implement the interface with a mock struct:

```go
// internal/tools/tools_test.go
type mockGeminiService struct {
    generateResult *gemini.ImageResult
    generateError  error
    editResult     *gemini.ImageResult
    editError      error
}

func (mock *mockGeminiService) GenerateImage(...) (*gemini.ImageResult, error) {
    return mock.generateResult, mock.generateError
}
```

This allows tool handler tests to run without a network connection or API key.

### Environment Variable Isolation

`test.Setenv` sets an environment variable for the duration of a single test and automatically restores the original value when the test ends. No manual cleanup is needed:

```go
// internal/config/config_test.go
test.Setenv("GEMINI_API_KEY", "test-key")
test.Setenv("MCP_LOG_LEVEL", "debug")
cfg, err := Load()
```

### Table-Driven Tests

Repetitive cases (valid/invalid inputs) use subtests to isolate failures:

```go
// internal/gemini/client_test.go
test.Run("basic prompt without aspect ratio", func(test *testing.T) {
    // ...
})
test.Run("prompt with aspect ratio sets system instruction", func(test *testing.T) {
    // ...
})
```

Each subtest appears as a separate line in `go test -v` output and can be run individually:

```bash
go test ./internal/gemini/... -run TestBuildGenerateInputs/basic_prompt
```

### Test Helpers

Functions shared across tests are marked with `test.Helper()` so that failure line numbers point to the call site, not the helper body:

```go
func extractTextContent(test *testing.T, result *mcp.CallToolResult) string {
    test.Helper()
    textContent, ok := result.Content[0].(mcp.TextContent)
    if !ok {
        test.Fatalf("expected TextContent, got %T", result.Content[0])
    }
    return textContent.Text
}
```

### httptest for Middleware

`net/http/httptest` allows testing the full HTTP middleware stack without starting a real server:

```go
// internal/server/server_test.go
req := httptest.NewRequest(http.MethodPost, "/mcp", jsonRPCBody("tools/list"))
req.Header.Set("Authorization", "Bearer test-secret")
rec := httptest.NewRecorder()
handler.ServeHTTP(rec, req)

if rec.Code != http.StatusOK { ... }
```

### Security-Specific Tests

Several tests explicitly verify security invariants:

**No raw error text leaked from Gemini errors:**
```go
// internal/tools/tools_test.go
svc := &mockGeminiService{generateError: errors.New("quota exceeded somewhere")}
// ...
if strings.Contains(textContent, "quota exceeded somewhere") {
    test.Errorf("raw error text must not appear in response, got: %q", textContent)
}
```

**No GeminiID in list_models output:**
```go
// internal/tools/tools_test.go
if strings.Contains(textContent, "gemini_id") || strings.Contains(textContent, "GeminiID") {
    test.Errorf("SECURITY: response must not contain gemini_id or GeminiID, got: %q", textContent)
}
```

**Secret redaction in sanitizer:**
```go
// internal/security/sanitize_test.go
RegisterSecret("super-secret-value")
output := SanitizeString("token: super-secret-value in a string")
// output must contain [REDACTED], not the secret
```
