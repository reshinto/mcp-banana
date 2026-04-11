# Go Language Guide

This guide explains the Go language concepts used in mcp-banana, with examples drawn directly from the codebase. It is written for developers who are new to Go.

---

## 1. Packages and Imports

Every Go file begins with a `package` declaration. Files in the same directory share a package name and can access each other's unexported identifiers. The `internal/` path prefix is enforced by the Go toolchain: code outside the module root cannot import anything under `internal/`.

```go
// internal/config/config.go
package config
```

An import alias renames a package locally to avoid a name collision. Here, the external `server` package is aliased to `mcpserver` so it does not shadow the local `server` package:

```go
// cmd/mcp-banana/main.go
import (
    // standard library
    "context"
    "fmt"
    "os"

    // third-party
    "github.com/mark3labs/mcp-go/server"

    // internal — aliased to avoid collision with the external "server"
    internalserver "github.com/reshinto/mcp-banana/internal/server"
)
```

Why three groups with blank lines between them? `gofmt` enforces a canonical import style; separate groups signal to readers exactly where a dependency comes from at a glance.

---

## 2. Variables and Types

`var` declares a package-level variable. `const` declares a compile-time constant. A type declaration creates a new named type.

```go
// cmd/mcp-banana/main.go
var version = "dev"           // set at build time via -ldflags

// internal/server/middleware.go
const (
    maxBodyBytes    = 15 * 1024 * 1024
    concurrencyTimeout = 5 * time.Second
)
```

`strconv.Atoi` converts a string to an `int`. The second return value is an error (see Section 5):

```go
// internal/config/config.go
parsed, parseError := strconv.Atoi(rawValue)
```

---

## 3. Structs and Struct Tags

A struct groups related fields. Struct tags (backtick annotations) tell `encoding/json` how to name each field in JSON output. The `SafeModelInfo` type deliberately omits the internal `GeminiID` field — the struct shape is the security boundary.

```go
// internal/gemini/registry.go

// ModelInfo is INTERNAL — never expose to clients.
type ModelInfo struct {
    Alias          string
    GeminiID       string // INTERNAL ONLY
    Description    string
    Capabilities   []string
    TypicalLatency string
    BestFor        string
}

// SafeModelInfo contains only the fields safe to expose to Claude Code.
type SafeModelInfo struct {
    Alias          string   `json:"id"`
    Description    string   `json:"description"`
    Capabilities   []string `json:"capabilities"`
    TypicalLatency string   `json:"typical_latency"`
    BestFor        string   `json:"best_for"`
}
```

Similarly, `ImageResult` uses JSON tags so `json.Marshal` in the tool handler produces the correct field names:

```go
// internal/gemini/service.go
type ImageResult struct {
    ImageBase64    string `json:"image_base64"`
    MIMEType       string `json:"mime_type"`
    ModelUsed      string `json:"model_used"`
    GenerationTime int64  `json:"generation_time_ms"`
}
```

---

## 4. Functions and Methods

Go functions can return multiple values. The idiomatic pattern is `(result, error)`. A method is a function with a receiver; pointer receivers (`*Client`) let the method mutate the value and avoid copying.

```go
// internal/gemini/client.go
func NewClient(startupContext context.Context, apiKey string, timeoutSecs int, proConcurrency int) (*Client, error) {
    inner, clientError := genaiClientFactory(startupContext, &genai.ClientConfig{
        APIKey:  apiKey,
        Backend: genai.BackendGeminiAPI,
    })
    if clientError != nil {
        return nil, fmt.Errorf("failed to create genai client: %w", clientError)
    }
    return &Client{
        generator:    inner.Models,
        timeoutSecs:  timeoutSecs,
        proSemaphore: make(chan struct{}, proConcurrency),
    }, nil
}

// GenerateImage is a method on *Client.
func (client *Client) GenerateImage(requestContext context.Context, modelAlias string, prompt string, options GenerateOptions) (*ImageResult, error) {
    ...
}
```

