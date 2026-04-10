# Go Language Guide

This guide explains the Go language concepts used in mcp-banana, with examples drawn directly from the codebase. It is written for developers who are new to Go.

## 1. Packages and Imports

Every Go file begins with a `package` declaration. Files in the same directory share a package. The `internal/` prefix is a Go convention that prevents packages inside it from being imported by code outside the module.

```go
// internal/config/config.go
package config
```

Imports are grouped: standard library first, then third-party, then internal packages. A blank line separates each group.

```go
import (
    "errors"
    "fmt"
    "os"
    "strconv"
    "strings"
)
```

When two packages have the same base name, an alias resolves the conflict:

```go
// cmd/mcp-banana/main.go
import (
    "github.com/mark3labs/mcp-go/server"
    internalserver "github.com/reshinto/mcp-banana/internal/server"
)
```

Here `internalserver` is used to call `internalserver.NewMCPServer()` while `server.ServeStdio()` refers to the mcp-go package.

## 2. Structs and Types

A struct groups related fields. Struct tags (the backtick annotations) control JSON serialization:

```go
// internal/gemini/service.go
type ImageResult struct {
    ImageBase64    string `json:"image_base64"`
    MIMEType       string `json:"mime_type"`
    ModelUsed      string `json:"model_used"`
    GenerationTime int64  `json:"generation_time_ms"`
}
```

