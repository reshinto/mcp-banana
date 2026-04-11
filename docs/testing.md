# Testing

## Test Runner

All tests use the standard Go `testing` package. The canonical test command is:

```bash
go test -race -coverprofile=coverage.out ./...
```

Flags:
- `-race` enables the race detector to catch concurrent access bugs
- `-coverprofile=coverage.out` writes coverage data for later reporting

## CI Sequence

Run steps in this exact order and fix each before proceeding to the next:

```bash
golangci-lint run
gofmt -w .
go vet ./...
go test -race -coverprofile=coverage.out ./...
```

The `make quality-gate` target runs all four steps in sequence:

```bash
make quality-gate
```

Do not run lint, format, or tests mid-implementation. Run the full CI sequence only at the final quality gate before committing.

## Coverage Thresholds

Minimum line coverage: **80%**. CI fails if total coverage falls below this threshold.

Check coverage after a test run:

```bash
go tool cover -func=coverage.out           # per-function table
go tool cover -html=coverage.out           # open in browser
```

CI uses:

```bash
COVERAGE=$(go tool cover -func=coverage.out | grep total | awk '{print $3}' | tr -d '%')
awk -v cov="$COVERAGE" 'BEGIN { if (cov < 80.0) { print "Coverage below 80%"; exit 1 } }'
```

## Test Location Conventions

- Test files use the `_test.go` suffix and are co-located with the source they test.
- One test file per source file — do not group unrelated tests into a single file.
- Test function parameter is `test *testing.T`, not `t *testing.T`.

```go
// correct
func TestSomething(test *testing.T) { ... }

// wrong — single-character variable name banned by coding standards
func TestSomething(t *testing.T) { ... }
```

## Naming Convention

The `test *testing.T` parameter name is mandatory throughout the codebase. This follows the no-single-character-variable-name rule in the coding standards. The standard Go abbreviation `test` for `*testing.T` is explicitly permitted.

## What to Test

Every new feature or algorithm needs at minimum:

- Correctness tests for the happy path
- One integration or pipeline test exercising the feature end to end
- Edge cases: empty input, maximum bounds, invalid input, type mismatches

Do not test implementation details. Test observable behavior and outputs.

## Security Test Requirements

- Every handler that touches user input must have a test verifying no secrets appear in the response.
- Tests for `internal/gemini/` must verify that `genai.APIError` is safely unwrapped and that raw SDK error text never propagates to the caller.
- Security boundary tests in `internal/security/` must cover sanitization of both input and output.

Example pattern for verifying no raw error text leaks:

```go
svc := &mockGeminiService{generateError: errors.New("quota exceeded somewhere internal")}
// ... call the handler ...
if strings.Contains(textContent, "quota exceeded somewhere internal") {
    test.Errorf("raw error text must not appear in response, got: %q", textContent)
}
```

Example for verifying GeminiID is excluded from list_models:

```go
if strings.Contains(textContent, "gemini_id") || strings.Contains(textContent, "GeminiID") {
    test.Errorf("SECURITY: response must not contain gemini_id or GeminiID, got: %q", textContent)
}
```

## Testing Patterns

### Dependency Injection via Interfaces

All tool handlers accept `gemini.GeminiService` (an interface), not `*gemini.Client` (the concrete type). Tests provide a mock:

```go
type mockGeminiService struct {
    generateResult *gemini.ImageResult
    generateError  error
    editResult     *gemini.ImageResult
    editError      error
}

func (mock *mockGeminiService) GenerateImage(
    requestContext context.Context, modelAlias string, prompt string, options gemini.GenerateOptions,
) (*gemini.ImageResult, error) {
    return mock.generateResult, mock.generateError
}

func (mock *mockGeminiService) EditImage(
    requestContext context.Context, modelAlias string, imageData []byte, mimeType string, instructions string,
) (*gemini.ImageResult, error) {
    return mock.editResult, mock.editError
}
```

This lets handler tests run without a network connection or API key.

### Environment Variable Isolation

Use `test.Setenv` to set environment variables for a single test. The value is automatically restored when the test ends — no manual cleanup needed:

```go
test.Setenv("MCP_CREDENTIALS_FILE", "/tmp/test-creds.json")
test.Setenv("MCP_LOG_LEVEL", "debug")
cfg, loadError := Load()
```

### Table-Driven Tests

Use subtests (`test.Run`) to isolate failures across repetitive cases:

```go
test.Run("basic prompt without aspect ratio", func(test *testing.T) {
    // ...
})
test.Run("prompt with aspect ratio sets system instruction", func(test *testing.T) {
    // ...
})
```

Run a single subtest directly:

```bash
go test ./internal/gemini/... -run TestBuildGenerateInputs/basic_prompt
```

### Test Helpers

Mark shared assertion functions with `test.Helper()` so that failure line numbers point to the call site, not the helper body:

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

Use `net/http/httptest` to test the full middleware stack without starting a real server:

```go
req := httptest.NewRequest(http.MethodPost, "/mcp", jsonRPCBody("tools/list"))
req.Header.Set("Authorization", "Bearer test-secret")
rec := httptest.NewRecorder()
handler.ServeHTTP(rec, req)

if rec.Code != http.StatusOK {
    test.Errorf("expected 200, got %d", rec.Code)
}
```

### Package-Level Variables for Test Injection

`cmd/mcp-banana/main.go` uses package-level `var` declarations for factory functions and the OS exit function. Tests replace these to avoid real SDK calls and process termination:

```go
// in main.go
var osExit = os.Exit
var clientFactory = func(...) (*gemini.Client, error) { return gemini.NewClient(...) }

// in tests
osExit = func(code int) { /* capture code */ }
clientFactory = func(...) (*gemini.Client, error) { return nil, errors.New("injected failure") }
```

## Test Inventory

| File | Package | What It Covers |
|---|---|---|
| `internal/config/config_test.go` | `config` | `Load()`: required variable enforcement, default values, positive-integer validation, `MCP_PRO_CONCURRENCY <= MCP_GLOBAL_CONCURRENCY` constraint, invalid log level rejection |
| `internal/gemini/client_test.go` | `gemini` | `buildGenerateInputs`, `buildEditInputs`, `extractImage` (nil response, empty candidates, text-only parts, nil content, valid image, disallowed MIME), pro semaphore cancellation |
| `internal/gemini/errors_test.go` | `gemini` | `MapError`: nil input, `genai.APIError` with HTTP status codes 400/403/404/429/500+, string-based fallback classification |
| `internal/gemini/registry_test.go` | `gemini` | `LookupModel`, `AllModelsSafe` (no GeminiID, sorted order), `ValidAliases`, `ValidateRegistryAtStartup` (passes with real IDs, fails with sentinel) |
| `internal/policy/selector_test.go` | `policy` | `Recommend`: speed/quality priorities, balanced with pro/speed/no keywords, empty priority treated as balanced, unknown priority normalized to balanced, case-insensitivity |
| `internal/security/sanitize_test.go` | `security` | `RegisterSecret` and `SanitizeString`: exact secret match, Gemini API key pattern match, log injection prevention, multiple secrets, empty secret ignored, `ClearSecrets` cleanup |
| `internal/security/validate_test.go` | `security` | All validators: `ValidatePrompt`, `ValidateModelAlias`, `ValidateAspectRatio`, `ValidatePriority`, `ValidateAndDecodeImage` (bad base64, oversized, unsupported MIME, magic byte mismatch for PNG/JPEG/WebP), `ValidateTaskDescription` |
| `internal/server/server_test.go` | `server_test` | HTTP middleware: correct bearer token, wrong token (401), missing token (401), rate limit exhaustion (429 + Retry-After), health check bypasses auth, panic recovery (500), `tools/list` integration, oversized body (413) |
| `internal/tools/tools_test.go` | `tools` | Tool handlers: `generate_image` success/empty prompt/invalid model/Gemini error (no raw text leak), `list_models` (no GeminiID, valid JSON array), `recommend_model` success/empty description, `edit_image` success/invalid image |

## Running Individual Packages

```bash
go test ./internal/security/...
go test ./internal/gemini/...
go test ./internal/tools/...
go test ./internal/server/...
go test ./internal/policy/...
go test ./internal/config/...
```