Named return values document intent and can be used with a bare `return`, but mcp-banana avoids bare returns for clarity. The `buildGenerateInputs` function uses named returns as documentation only:

```go
// internal/gemini/client.go
func buildGenerateInputs(prompt string, modelID string, aspectRatio string) (modelName string, contents []*genai.Content, config *genai.GenerateContentConfig) {
    modelName = modelID
    ...
    return modelName, contents, config
}
```

---

## 5. Error Handling

Go errors are values. Functions signal failure by returning a non-nil `error`. `errors.New` creates a plain error; `fmt.Errorf` with `%w` wraps an existing error so callers can unwrap it with `errors.As`.

```go
// internal/config/config.go
if rateLimit <= 0 {
    return nil, errors.New("MCP_RATE_LIMIT must be a positive integer")
}

if (cfg.TLSCertFile != "" && cfg.TLSKeyFile == "") || ... {
    return nil, fmt.Errorf("both MCP_TLS_CERT_FILE and MCP_TLS_KEY_FILE must be set together")
}

// Wrapping: the %w verb embeds the original error so errors.As can find it.
return nil, fmt.Errorf("failed to create genai client: %w", clientError)
```

`errors.As` traverses the error chain and type-asserts into a target type. The security boundary in `MapError` uses this to extract the HTTP status from a Gemini SDK error without leaking raw error text:

```go
// internal/gemini/errors.go
var apiErrorPointer *genai.APIError
if errors.As(inputError, &apiErrorPointer) {
    safeCode := mapHTTPStatus(apiErrorPointer.Code)
    return safeCode, safeMessages[safeCode]
}
```

Oversized HTTP body detection uses the same pattern:

```go
// internal/server/middleware.go
var maxBytesError *http.MaxBytesError
if errors.As(readError, &maxBytesError) {
    writeJSONError(writer, http.StatusRequestEntityTooLarge, "request_too_large")
    return
}
```

---

## 6. Interfaces and Dependency Injection

An interface defines a set of method signatures. Any type that implements all methods satisfies the interface — no explicit declaration is required. Accepting an interface instead of a concrete type lets callers inject fakes or mocks in tests.

```go
// internal/gemini/service.go
type GeminiService interface {
    GenerateImage(requestContext context.Context, modelAlias string, prompt string, options GenerateOptions) (*ImageResult, error)
    EditImage(requestContext context.Context, modelAlias string, imageData []byte, mimeType string, instructions string) (*ImageResult, error)
}
```

`*Client` satisfies `GeminiService` because it implements both methods. The tool handler accepts `GeminiService`, not `*Client`, so tests can pass a struct that returns canned responses:

```go
// internal/server/server.go
func NewMCPServer(service gemini.GeminiService, clientCache *gemini.ClientCache, maxImageBytes int) *mcpserver.MCPServer {
    ...
    srv.AddTool(generateImageTool, tools.NewGenerateImageHandler(service, clientCache, maxImageBytes))
}
```

The internal `contentGenerator` interface follows the same pattern for the genai SDK call — `client.go` only depends on the interface, making `GenerateContent` replaceable in tests without a real API key.

---

## 7. Closures

A closure is a function literal that captures variables from its enclosing scope. Handler factories in `internal/tools/` return closures that close over `service`, `clientCache`, and `maxImageBytes`:

```go
// internal/tools/generate.go
func NewGenerateImageHandler(service gemini.GeminiService, clientCache *gemini.ClientCache, maxImageBytes int) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    return func(requestContext context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
        // service, clientCache, and maxImageBytes are captured from the outer scope.
        resolvedService := service
        ...
    }
}
```

The OAuth cleanup goroutine also uses a closure to capture `oauthStore`:

```go
// cmd/mcp-banana/main.go
go func() {
    ticker := time.NewTicker(cleanupInterval)
    defer ticker.Stop()
    for range ticker.C {
        oauthStore.CleanupExpired() // oauthStore captured from outer scope
    }
}()
```

---