`json:"image_base64"` means the field serializes to `"image_base64"` in JSON output (snake_case rather than Go's PascalCase).

The `Config` struct holds all server settings loaded from environment variables:

```go
// internal/config/config.go
type Config struct {
    GeminiAPIKey       string
    AuthToken          string
    LogLevel           string
    RateLimit          int
    GlobalConcurrency  int
    ProConcurrency     int
    MaxImageBytes      int
    RequestTimeoutSecs int
}
```

## 3. Functions and Methods

Functions can return multiple values. The convention is to return `(result, error)`:

```go
// internal/config/config.go
func Load() (*Config, error) {
    // ...
    return &Config{...}, nil  // success: non-nil config, nil error
    // return nil, errors.New("...")  // failure: nil config, non-nil error
}
```

A method is a function with a receiver. A pointer receiver (`*Client`) means the method can modify the receiver's state and avoids copying a large struct:

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

`errors.New` creates a simple error with a fixed message. `fmt.Errorf` creates a formatted error; the `%w` verb wraps an existing error so callers can unwrap it:

```go
// internal/config/config.go
return nil, errors.New("GEMINI_API_KEY is required")
return 0, fmt.Errorf("%s must be a valid integer, got %q", name, rawValue)
return nil, fmt.Errorf("failed to create genai client: %w", err)
```

`errors.As` unwraps an error chain to check whether any wrapped error is a specific type:

```go
// internal/gemini/errors.go
var apiErrorPointer *genai.APIError
if errors.As(inputError, &apiErrorPointer) {
    safeCode := mapHTTPStatus(apiErrorPointer.Code)
    return safeCode, safeMessages[safeCode]
}
```

## 5. Interfaces and Dependency Injection

An interface defines a set of method signatures. Any type that implements all the methods satisfies the interface — no explicit declaration is needed.

```go
// internal/gemini/service.go
type GeminiService interface {
    GenerateImage(requestContext context.Context, modelAlias string, prompt string, options GenerateOptions) (*ImageResult, error)
    EditImage(requestContext context.Context, modelAlias string, imageData []byte, mimeType string, instructions string) (*ImageResult, error)
}
```

`*Client` satisfies `GeminiService` because it has both methods. Tests use a mock struct that also satisfies the interface, allowing handlers to be tested without a real Gemini API key:

```go
// internal/tools/tools_test.go
type mockGeminiService struct {
    generateResult *gemini.ImageResult
    generateError  error
}

func (mock *mockGeminiService) GenerateImage(...) (*gemini.ImageResult, error) {
    return mock.generateResult, mock.generateError
}
```

The handler factory accepts `gemini.GeminiService` (the interface), not `*gemini.Client` (the concrete type). This is dependency injection:

```go
// internal/tools/generate.go
func NewGenerateImageHandler(service gemini.GeminiService, maxImageBytes int) func(...) {
    // service could be *Client in production or *mockGeminiService in tests
}
```

## 6. Closures and Higher-Order Functions

A closure is a function that captures variables from its enclosing scope. Handler factories return closures that capture their dependencies:

```go
// internal/tools/generate.go
func NewGenerateImageHandler(service gemini.GeminiService, maxImageBytes int) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    return func(requestContext context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
        // 'service' and 'maxImageBytes' are captured from the outer function
        result, err := service.GenerateImage(requestContext, modelAlias, prompt, gemini.GenerateOptions{
            AspectRatio: aspectRatio,
        })
        // ...
    }
}
```

The returned function closes over `service` and `maxImageBytes`. Each call to `NewGenerateImageHandler` produces a fresh handler with its own captured state.

## 7. Context

`context.Context` carries deadlines, cancellation signals, and request-scoped values. It is always the first parameter and always named descriptively (here `requestContext`, `startupContext`, `timeoutContext`, `shutdownContext`):

```go
// internal/gemini/client.go
func (client *Client) GenerateImage(requestContext context.Context, ...) (*ImageResult, error) {
    timeoutContext, cancel := context.WithTimeout(requestContext, time.Duration(client.timeoutSecs)*time.Second)
    defer cancel()
    resp, err := client.inner.Models.GenerateContent(timeoutContext, ...)
}
```

`context.WithTimeout` returns a derived context that automatically cancels after the specified duration. The `cancel` function must always be called (via `defer`) to release resources even if the timeout fires first.

## 8. Goroutines and Channels

A goroutine is a lightweight concurrent function started with `go`. Channels pass values between goroutines safely.

The HTTP server runs its shutdown logic in a goroutine that waits for a signal:

```go
// cmd/mcp-banana/main.go
stopChan := make(chan os.Signal, 1)
signal.Notify(stopChan, syscall.SIGTERM, syscall.SIGINT)

go func() {
    sig := <-stopChan   // block until a signal arrives
    shutdownContext, shutdownCancel := context.WithTimeout(context.Background(), 120*time.Second)
    defer shutdownCancel()
    httpServer.Shutdown(shutdownContext)
}()
```

A buffered channel acts as a semaphore. The pro-model concurrency limit works by attempting to send to a full channel:

```go
// internal/gemini/client.go
client.proSemaphore: make(chan struct{}, proConcurrency),

// acquiring the semaphore slot:
select {
case client.proSemaphore <- struct{}{}:
    defer func() { <-client.proSemaphore }()
case <-requestContext.Done():
    return nil, fmt.Errorf(...)
}
```

`select` blocks until one of its cases can proceed. If both are ready simultaneously, Go picks one at random.

## 9. Defer

`defer` schedules a function call to run when the surrounding function returns, regardless of how it returns (normal, panic, early return). Common uses in this codebase:

```go
// cancelling a context:
defer cancel()

// releasing a semaphore slot:
defer func() { <-client.proSemaphore }()

// panic recovery in middleware:
defer func() {
    if recovered := recover(); recovered != nil {
        writeJSONError(writer, http.StatusInternalServerError, "server_error")
    }
}()
```

Multiple defers stack and run in last-in, first-out order.

## 10. Sync Primitives

`sync.RWMutex` allows many concurrent readers or one exclusive writer. It protects the registered-secrets slice in the sanitizer:

```go
// internal/security/sanitize.go
var (
    secretsMutex      sync.RWMutex
    registeredSecrets []string
)

func RegisterSecret(secret string) {
    secretsMutex.Lock()         // exclusive write lock
    defer secretsMutex.Unlock()
    registeredSecrets = append(registeredSecrets, secret)
}

func SanitizeString(input string) string {
    secretsMutex.RLock()        // shared read lock
    secrets := make([]string, len(registeredSecrets))
    copy(secrets, registeredSecrets)
    secretsMutex.RUnlock()
    // ...
}
```

The read lock (`RLock`) is released before the loop runs so that sanitization does not hold the lock while doing string replacement work.

## 11. Maps and Slices

Maps are used for O(1) membership checks. The zero value of a missing key is `false` for `bool` maps and the zero struct for `struct{}` maps:

```go
// internal/config/config.go
validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
if !validLevels[logLevel] {
    return nil, errors.New("MCP_LOG_LEVEL must be one of: debug, info, warn, error")
}

// internal/security/validate.go — struct{} uses no memory for the value
var validAspectRatios = map[string]struct{}{
    "1:1": {}, "16:9": {}, "9:16": {}, "4:3": {}, "3:4": {},
}
if _, ok := validAspectRatios[ratio]; !ok {
    return fmt.Errorf("invalid aspect ratio ...")
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

`utf8.RuneCountInString` counts Unicode code points (runes), not bytes. This correctly limits prompt length for multi-byte characters:

```go
// internal/security/validate.go
if utf8.RuneCountInString(prompt) > maxPromptRunes {
    return fmt.Errorf("prompt exceeds maximum length of %d runes", maxPromptRunes)
}
```

`base64.StdEncoding.DecodeString` decodes base64 to bytes. `base64.StdEncoding.EncodeToString` does the reverse:

```go
// internal/security/validate.go
decoded, err := base64.StdEncoding.DecodeString(encoded)

// internal/gemini/client.go
encoded := base64.StdEncoding.EncodeToString(part.InlineData.Data)
```

`json.Marshal` serializes a Go value to JSON bytes. The result is cast to `string` before being returned in the MCP response:

```go
// internal/tools/generate.go
jsonBytes, marshalErr := json.Marshal(result)
return mcp.NewToolResultText(string(jsonBytes)), nil
```

`regexp.MustCompile` compiles a regular expression at package initialization. `MustCompile` panics on a bad pattern, catching bugs at startup rather than at runtime:

```go
// internal/security/sanitize.go
var geminiAPIKeyPattern = regexp.MustCompile(`AIza[0-9A-Za-z_-]{35}`)
```

## 13. HTTP Server

`http.Handler` is an interface with a single method: `ServeHTTP(ResponseWriter, *Request)`. `http.HandlerFunc` adapts a plain function to that interface:

```go
// internal/server/server.go
mux.HandleFunc("/healthz", func(writer http.ResponseWriter, _ *http.Request) {
    writer.Header().Set("Content-Type", "application/json")
    writer.WriteHeader(http.StatusOK)
    writer.Write([]byte(`{"status":"ok"}`))
})
```

`http.ServeMux` routes requests to handlers by path prefix. An `http.Server` struct adds timeouts:

```go
// cmd/mcp-banana/main.go
httpServer := &http.Server{
    Addr:        *address,
    Handler:     handler,
    ReadTimeout: 30 * time.Second,
    IdleTimeout: 120 * time.Second,
}
```

Graceful shutdown drains in-flight requests before exiting:

```go
httpServer.Shutdown(shutdownContext)
```

## 14. Flags and Configuration

The `flag` package parses command-line arguments:

```go
// cmd/mcp-banana/main.go
transport := flag.String("transport", "stdio", "Transport mode: stdio or http")
address   := flag.String("addr", "0.0.0.0:8847", "Address to listen on (HTTP mode only)")
flag.Parse()

// *transport dereferences the pointer to get the string value
if *transport == "http" { ... }
```

Build-time version injection uses `-ldflags`:

```go
// cmd/mcp-banana/main.go
var version = "dev"  // overridden at build time

// Makefile:
// go build -ldflags="-X main.version=1.0.0" ...
```

## 15. Testing

Test files end in `_test.go`. Test functions take `*testing.T` as their sole argument (named `test` in this codebase, not `t`, per the no-single-char-variable rule):

```go
// internal/tools/tools_test.go
func TestGenerateImageHandler_Success(test *testing.T) {
    svc := &mockGeminiService{generateResult: stubImageResult()}
    handler := NewGenerateImageHandler(svc, 10*1024*1024)

    result, err := handler(context.Background(), req)
    if err != nil {
        test.Fatalf("expected no Go error, got: %v", err)
    }
}
```

`test.Fatalf` stops the current test immediately. `test.Errorf` records a failure but continues running. `test.Helper()` marks a function as a test helper so that failure lines point to the caller:

```go
func extractTextContent(test *testing.T, result *mcp.CallToolResult) string {
    test.Helper()
    // ...
}
```

`test.Setenv` sets an environment variable for the duration of the test and automatically restores the original value via `test.Cleanup`:

```go
// internal/config/config_test.go
test.Setenv("GEMINI_API_KEY", "test-key")
// restored automatically when the test ends
```

The race detector is enabled with `-race`:

```go
// Makefile
go test -coverprofile=coverage.out -race ./... -v
```

## 16. OAuth Implementation Concepts

This section explains the Go standard library features used in the OAuth 2.1 implementation.

### `embed` — Embedding Static Files into the Binary

The `embed` package lets you bundle files (such as the OAuth login page `login.html`) directly into the compiled binary. This means the server does not need to read files from disk at runtime.

```go
import _ "embed"

//go:embed login.html
var loginHTML []byte
```

The `//go:embed` directive is a build-time instruction. At compile time, Go reads `login.html` from the filesystem and stores its contents in `loginHTML`. The deployed binary contains the HTML — no separate file deployment required.

### `crypto/rand` — Cryptographically Secure Random Numbers

`math/rand` generates pseudorandom numbers from a deterministic seed — unsuitable for security tokens. `crypto/rand` reads from the operating system's entropy source (e.g., `/dev/urandom` on Linux) and is safe for generating unpredictable values such as OAuth `state` parameters and authorization codes.

```go
import "crypto/rand"

// Generate 32 random bytes and encode as hex
tokenBytes := make([]byte, 32)
if _, err := crypto/rand.Read(tokenBytes); err != nil {
    return "", fmt.Errorf("failed to generate token: %w", err)
}
token := hex.EncodeToString(tokenBytes)
```

`crypto/rand.Read` fills the slice with random bytes. It returns an error only if the OS entropy source is unavailable, which is rare but must be handled.

### `sync.RWMutex` — Reader-Writer Mutual Exclusion

The OAuth token store holds active tokens in a map that multiple goroutines read and write concurrently. `sync.RWMutex` allows many concurrent readers or exactly one exclusive writer, which is more efficient than a plain `sync.Mutex` for read-heavy workloads.

```go
// internal/oauth/store.go
type TokenStore struct {
    tokensMutex sync.RWMutex
    tokens      map[string]*Token
}

func (store *TokenStore) Get(tokenID string) (*Token, bool) {
    store.tokensMutex.RLock()         // shared read lock — multiple readers OK
    defer store.tokensMutex.RUnlock()
    token, found := store.tokens[tokenID]
    return token, found
}

func (store *TokenStore) Put(tokenID string, token *Token) {
    store.tokensMutex.Lock()          // exclusive write lock — blocks all readers
    defer store.tokensMutex.Unlock()
    store.tokens[tokenID] = token
}
```

This is the same pattern used by `internal/security/sanitize.go` for the registered-secrets slice (see section 10).

### `net/http` `ListenAndServeTLS` — Built-in TLS Support

The standard library's `http.Server` supports TLS natively. When `MCP_TLS_CERT_FILE` and `MCP_TLS_KEY_FILE` are set, the server calls `ListenAndServeTLS` instead of `ListenAndServe`:

```go
// cmd/mcp-banana/main.go
if serverConfig.TLSCertFile != "" && serverConfig.TLSKeyFile != "" {
    startError = httpServer.ListenAndServeTLS(serverConfig.TLSCertFile, serverConfig.TLSKeyFile)
} else {
    startError = httpServer.ListenAndServe()
}
```

`ListenAndServeTLS` loads the certificate and key from disk, performs the TLS handshake for each incoming connection, and then handles the HTTP protocol as usual. The rest of the application (middleware, handlers, MCP dispatch) is identical for HTTPS and plain HTTP.

### `html/template` — Server-Side HTML Rendering

The OAuth login page is generated server-side using Go's `html/template` package. Unlike `text/template`, `html/template` automatically escapes values inserted into HTML to prevent cross-site scripting (XSS).

```go
import "html/template"

var loginPageTemplate = template.Must(template.New("login").Parse(string(loginHTML)))

func (handler *OAuthHandler) serveLoginPage(writer http.ResponseWriter, req *http.Request) {
    data := struct {
        Providers []string
        CSRFToken string
    }{
        Providers: handler.enabledProviders(),
        CSRFToken: generateStateToken(),
    }
    loginPageTemplate.Execute(writer, data)
}
```

`template.Must` panics if the template fails to parse, catching template syntax errors at startup rather than at request time. Values inserted via `{{ .CSRFToken }}` are HTML-escaped automatically — an attacker cannot inject script tags through provider names or state values.

## 17. Go Glossary

| Abbreviation | Meaning | Explanation |
|---|---|---|
| `err` | error | Holds the error returned by the previous operation. In Go, functions return errors as values, not exceptions, so error handling is explicit. |
| `ctx` | context | Carries request-scoped deadlines, cancellation signals, and metadata across API boundaries. Essential for managing timeouts and coordinating shutdown. |
| `req` | request | The incoming tool call or HTTP request. Contains arguments, headers, and body. |
| `resp` | response | The HTTP or API response received from an upstream service. Contains status code, headers, and body. |
| `cfg` | config | The parsed application configuration loaded from environment variables at startup. |
| `srv` | server | The HTTP server instance or MCP server. In this project: `*http.Server` or `*mcpserver.MCPServer`. |
| `test` | test | The Go test runner (`*testing.T`) that provides logging and failure reporting. Used in all unit tests. |
