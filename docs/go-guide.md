# Go Language Guide

This guide explains the Go language concepts used in mcp-banana, with examples drawn directly from the codebase. It is written for developers who are new to Go.

## 1. Packages and Imports

Every Go file begins with a `package` declaration. Files in the same directory share a package. The `internal/` prefix prevents packages inside it from being imported by code outside the module.

```go
// internal/config/config.go
package config
```

Imports are grouped: standard library first, then third-party, then internal packages. A blank line separates each group.

```go
import (
    // standard library
    "context"
    "fmt"
    "os"

    // third-party
    "github.com/mark3labs/mcp-go/mcp"

    // internal
    "github.com/reshinto/mcp-banana/internal/config"
)
```

When two packages share the same base name, an import alias resolves the conflict:

```go
// cmd/mcp-banana/main.go
import (
    "github.com/mark3labs/mcp-go/server"
    internalserver "github.com/reshinto/mcp-banana/internal/server"
)
```

`internalserver.NewMCPServer()` refers to the internal package; `server.ServeStdio()` refers to mcp-go.

## 2. Structs and Struct Tags

A struct groups related fields. Backtick annotations (struct tags) control JSON serialization:

```go
// internal/gemini/service.go
type ImageResult struct {
    ImageBase64    string `json:"image_base64"`
    MIMEType       string `json:"mime_type"`
    ModelUsed      string `json:"model_used"`
    GenerationTime int64  `json:"generation_time_ms"`
}
```