## 8. context.Context

`context.Context` carries deadlines, cancellation signals, and request-scoped values through a call chain. Every function that performs I/O accepts a `ctx context.Context` as its first parameter.

`context.WithTimeout` creates a child context that cancels automatically after the duration. The cancel function must always be called (via `defer`) to free resources:

```go
// internal/gemini/client.go
timeoutContext, cancel := context.WithTimeout(requestContext, time.Duration(client.timeoutSecs)*time.Second)
defer cancel()

resp, generateError := client.generator.GenerateContent(timeoutContext, modelName, contents, config)
```

`context.WithValue` attaches a value to a context. A private named type for the key prevents collisions with keys from other packages:

```go
// internal/gemini/context.go
type contextKey string
const apiKeyContextKey contextKey = "gemini-api-key"

func WithAPIKey(ctx context.Context, apiKey string) context.Context {
    return context.WithValue(ctx, apiKeyContextKey, apiKey)
}

func APIKeyFromContext(ctx context.Context) string {
    value, _ := ctx.Value(apiKeyContextKey).(string) // type assertion with ok-idiom
    return value
}
```

The middleware stores the per-request key in the context; the tool handler reads it back. The two packages share no global state — only the context value travels between them.

---

## 9. Goroutines and Channels

The `go` keyword launches a goroutine — a lightweight concurrent function. Channels are typed conduits for communication between goroutines.

**Signal handling** uses a buffered channel so the OS signal is never dropped even if the receiver is not yet waiting:

```go
// cmd/mcp-banana/main.go
stopChan := make(chan os.Signal, 1) // buffered: size 1
signal.Notify(stopChan, syscall.SIGTERM, syscall.SIGINT)

go func() {
    sig := <-stopChan // block until signal arrives
    shutdownContext, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
    defer shutdownCancel()
    httpServer.Shutdown(shutdownContext)
}()
```

**Pre-filled channel semaphore** limits global concurrency. The channel is pre-filled with `struct{}{}` tokens; each request must receive one before proceeding, then returns it via `defer`:

```go
// internal/server/middleware.go
semaphore := make(chan struct{}, cfg.GlobalConcurrency)
for slotIndex := 0; slotIndex < cfg.GlobalConcurrency; slotIndex++ {
    semaphore <- struct{}{}
}

// In the handler:
select {
case slot := <-mw.semaphore:
    defer func() { mw.semaphore <- slot }()
case <-time.After(concurrencyTimeout):
    writeJSONError(writer, http.StatusServiceUnavailable, "server_busy")
    return
}
```

**Empty channel semaphore** for the pro model works the opposite way — sending claims a slot, receiving releases it:

```go
// internal/gemini/client.go
proSemaphore: make(chan struct{}, proConcurrency), // starts empty

// Claim a slot (send), or abort if context is done:
select {
case client.proSemaphore <- struct{}{}:
    defer func() { <-client.proSemaphore }() // release on return
case <-requestContext.Done():
    return nil, fmt.Errorf("%s: %s", ErrServerError, "request cancelled while waiting for pro model slot")
}
```

---

## 10. Defer

`defer` schedules a function call to run when the surrounding function returns, regardless of how it returns (normal return, early return, or panic). Common uses: closing resources, releasing locks, and panic recovery.

```go
// internal/gemini/client.go
timeoutContext, cancel := context.WithTimeout(requestContext, ...)
defer cancel() // always called, even if GenerateContent panics

// internal/server/middleware.go — panic recovery
defer func() {
    if recovered := recover(); recovered != nil {
        mw.logger.Error("panic recovered in HTTP handler", "recovered", recovered)
        writeJSONError(writer, http.StatusInternalServerError, "server_error")
    }
}()

// internal/oauth/store.go — mutex unlock
func (store *Store) RegisterClient(client *Client) {
    store.mutex.Lock()
    defer store.mutex.Unlock()
    store.clients[client.ClientID] = client
}
```

---

## 11. sync.RWMutex

