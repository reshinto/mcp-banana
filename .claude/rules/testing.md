## Testing Rules

### Test Runner

**Unit / Integration**: `go test -race -coverprofile=coverage.out ./...`
**E2E**: None

### Coverage Thresholds

Minimum thresholds (lines/functions/branches/statements): **80/75/80/80**

Coverage checks run as part of the CI sequence. A build that drops below any threshold is treated as a failing build.

### Test Location Conventions

- Test files use the `_test.go` suffix and are co-located with the source they test.
- One test file per source file — do not group unrelated tests into a single file.
- Test function parameter: `test *testing.T` (not `t *testing.T`).

### What to Test

- Every new feature or algorithm needs at minimum: correctness tests + one integration/pipeline test.
- Edge cases: empty input, single element, maximum bounds, invalid input.
- Do not test implementation details — test observable behavior and outputs.
- Mocks: use the `GeminiService` interface for mocking Gemini API calls. Prefer real implementations elsewhere.

### Security Test Requirements

- Every handler that touches user input must have a test verifying no secrets appear in the response.
- Tests for `internal/gemini/` must verify that `genai.APIError` is safely unwrapped and never propagates raw.
- Security boundary tests live in `internal/security/` and must cover sanitization of both input and output.

### CI Sequence

Run in this exact order and fix each step before proceeding to the next:

```
golangci-lint run
gofmt -w .
go vet ./...
go test -race -coverprofile=coverage.out ./...
```

### Quality Gate

Do not run lint/typecheck/format/test mid-implementation. Run the full CI sequence only at the final quality gate before committing. Fix iteratively until all steps are green.