`json:"image_base64"` serializes the field as `"image_base64"` in JSON output (snake_case rather than Go's PascalCase).

When a struct is only safe to expose externally with a subset of fields, a separate type is defined:

```go
// internal/gemini/registry.go
type SafeModelInfo struct {
    Alias          string   `json:"id"`
    Description    string   `json:"description"`
    Capabilities   []string `json:"capabilities"`
    TypicalLatency string   `json:"typical_latency"`
    BestFor        string   `json:"best_for"`
}
```

`ModelInfo` contains `GeminiID` (internal-only). `SafeModelInfo` deliberately omits it. All external responses use `SafeModelInfo`.

## 3. Functions and Methods

Functions can return multiple values. The convention is `(result, error)`:

```go
// internal/config/config.go
func Load() (*Config, error) {
    // success: non-nil config, nil error
    return &Config{...}, nil
    // failure: nil config, non-nil error
    return nil, errors.New("GEMINI_API_KEY is required")
}
```

A method is a function with a receiver. A pointer receiver (`*middleware`) allows the method to read the struct's state and avoids copying:

```go
// internal/server/middleware.go
func (mw *middleware) WrapHTTP(next http.Handler) http.Handler {
    // mw is a pointer to the middleware struct
}
```

## 4. Error Handling

Go does not use exceptions. Functions return errors as values. The caller checks the error immediately:

```go
// cmd/mcp-banana/main.go
serverConfig, loadError := config.Load()
if loadError != nil {
    fmt.Fprintf(os.Stderr, "failed to load config: %s\n", loadError)
    os.Exit(1)
}
```

`errors.New` creates a simple error with a fixed message. `fmt.Errorf` creates a formatted error; the `%w` verb wraps an existing error so callers can unwrap it with `errors.As` or `errors.Is`:

```go
// internal/config/config.go
return 0, fmt.Errorf("%s must be a valid integer, got %q", name, rawValue)
return nil, fmt.Errorf("failed to create genai client: %w", clientError)
```

`errors.As` unwraps an error chain to check whether any wrapped error is a specific type. This is the safe error-mapping boundary in `internal/gemini/errors.go`:

```go
// internal/gemini/errors.go
var apiErrorPointer *genai.APIError
if errors.As(inputError, &apiErrorPointer) {
    safeCode := mapHTTPStatus(apiErrorPointer.Code)
    return safeCode, safeMessages[safeCode]
}
```

Raw `genai.APIError` text is discarded. Only the HTTP status code is used to select a safe message from a predefined allowlist.

## 5. Interfaces and Dependency Injection

An interface defines a set of method signatures. Any type that implements all the methods satisfies the interface — no explicit declaration needed.

```go
// internal/gemini/service.go
type GeminiService interface {
    GenerateImage(requestContext context.Context, modelAlias string, prompt string, options GenerateOptions) (*ImageResult, error)
    EditImage(requestContext context.Context, modelAlias string, imageData []byte, mimeType string, instructions string) (*ImageResult, error)
}
```

`*Client` satisfies `GeminiService` because it has both methods. Tests use a mock struct that also satisfies the interface, allowing handlers to run without a real API key:

```go
// internal/tools/tools_test.go
type mockGeminiService struct {
    generateResult *gemini.ImageResult
    generateError  error
}

func (mock *mockGeminiService) GenerateImage(
    requestContext context.Context, modelAlias string, prompt string, options gemini.GenerateOptions,
) (*gemini.ImageResult, error) {
    return mock.generateResult, mock.generateError
}
```

The handler factory accepts `gemini.GeminiService` (the interface), not `*gemini.Client` (the concrete type). This is dependency injection:

```go
// internal/tools/generate.go
func NewGenerateImageHandler(service gemini.GeminiService, clientCache *gemini.ClientCache, maxImageBytes int) func(...) {
    // service is *Client in production, *mockGeminiService in tests
}
```

## 6. Closures

A closure is a function that captures variables from its enclosing scope. Handler factories return closures that capture their dependencies:

```go
// internal/tools/generate.go
func NewGenerateImageHandler(service gemini.GeminiService, clientCache *gemini.ClientCache, maxImageBytes int) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    return func(requestContext context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
        // 'service', 'clientCache', and 'maxImageBytes' are captured from the outer function
        result, err := service.GenerateImage(requestContext, modelAlias, prompt, gemini.GenerateOptions{
            AspectRatio: aspectRatio,
        })
        // ...
    }
}
```

Each call to `NewGenerateImageHandler` produces a fresh handler with its own captured state.

## 7. context.Context

`context.Context` carries deadlines, cancellation signals, and request-scoped values. It is always the first parameter, and named descriptively in this codebase (`requestContext`, `startupContext`, `timeoutContext`, `shutdownContext`):

```go
// internal/gemini/client.go
func (client *Client) GenerateImage(requestContext context.Context, ...) (*ImageResult, error) {
    timeoutContext, cancel := context.WithTimeout(requestContext, time.Duration(client.timeoutSecs)*time.Second)
    defer cancel()
    resp, generateError := client.generator.GenerateContent(timeoutContext, ...)
}
```

`context.WithTimeout` returns a derived context that automatically cancels after the duration. The `cancel` function must always be called (via `defer`) to release resources even if the timeout fires first.

### Storing Values in Context

`context.WithValue` stores a typed key-value pair in the context. A private key type prevents collisions with other packages:

```go
// internal/gemini/context.go
type contextKey string
const apiKeyContextKey contextKey = "gemini-api-key"

func WithAPIKey(ctx context.Context, apiKey string) context.Context {
    return context.WithValue(ctx, apiKeyContextKey, apiKey)
}

func APIKeyFromContext(ctx context.Context) string {
    value, _ := ctx.Value(apiKeyContextKey).(string)
    return value
}
```

The middleware stores the per-request Gemini API key (from the `X-Gemini-API-Key` header) in the context. Downstream tool handlers retrieve it to select the correct Gemini client.

## 8. Goroutines and Channels

A goroutine is a lightweight concurrent function started with `go`. Channels pass values between goroutines safely.

The HTTP shutdown logic runs in a goroutine that waits for a signal:

```go
// cmd/mcp-banana/main.go
stopChan := make(chan os.Signal, 1)
signal.Notify(stopChan, syscall.SIGTERM, syscall.SIGINT)

go func() {
    sig := <-stopChan  // block until a signal arrives
    shutdownContext, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
    defer shutdownCancel()
    httpServer.Shutdown(shutdownContext)
}()
```

### Channel-Based Semaphore Pattern

A buffered channel acts as a semaphore — it limits how many goroutines can proceed past a point simultaneously. The pro-model concurrency limit uses this pattern:

```go
// internal/gemini/client.go
// created with capacity == proConcurrency:
client.proSemaphore = make(chan struct{}, proConcurrency)

// acquiring a slot before the API call:
select {
case client.proSemaphore <- struct{}{}:
    defer func() { <-client.proSemaphore }()
case <-requestContext.Done():
    return nil, fmt.Errorf("%s: %s", ErrServerError, "request cancelled while waiting for pro model slot")
}
```

Sending to a full channel blocks. `select` with `requestContext.Done()` as the second case means the request fails fast if the caller cancels rather than queuing indefinitely. The `defer` releases the slot when the function returns.

The global concurrency semaphore in middleware uses the same pattern, but initialized with tokens pre-loaded (receives to acquire, sends to release):

```go
// internal/server/middleware.go
semaphore := make(chan struct{}, cfg.GlobalConcurrency)
for slotIndex := 0; slotIndex < cfg.GlobalConcurrency; slotIndex++ {
    semaphore <- struct{}{}
}

// acquiring:
select {
case slot := <-mw.semaphore:
    defer func() { mw.semaphore <- slot }()
case <-time.After(concurrencyTimeout):
    writeJSONError(writer, http.StatusServiceUnavailable, "server_busy")
    return
}
```

## 9. Defer

`defer` schedules a call to run when the surrounding function returns, regardless of how it returns. Common uses:

```go
// cancelling a context:
defer cancel()

// releasing a semaphore slot:
defer func() { <-client.proSemaphore }()

// panic recovery in middleware:
defer func() {
    if recovered := recover(); recovered != nil {
        mw.logger.Error("panic recovered in HTTP handler", "recovered", recovered)
        writeJSONError(writer, http.StatusInternalServerError, "server_error")
    }
}()
```

Multiple defers in the same function run in last-in, first-out order.

## 10. sync.RWMutex

`sync.RWMutex` allows many concurrent readers or one exclusive writer. It is more efficient than a plain `sync.Mutex` for read-heavy workloads.

The registered-secrets slice in `internal/security/sanitize.go` is protected this way:

```go
var (
    secretsMutex      sync.RWMutex
    registeredSecrets []string
)

func RegisterSecret(secret string) {
    secretsMutex.Lock()          // exclusive write lock
    defer secretsMutex.Unlock()
    registeredSecrets = append(registeredSecrets, secret)
}

func SanitizeString(input string) string {
    secretsMutex.RLock()         // shared read lock — multiple callers OK
    secrets := make([]string, len(registeredSecrets))
    copy(secrets, registeredSecrets)
    secretsMutex.RUnlock()       // released before the loop to minimize lock hold time
    // ...
}
```

The same pattern is used in `internal/gemini/cache.go` (`ClientCache`) and `internal/oauth/store.go` (`Store`).

The `ClientCache` uses a double-check pattern to avoid creating duplicate clients under concurrent load:

```go
// internal/gemini/cache.go
cache.mutex.RLock()
existing, found := cache.clients[apiKey]
cache.mutex.RUnlock()
if found {
    return existing, nil  // fast path
}

// slow path: create under write lock
cache.mutex.Lock()
defer cache.mutex.Unlock()
if existing, found = cache.clients[apiKey]; found {
    return existing, nil  // re-check after acquiring write lock
}
// create and store new client
```

## 11. Maps and Slices

Maps are used for O(1) membership checks. A `map[string]struct{}` uses no memory for values:

```go
// internal/security/validate.go
var validAspectRatios = map[string]struct{}{
    "1:1": {}, "16:9": {}, "9:16": {}, "4:3": {}, "3:4": {},
}
if _, ok := validAspectRatios[ratio]; !ok {
    return fmt.Errorf("invalid aspect ratio %q: must be one of 1:1, 16:9, 9:16, 4:3, 3:4", ratio)
}
```

Pre-allocating a slice with `make` avoids repeated reallocations when the size is known:

```go
// internal/gemini/registry.go
models := make([]SafeModelInfo, 0, len(registry))
for _, model := range registry {
    models = append(models, SafeModelInfo{...})
}
```

## 12. String and Encoding

`utf8.RuneCountInString` counts Unicode code points (runes), not bytes, which correctly limits prompt length for multi-byte characters:

```go
// internal/security/validate.go
if utf8.RuneCountInString(prompt) > maxPromptRunes {
    return fmt.Errorf("prompt exceeds maximum length of %d runes", maxPromptRunes)
}
```

`base64.StdEncoding.DecodeString` decodes base64 to bytes. `base64.StdEncoding.EncodeToString` does the reverse:

```go
// input validation:
decoded, err := base64.StdEncoding.DecodeString(encoded)

// output encoding in client.go:
encoded := base64.StdEncoding.EncodeToString(part.InlineData.Data)
```

`json.Marshal` serializes a Go value to JSON bytes:

```go
// internal/tools/generate.go
jsonBytes, _ := json.Marshal(result)
return mcp.NewToolResultText(string(jsonBytes)), nil
```

`regexp.MustCompile` compiles a regular expression at package initialization. `MustCompile` panics on an invalid pattern, catching bugs at startup:

```go
// internal/security/sanitize.go
var geminiAPIKeyPattern = regexp.MustCompile(`AIza[0-9A-Za-z_-]{35}`)
```

## 13. net/http — Handlers, Routing, and TLS

`http.Handler` is an interface with one method: `ServeHTTP(ResponseWriter, *Request)`. `http.HandlerFunc` adapts a plain function to that interface.

`http.ServeMux` routes requests by path:

```go
// internal/server/server.go
mux := http.NewServeMux()
mux.HandleFunc("/healthz", func(writer http.ResponseWriter, _ *http.Request) {
    writer.Header().Set("Content-Type", "application/json")
    writer.WriteHeader(http.StatusOK)
    writer.Write([]byte(`{"status":"ok"}`))
})
mux.Handle("/mcp", streamableHandler)
```

`http.Server` adds timeouts and graceful shutdown:

```go
// cmd/mcp-banana/main.go
httpServer := &http.Server{
    Handler:     handler,
    ReadTimeout: 30 * time.Second,
    IdleTimeout: 120 * time.Second,
}
```

When `MCP_TLS_CERT_FILE` and `MCP_TLS_KEY_FILE` are both set, the server calls `ServeTLS` instead of `Serve`:

```go
// cmd/mcp-banana/main.go
if serverConfig.TLSCertFile != "" && serverConfig.TLSKeyFile != "" {
    serveError = httpServer.ServeTLS(listener, serverConfig.TLSCertFile, serverConfig.TLSKeyFile)
} else {
    serveError = httpServer.Serve(listener)
}
```

`ServeTLS` loads the certificate and key, performs the TLS handshake per connection, and then handles HTTP as usual. The rest of the application — middleware, handlers, MCP dispatch — is identical for HTTPS and plain HTTP.

## 14. embed and html/template

The `embed` package bundles files into the compiled binary at build time. The OAuth login page uses this:

```go
// internal/oauth/handler.go
import "embed"

//go:embed login.html
var loginPageFS embed.FS
```

The `//go:embed` directive is a build-time instruction. Go reads `login.html` from the filesystem and stores its contents in `loginPageFS`. The deployed binary contains the HTML — no separate file deployment is required.

`html/template` renders server-side HTML. Unlike `text/template`, it automatically escapes values inserted into HTML to prevent cross-site scripting:

```go
// internal/oauth/handler.go
loginTemplate := template.Must(template.ParseFS(loginPageFS, "login.html"))

renderError := loginTemplate.Execute(writer, map[string]interface{}{
    "Providers": providerLinks,
})
```

`template.Must` panics if the template fails to parse, catching template syntax errors at startup rather than at request time.

## 15. crypto/rand

`math/rand` generates pseudorandom numbers from a deterministic seed — unsuitable for security tokens. `crypto/rand` reads from the OS entropy source and is safe for generating unpredictable values such as OAuth state parameters and authorization codes:

```go
// internal/oauth/pkce.go
var GenerateRandomToken = func(byteLength int) (string, error) {
    randomBytes := make([]byte, byteLength)
    _, readError := rand.Read(randomBytes)
    if readError != nil {
        return "", readError
    }
    return hex.EncodeToString(randomBytes), nil
}
```

`rand.Read` fills the slice with random bytes. It returns an error only if the OS entropy source is unavailable.

`GenerateRandomToken` is a package-level variable (not a function) so tests can inject a failing implementation to exercise error paths without requiring special OS conditions.

## 16. Package-Level Variables for Dependency Injection in Tests

`cmd/mcp-banana/main.go` uses package-level variables for functions that are difficult to replace in tests (SDK calls, OS exit):

```go
// cmd/mcp-banana/main.go
var osExit = os.Exit

var clientFactory = func(ctx context.Context, apiKey string, timeoutSecs int, proConcurrency int) (*gemini.Client, error) {
    return gemini.NewClient(ctx, apiKey, timeoutSecs, proConcurrency)
}

var registryValidator = func() error {
    return gemini.ValidateRegistryAtStartup()
}

var stdioServe = func(mcpServer *server.MCPServer) error {
    return server.ServeStdio(mcpServer)
}
```

Tests replace these variables before calling `run(...)`:

```go
osExit = func(code int) { /* capture exit code */ }
clientFactory = func(...) (*gemini.Client, error) { return nil, errors.New("injected failure") }
```

This avoids the need for an interface for every external dependency and keeps `cmd/` thin.

## 17. Go Glossary

| Abbreviation | Full Name | Notes |
|---|---|---|
| `err` | error | Error returned by the previous operation. Checked immediately after assignment. |
| `ctx` | context.Context | Carries deadlines, cancellation, and request-scoped values across API boundaries. |
| `req` | request | The incoming tool call or HTTP request. |
| `resp` | response | The HTTP or API response received from an upstream service. |
| `cfg` | config | The parsed application configuration loaded from environment variables at startup. |
| `srv` | server | The HTTP server instance (`*http.Server`) or MCP server (`*mcpserver.MCPServer`). |
| `test` | *testing.T | The Go test runner. Named `test`, not `t`, per the no-single-character-variable rule. |