`sync.RWMutex` allows multiple concurrent readers (`RLock`/`RUnlock`) but only one writer (`Lock`/`Unlock`). This is more efficient than a plain `sync.Mutex` for read-heavy workloads.

`ClientCache` uses the **double-check pattern** to avoid creating the same client twice under concurrent load:

```go
// internal/gemini/cache.go
func (cache *ClientCache) GetClient(ctx context.Context, apiKey string) (*Client, error) {
    // Fast path: read lock only.
    cache.mutex.RLock()
    existing, found := cache.clients[apiKey]
    cache.mutex.RUnlock()
    if found {
        return existing, nil
    }

    // Slow path: promote to write lock.
    cache.mutex.Lock()
    defer cache.mutex.Unlock()

    // Re-check: another goroutine may have created the client between our
    // read-unlock and write-lock.
    if existing, found = cache.clients[apiKey]; found {
        return existing, nil
    }

    created, clientError := NewClient(ctx, apiKey, cache.timeoutSecs, cache.proConcurrency)
    ...
    cache.clients[apiKey] = created
    return created, nil
}
```

The secret sanitizer in `internal/security/sanitize.go` follows the same read/write lock split:

```go
// internal/security/sanitize.go
var (
    secretsMutex      sync.RWMutex
    registeredSecrets []string
)

func RegisterSecret(secret string) {
    secretsMutex.Lock()
    defer secretsMutex.Unlock()
    registeredSecrets = append(registeredSecrets, secret)
}

func SanitizeString(input string) string {
    secretsMutex.RLock()
    secrets := make([]string, len(registeredSecrets))
    copy(secrets, registeredSecrets)
    secretsMutex.RUnlock()
    ...
}
```

---

## 12. Maps and Slices

`map[K]V` is Go's hash map. `map[string]struct{}` is the idiomatic set — `struct{}` uses zero bytes. Membership is tested with the two-value form of a map lookup:

```go
// internal/security/validate.go
var validAspectRatios = map[string]struct{}{
    "1:1":  {},
    "16:9": {},
    "9:16": {},
    "4:3":  {},
    "3:4":  {},
}

if _, ok := validAspectRatios[ratio]; !ok {
    return fmt.Errorf("invalid aspect ratio %q", ratio)
}
```

`make([]T, 0, n)` pre-allocates a slice with capacity `n`, avoiding repeated allocations when appending in a loop:

```go
// internal/gemini/registry.go
models := make([]SafeModelInfo, 0, len(registry))
for _, model := range registry {
    models = append(models, SafeModelInfo{...})
}
```

`range` over a map gives `key, value` pairs; `range` over a channel receives values until the channel is closed:

```go
// cmd/mcp-banana/main.go
for range ticker.C { // receive each tick; discard the value
    oauthStore.CleanupExpired()
}
```

---

## 13. Strings and Encoding

`utf8.RuneCountInString` counts Unicode code points (runes), not bytes — essential for multi-byte characters in prompts:

```go
// internal/security/validate.go
if utf8.RuneCountInString(prompt) > maxPromptRunes {
    return fmt.Errorf("prompt exceeds maximum length of %d runes", maxPromptRunes)
}
```

`base64.StdEncoding.DecodeString` / `EncodeToString` convert between raw bytes and base64 strings. Image data travels as base64 over the MCP protocol:

```go
// internal/security/validate.go
decoded, err := base64.StdEncoding.DecodeString(encoded)

// internal/gemini/client.go
encoded := base64.StdEncoding.EncodeToString(part.InlineData.Data)
```

`hex.EncodeToString` converts random bytes to a safe ASCII token:

```go
// internal/oauth/pkce.go
return hex.EncodeToString(randomBytes), nil
```

`regexp.MustCompile` compiles a regular expression at package init time; `MustCompile` panics if the pattern is invalid, catching bugs at startup rather than at runtime:

```go
// internal/security/sanitize.go
var geminiAPIKeyPattern = regexp.MustCompile(`AIza[0-9A-Za-z_-]{35}`)

output = geminiAPIKeyPattern.ReplaceAllString(output, redacted)
```

`json.Marshal` / `json.Unmarshal` serialize and deserialize structs. `json.NewEncoder(w).Encode(v)` streams JSON directly to an `io.Writer`:

```go
// internal/tools/generate.go
jsonBytes, _ := json.Marshal(result)
return mcp.NewToolResultText(string(jsonBytes)), nil

// internal/oauth/handler.go
json.NewEncoder(writer).Encode(map[string]string{
    "error":             "invalid_grant",
    "error_description": "invalid or expired code",
})
```

---

## 14. net/http

`http.Handler` is an interface with a single method: `ServeHTTP(ResponseWriter, *Request)`. `http.HandlerFunc` is an adapter type that lets a plain function satisfy the interface:

```go
// internal/oauth/handler.go
return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
    ...
})
```

`http.ServeMux` routes requests by path prefix. `Handle` registers an `http.Handler`; `HandleFunc` registers a plain function:

```go
// internal/server/server.go
mux := http.NewServeMux()
mux.HandleFunc("/healthz", func(writer http.ResponseWriter, _ *http.Request) {
    writer.WriteHeader(http.StatusOK)
    writer.Write([]byte(`{"status":"ok"}`))
})
mux.Handle("/mcp", streamableHandler)
```

`http.Server` with explicit timeouts prevents slow-client attacks:

```go
// cmd/mcp-banana/main.go
httpServer := &http.Server{
    Handler:     handler,
    ReadTimeout: 30 * time.Second,
    IdleTimeout: 120 * time.Second,
}
```

`ServeTLS` vs `Serve` — both accept a `net.Listener`; `ServeTLS` also loads TLS credentials:

```go
if serverConfig.TLSCertFile != "" && serverConfig.TLSKeyFile != "" {
    serveError = httpServer.ServeTLS(listener, serverConfig.TLSCertFile, serverConfig.TLSKeyFile)
} else {
    serveError = httpServer.Serve(listener)
}
```

`httptest.NewRecorder` / `httptest.NewRequest` create in-memory request/response pairs for handler tests without starting a real server.

---

## 15. embed

The `//go:embed` directive instructs the compiler to bundle files from the filesystem into the binary. `embed.FS` is a read-only virtual filesystem. `template.ParseFS` reads templates from it:

```go
// internal/oauth/handler.go
import "embed"

//go:embed login.html
var loginPageFS embed.FS

loginTemplate := template.Must(template.ParseFS(loginPageFS, "login.html"))
```

Why embed? The deployed binary is a single file (distroless Docker image with no shell). Embedding the HTML file means there is no runtime dependency on the filesystem.

---

## 16. html/template

`html/template` auto-escapes all values injected into HTML, preventing XSS. `template.Must` wraps `ParseFS` and panics if the template cannot be parsed — a startup failure is better than silently serving broken HTML:

```go
// internal/oauth/handler.go
loginTemplate := template.Must(template.ParseFS(loginPageFS, "login.html"))

renderError := loginTemplate.Execute(writer, map[string]interface{}{
    "Providers": providerLinks,
})
```

Values in `{{ .Providers }}` are automatically HTML-escaped. Never use `text/template` for HTML output — it does not escape.

---

## 17. crypto/rand

`crypto/rand.Read` fills a byte slice with cryptographically secure random data from the OS. Always use `crypto/rand` for tokens, nonces, and secrets — never `math/rand`, which is deterministic given the same seed.

```go
// internal/oauth/pkce.go
import "crypto/rand"

var GenerateRandomToken = func(byteLength int) (string, error) {
    randomBytes := make([]byte, byteLength)
    _, readError := rand.Read(randomBytes)
    if readError != nil {
        return "", readError
    }
    return hex.EncodeToString(randomBytes), nil
}
```

`GenerateRandomToken` is a `var` holding a function (not a plain function) so tests can replace it with a version that returns a fixed string or an error.

---

## 18. Package-Level Variables for Testing

Go has no built-in dependency injection framework. A common pattern is to declare dependencies as package-level `var` values that hold functions or interfaces. Tests replace them before calling the code under test, then restore the original via `defer`.

```go
// cmd/mcp-banana/main.go

// osExit is os.Exit, overridden in tests to prevent process termination.
var osExit = os.Exit

// clientFactory creates a Gemini API client. Overridden in tests.
var clientFactory = func(ctx context.Context, apiKey string, timeoutSecs int, proConcurrency int) (*Client, error) {
    return gemini.NewClient(ctx, apiKey, timeoutSecs, proConcurrency)
}

// stdioServe runs the MCP server in stdio mode. Overridden in tests.
var stdioServe = func(mcpServer *server.MCPServer) error {
    return server.ServeStdio(mcpServer)
}
```

In a test:
```go
original := genaiClientFactory
defer func() { genaiClientFactory = original }()
genaiClientFactory = func(_ context.Context, _ *genai.ClientConfig) (*genai.Client, error) {
    return nil, errors.New("simulated factory failure")
}
```

---

## 19. Testing

Test files end in `_test.go` and live next to the source they test. The test function parameter is named `test` (not `t`) per this project's convention.

**Table-driven tests** express many cases compactly:

```go
// internal/gemini/errors_test.go pattern (representative)
func TestMapError_HTTPStatus(test *testing.T) {
    cases := []struct {
        name     string
        status   int
        wantCode string
    }{
        {"bad_request", 400, "content_policy_violation"},
        {"too_many_requests", 429, "quota_exceeded"},
        {"server_error", 500, "generation_failed"},
    }
    for _, tc := range cases {
        test.Run(tc.name, func(test *testing.T) {
            code, _ := MapError(&genai.APIError{Code: tc.status})
            if code != tc.wantCode {
                test.Errorf("got %q, want %q", code, tc.wantCode)
            }
        })
    }
}
```

`test.Setenv` sets an environment variable for the duration of the test and restores it automatically on cleanup — no manual teardown needed.

`test.Cleanup` registers a function to run when the test ends, equivalent to `defer` but usable in helper functions:

```go
// internal/gemini/cache_test.go
test.Cleanup(func() { afterReadMiss = nil })
```

`httptest.NewRequest` + `httptest.NewRecorder` let you test HTTP handlers without a real server or open port:

```go
req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
rec := httptest.NewRecorder()
handler.ServeHTTP(rec, req)
if rec.Code != http.StatusOK { ... }
```

---

## 20. Build Tags and Flags

The `Dockerfile` builds with:

```
CGO_ENABLED=0 go build -trimpath -ldflags="-X main.version=$(git rev-parse --short HEAD)" ./cmd/mcp-banana/
```

- `CGO_ENABLED=0` — disables C bindings so the binary is fully static and runs in the distroless image (no glibc).
- `-trimpath` — strips absolute source paths from the binary, preventing local paths from leaking in stack traces.
- `-ldflags="-X main.version=..."` — sets the `version` package-level variable at link time without modifying source code.

---

## 21. Go Glossary

Standard Go abbreviations used throughout this codebase:

| Abbreviation | Meaning                          | Example                          |
|--------------|----------------------------------|----------------------------------|
| `err`        | error value                      | `if err != nil`                  |
| `ctx`        | `context.Context`                | `ctx context.Context`            |
| `req`        | request (HTTP or MCP)            | `req mcp.CallToolRequest`        |
| `resp`       | response                         | `resp, err := client.Generate()` |
| `cfg`        | config/configuration struct      | `cfg *config.Config`             |
| `srv`        | server                           | `srv := mcpserver.NewMCPServer()`|
| `test`       | `*testing.T` in test functions   | `func TestFoo(test *testing.T)`  |

All other names use full words: `requestContext` not `reqCtx`; `clientError` not `err` when multiple errors are in scope; `goroutineIndex` not `i`.
