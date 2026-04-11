package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/reshinto/mcp-banana/internal/gemini"
	"github.com/reshinto/mcp-banana/internal/security"
)

// withCleanSecrets clears registered secrets after the test to prevent cross-test pollution.
func withCleanSecrets(test *testing.T) {
	test.Helper()
	test.Cleanup(security.ClearSecrets)
}

// withVerifiedRegistry overrides the registry validator to always pass.
func withVerifiedRegistry(test *testing.T) {
	test.Helper()
	original := registryValidator
	test.Cleanup(func() { registryValidator = original })
	registryValidator = func() error { return nil }
}

// withMockClientFactory replaces the client factory with one that returns a real
// client (no network calls in genai.NewClient) or an error.
func withMockClientFactory(test *testing.T, mockError error) {
	test.Helper()
	original := clientFactory
	test.Cleanup(func() { clientFactory = original })
	if mockError != nil {
		clientFactory = func(_ context.Context, _ string, _ int, _ int) (*gemini.Client, error) {
			return nil, mockError
		}
	} else {
		clientFactory = func(ctx context.Context, apiKey string, timeoutSecs int, proConcurrency int) (*gemini.Client, error) {
			return gemini.NewClient(ctx, apiKey, timeoutSecs, proConcurrency)
		}
	}
}

// withMockStdio overrides stdioServe with a mock that returns the given error.
func withMockStdio(test *testing.T, mockError error) {
	test.Helper()
	original := stdioServe
	test.Cleanup(func() { stdioServe = original })
	stdioServe = func(_ *mcpserver.MCPServer) error {
		return mockError
	}
}

// withMockListener overrides listenFunc with a mock that returns the given listener or error.
func withMockListener(test *testing.T, listener net.Listener, mockError error) {
	test.Helper()
	original := listenFunc
	test.Cleanup(func() { listenFunc = original })
	listenFunc = func(_, _ string) (net.Listener, error) {
		if mockError != nil {
			return nil, mockError
		}
		return listener, nil
	}
}

// setupServerEnv configures a valid environment, bypasses registry, and provides a mock client.
func setupServerEnv(test *testing.T) {
	test.Helper()
	withCleanSecrets(test)
	test.Setenv("GEMINI_API_KEY", "test-gemini-key-placeholder-for-unit-tests")
	withVerifiedRegistry(test)
	withMockClientFactory(test, nil)
}

// --- Default var function bodies ---

// TestDefaultRegistryValidator exercises the production registryValidator var
// so that its function body is included in coverage.
func TestDefaultRegistryValidator(test *testing.T) {
	if validationError := registryValidator(); validationError != nil {
		test.Fatalf("default registry validator failed: %v", validationError)
	}
}

// TestDefaultClientFactory exercises the production clientFactory var
// so that its function body is included in coverage.
func TestDefaultClientFactory(test *testing.T) {
	client, factoryError := clientFactory(context.Background(), "test-key-for-coverage", 30, 2)
	if factoryError != nil {
		test.Fatalf("default client factory failed: %v", factoryError)
	}
	if client == nil {
		test.Fatal("expected non-nil client")
	}
}

// TestDefaultStdioServe exercises the production stdioServe var by providing
// a closed stdin so ServeStdio returns immediately with an EOF/read error.
func TestDefaultStdioServe(test *testing.T) {
	// Replace os.Stdin with a pipe that is immediately closed, so ServeStdio
	// reads EOF and returns instead of blocking.
	originalStdin := os.Stdin
	pipeReader, pipeWriter, pipeError := os.Pipe()
	if pipeError != nil {
		test.Fatalf("failed to create pipe: %v", pipeError)
	}
	_ = pipeWriter.Close() // close write end so reads get EOF
	os.Stdin = pipeReader
	test.Cleanup(func() { os.Stdin = originalStdin; _ = pipeReader.Close() })

	// Create a minimal MCP server to pass to stdioServe.
	mcpServer := mcpserver.NewMCPServer("test", "0.0.0")

	// stdioServe may return an error (EOF) which is expected; we only care
	// that the production closure body was executed for coverage.
	_ = stdioServe(mcpServer)
}

// TestDefaultListenFunc exercises the production listenFunc var
// so that its function body is included in coverage.
func TestDefaultListenFunc(test *testing.T) {
	listener, listenError := listenFunc("tcp", "127.0.0.1:0")
	if listenError != nil {
		test.Fatalf("default listen func failed: %v", listenError)
	}
	_ = listener.Close()
}

// --- main() ---

func TestMain_CallsOsExit(test *testing.T) {
	// Override osExit to capture the exit code instead of terminating.
	var capturedCode int
	originalOsExit := osExit
	test.Cleanup(func() { osExit = originalOsExit })
	osExit = func(code int) { capturedCode = code }

	// Override os.Args to pass --version (a quick, side-effect-free path).
	originalArgs := os.Args
	test.Cleanup(func() { os.Args = originalArgs })
	os.Args = []string{"mcp-banana", "--version"}

	main()

	if capturedCode != 0 {
		test.Fatalf("expected exit code 0, got %d", capturedCode)
	}
}

// --- Version flag ---

func TestRun_VersionFlag(test *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{"--version"}, &stdout, &stderr)
	if exitCode != 0 {
		test.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(stdout.String(), "mcp-banana") {
		test.Errorf("expected version output, got: %q", stdout.String())
	}
}

// --- Invalid flags ---

func TestRun_InvalidFlag(test *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run([]string{"--invalid-flag"}, &stdout, &stderr)
	if exitCode != 1 {
		test.Fatalf("expected exit code 1, got %d", exitCode)
	}
}

// --- Health check ---

func TestRun_HealthCheckSuccess(test *testing.T) {
	healthServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/healthz" {
			writer.WriteHeader(http.StatusOK)
			return
		}
		writer.WriteHeader(http.StatusNotFound)
	}))
	defer healthServer.Close()

	address := strings.TrimPrefix(healthServer.URL, "http://")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run([]string{"--healthcheck", "--addr", address}, &stdout, &stderr)
	if exitCode != 0 {
		test.Fatalf("expected exit code 0, got %d; stderr: %s", exitCode, stderr.String())
	}
}

func TestRun_HealthCheckNon200(test *testing.T) {
	healthServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusInternalServerError)
	}))
	defer healthServer.Close()

	address := strings.TrimPrefix(healthServer.URL, "http://")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run([]string{"--healthcheck", "--addr", address}, &stdout, &stderr)
	if exitCode != 1 {
		test.Fatalf("expected exit code 1, got %d", exitCode)
	}
	if !strings.Contains(stderr.String(), "health check returned status") {
		test.Errorf("expected status error, got: %q", stderr.String())
	}
}

func TestRun_HealthCheckConnectionRefused(test *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run([]string{"--healthcheck", "--addr", "127.0.0.1:1"}, &stdout, &stderr)
	if exitCode != 1 {
		test.Fatalf("expected exit code 1, got %d", exitCode)
	}
	if !strings.Contains(stderr.String(), "health check failed") {
		test.Errorf("expected fetch error, got: %q", stderr.String())
	}
}

// --- Config loading ---

func TestRun_NoAPIKey_StartsSuccessfully(test *testing.T) {
	withCleanSecrets(test)
	test.Setenv("GEMINI_API_KEY", "")

	origValidator := registryValidator
	origStdio := stdioServe
	defer func() { registryValidator = origValidator; stdioServe = origStdio }()

	registryValidator = func() error { return nil }
	stdioServe = func(_ *mcpserver.MCPServer) error { return nil }

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run([]string{"--transport", "stdio"}, &stdout, &stderr)
	if exitCode != 0 {
		test.Fatalf("expected exit code 0 (server starts without API key), got %d; stderr: %s", exitCode, stderr.String())
	}
}

// --- Config load failure ---

func TestRun_ConfigLoadFailure(test *testing.T) {
	withCleanSecrets(test)
	// Set an invalid log level to force config.Load() to fail.
	test.Setenv("MCP_LOG_LEVEL", "INVALID_LEVEL_XYZ")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run([]string{"--transport", "stdio"}, &stdout, &stderr)
	if exitCode != 1 {
		test.Fatalf("expected exit code 1, got %d", exitCode)
	}
	if !strings.Contains(stderr.String(), "failed to load config") {
		test.Errorf("expected config error message, got: %q", stderr.String())
	}
}

// --- Credentials file ---

func TestRun_CredentialsFileLoadError(test *testing.T) {
	withCleanSecrets(test)
	test.Setenv("GEMINI_API_KEY", "test-gemini-key-placeholder-for-unit-tests")
	// Point to a directory (not a file) to trigger credentials.NewStore failure.
	test.Setenv("MCP_CREDENTIALS_FILE", "/dev/null/nonexistent")
	withVerifiedRegistry(test)
	withMockClientFactory(test, nil)
	withMockStdio(test, nil)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run([]string{"--transport", "stdio"}, &stdout, &stderr)
	if exitCode != 1 {
		test.Fatalf("expected exit code 1, got %d", exitCode)
	}
	if !strings.Contains(stderr.String(), "failed to load credentials file") {
		test.Errorf("expected credentials file error, got: %q", stderr.String())
	}
}

func TestRun_CredentialsFileLoadSuccess(test *testing.T) {
	withCleanSecrets(test)
	test.Setenv("GEMINI_API_KEY", "test-gemini-key-placeholder-for-unit-tests")

	// Create a temporary valid credentials file.
	credFile, fileError := os.CreateTemp(test.TempDir(), "creds-*.json")
	if fileError != nil {
		test.Fatalf("failed to create temp credentials file: %v", fileError)
	}
	_, _ = credFile.WriteString("{}")
	_ = credFile.Close()

	test.Setenv("MCP_CREDENTIALS_FILE", credFile.Name())
	withVerifiedRegistry(test)
	withMockClientFactory(test, nil)
	withMockStdio(test, nil)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run([]string{"--transport", "stdio"}, &stdout, &stderr)
	if exitCode != 0 {
		test.Fatalf("expected exit code 0, got %d; stderr: %s", exitCode, stderr.String())
	}
}

// --- Registry validation ---

func TestRun_RegistryValidationFails(test *testing.T) {
	withCleanSecrets(test)
	test.Setenv("GEMINI_API_KEY", "test-gemini-key-placeholder-for-unit-tests")

	// Override the registry validator to simulate a sentinel ID failure.
	original := registryValidator
	test.Cleanup(func() { registryValidator = original })
	registryValidator = func() error {
		return fmt.Errorf("model %q has unverified GeminiID -- verify at https://ai.google.dev/gemini-api/docs/models before release", "test-model")
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run([]string{}, &stdout, &stderr)
	if exitCode != 1 {
		test.Fatalf("expected exit code 1, got %d", exitCode)
	}
	if !strings.Contains(stderr.String(), "registry validation failed") {
		test.Errorf("expected registry error, got: %q", stderr.String())
	}
}

// --- Client factory error ---

// --- Unknown transport ---

func TestRun_UnknownTransport(test *testing.T) {
	setupServerEnv(test)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run([]string{"--transport", "xyz"}, &stdout, &stderr)
	if exitCode != 1 {
		test.Fatalf("expected exit code 1, got %d", exitCode)
	}
	if !strings.Contains(stderr.String(), "unknown transport") {
		test.Errorf("expected unknown transport error, got: %q", stderr.String())
	}
}

// --- Log levels ---

func TestResolveLogLevel(test *testing.T) {
	cases := []struct {
		input    string
		expected slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"info", slog.LevelInfo},
		{"unknown", slog.LevelInfo},
	}
	for _, testCase := range cases {
		test.Run(testCase.input, func(test *testing.T) {
			result := resolveLogLevel(testCase.input)
			if result != testCase.expected {
				test.Errorf("resolveLogLevel(%q) = %v, expected %v", testCase.input, result, testCase.expected)
			}
		})
	}
}

// --- Stdio mode ---

func TestRun_StdioSuccess(test *testing.T) {
	setupServerEnv(test)
	withMockStdio(test, nil)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run([]string{"--transport", "stdio"}, &stdout, &stderr)
	if exitCode != 0 {
		test.Fatalf("expected exit code 0, got %d; stderr: %s", exitCode, stderr.String())
	}
}

func TestRun_StdioError(test *testing.T) {
	setupServerEnv(test)
	withMockStdio(test, errors.New("simulated stdio error"))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run([]string{"--transport", "stdio"}, &stdout, &stderr)
	if exitCode != 1 {
		test.Fatalf("expected exit code 1, got %d", exitCode)
	}
	if !strings.Contains(stderr.String(), "stdio server error") {
		test.Errorf("expected stdio error message, got: %q", stderr.String())
	}
}

// --- HTTP mode ---

func TestRun_HTTPModeStartsAndShutdown(test *testing.T) {
	setupServerEnv(test)

	// Create a real listener on a random port.
	listener, listenError := net.Listen("tcp", "127.0.0.1:0")
	if listenError != nil {
		test.Fatalf("failed to create listener: %v", listenError)
	}
	listenerAddress := listener.Addr().String()
	_ = listener.Close() // close so we can re-listen in the test

	original := listenFunc
	test.Cleanup(func() { listenFunc = original })
	listenFunc = func(_, _ string) (net.Listener, error) {
		return net.Listen("tcp", listenerAddress)
	}

	done := make(chan int, 1)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	go func() {
		exitCode := run([]string{"--transport", "http", "--addr", listenerAddress}, &stdout, &stderr)
		done <- exitCode
	}()

	// Wait for the server to be ready.
	httpClient := &http.Client{Timeout: 2 * time.Second}
	healthURL := "http://" + listenerAddress + "/healthz"
	for attempt := 0; attempt < 50; attempt++ {
		resp, fetchError := httpClient.Get(healthURL)
		if fetchError == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Send SIGINT to trigger graceful shutdown.
	sendSignalError := syscall.Kill(os.Getpid(), syscall.SIGINT)
	if sendSignalError != nil {
		test.Fatalf("failed to send SIGINT: %v", sendSignalError)
	}

	select {
	case exitCode := <-done:
		if exitCode != 0 {
			test.Fatalf("expected exit code 0, got %d; stderr: %s", exitCode, stderr.String())
		}
	case <-time.After(10 * time.Second):
		test.Fatal("timed out waiting for server shutdown")
	}
}

func TestRun_HTTPNoAuthWarning(test *testing.T) {
	setupServerEnv(test)
	test.Setenv("MCP_CREDENTIALS_FILE", "")

	listener, listenError := net.Listen("tcp", "127.0.0.1:0")
	if listenError != nil {
		test.Fatalf("failed to create listener: %v", listenError)
	}
	listenerAddress := listener.Addr().String()
	_ = listener.Close()

	original := listenFunc
	test.Cleanup(func() { listenFunc = original })
	listenFunc = func(_, _ string) (net.Listener, error) {
		return net.Listen("tcp", listenerAddress)
	}

	done := make(chan int, 1)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	go func() {
		exitCode := run([]string{"--transport", "http", "--addr", listenerAddress}, &stdout, &stderr)
		done <- exitCode
	}()

	// Wait for server to start.
	httpClient := &http.Client{Timeout: 2 * time.Second}
	healthURL := "http://" + listenerAddress + "/healthz"
	for attempt := 0; attempt < 50; attempt++ {
		resp, fetchError := httpClient.Get(healthURL)
		if fetchError == nil {
			_ = resp.Body.Close()
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Send SIGINT to shut down.
	_ = syscall.Kill(os.Getpid(), syscall.SIGINT)

	select {
	case exitCode := <-done:
		if exitCode != 0 {
			test.Fatalf("expected exit code 0, got %d; stderr: %s", exitCode, stderr.String())
		}
	case <-time.After(10 * time.Second):
		test.Fatal("timed out waiting for server shutdown")
	}

	if !strings.Contains(stderr.String(), "auth is disabled") {
		test.Errorf("expected auth warning in log output, got: %q", stderr.String())
	}
}

func TestRun_HTTPShutdownError(test *testing.T) {
	setupServerEnv(test)

	// Set shutdown timeout to 0 so Shutdown always fails when connections exist.
	originalTimeout := shutdownTimeout
	test.Cleanup(func() { shutdownTimeout = originalTimeout })
	shutdownTimeout = time.Nanosecond

	listener, listenError := net.Listen("tcp", "127.0.0.1:0")
	if listenError != nil {
		test.Fatalf("failed to create listener: %v", listenError)
	}
	listenerAddress := listener.Addr().String()
	_ = listener.Close()

	original := listenFunc
	test.Cleanup(func() { listenFunc = original })
	listenFunc = func(_, _ string) (net.Listener, error) {
		return net.Listen("tcp", listenerAddress)
	}

	done := make(chan int, 1)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	go func() {
		exitCode := run([]string{"--transport", "http", "--addr", listenerAddress}, &stdout, &stderr)
		done <- exitCode
	}()

	// Wait for the server to be ready.
	httpClient := &http.Client{Timeout: 2 * time.Second}
	healthURL := "http://" + listenerAddress + "/healthz"
	for attempt := 0; attempt < 50; attempt++ {
		resp, fetchError := httpClient.Get(healthURL)
		if fetchError == nil {
			_ = resp.Body.Close()
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Open a connection and hold it to force shutdown timeout error.
	holdConn, dialError := net.Dial("tcp", listenerAddress)
	if dialError != nil {
		test.Fatalf("failed to dial server: %v", dialError)
	}
	defer func() { _ = holdConn.Close() }()

	// Send partial HTTP request to keep the connection alive during shutdown.
	_, _ = holdConn.Write([]byte("GET / HTTP/1.1\r\nHost: localhost\r\n"))

	// Send SIGINT to trigger shutdown (which should fail due to zero timeout + held connection).
	_ = syscall.Kill(os.Getpid(), syscall.SIGINT)

	select {
	case <-done:
		// Server exited, check logs for shutdown error.
		// The shutdown error path in the goroutine logs the error but doesn't affect exit code,
		// so the server still exits cleanly (ErrServerClosed).
	case <-time.After(10 * time.Second):
		test.Fatal("timed out waiting for server shutdown")
	}
}

func TestRun_HTTPServeError(test *testing.T) {
	setupServerEnv(test)

	// Create a listener and close it immediately so Serve fails.
	listener, listenError := net.Listen("tcp", "127.0.0.1:0")
	if listenError != nil {
		test.Fatalf("failed to create listener: %v", listenError)
	}
	listenerAddress := listener.Addr().String()
	_ = listener.Close() // close it so Serve gets an error

	original := listenFunc
	test.Cleanup(func() { listenFunc = original })
	listenFunc = func(_, _ string) (net.Listener, error) {
		// Return the already-closed listener. Serve will fail with "use of closed network connection".
		return &closedListener{address: listenerAddress}, nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run([]string{"--transport", "http", "--addr", listenerAddress}, &stdout, &stderr)
	if exitCode != 1 {
		test.Fatalf("expected exit code 1, got %d", exitCode)
	}
}

// closedListener is a net.Listener that fails immediately on Accept.
type closedListener struct {
	address string
}

func (closed *closedListener) Accept() (net.Conn, error) {
	return nil, errors.New("listener is closed")
}

func (closed *closedListener) Close() error {
	return nil
}

func (closed *closedListener) Addr() net.Addr {
	addr, _ := net.ResolveTCPAddr("tcp", closed.address)
	return addr
}

func TestRun_HTTPListenError(test *testing.T) {
	setupServerEnv(test)
	withMockListener(test, nil, errors.New("simulated listen error"))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run([]string{"--transport", "http"}, &stdout, &stderr)
	if exitCode != 1 {
		test.Fatalf("expected exit code 1, got %d", exitCode)
	}
}

// --- OAuth store creation (run path with providers) ---

// TestRun_OAuthStoreCreatedWhenProvidersConfigured verifies that when at least one
// OAuth provider is configured, an oauth.Store is created and the cleanup goroutine
// is started. The test uses the stdio transport so it exits quickly and uses a
// very short cleanup interval to exercise the goroutine body (CleanupExpired call).
func TestRun_OAuthStoreCreatedWhenProvidersConfigured(test *testing.T) {
	setupServerEnv(test)
	// Set both ID and secret so BuildActiveProviders returns at least one provider.
	test.Setenv("OAUTH_GOOGLE_CLIENT_ID", "google-client-id-test")
	test.Setenv("OAUTH_GOOGLE_CLIENT_SECRET", "google-client-secret-test")
	withMockStdio(test, nil)

	// Set a very short cleanup interval so the goroutine fires during this test.
	originalInterval := oauthCleanupInterval
	test.Cleanup(func() { oauthCleanupInterval = originalInterval })
	oauthCleanupInterval = 1 * time.Millisecond

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run([]string{"--transport", "stdio"}, &stdout, &stderr)
	if exitCode != 0 {
		test.Fatalf("expected exit code 0 with OAuth providers configured, got %d; stderr: %s", exitCode, stderr.String())
	}

	// Give the cleanup goroutine a moment to fire before the test exits.
	time.Sleep(10 * time.Millisecond)
}

// --- TLS: runHealthCheck ---

// TestRunHealthCheck_TLSPath verifies that when MCP_TLS_CERT_FILE is set, the health
// check uses HTTPS (which will fail to connect since no real TLS server is running,
// but the code path through the TLS transport setup is exercised).
func TestRunHealthCheck_TLSPath(test *testing.T) {
	// Set the env var so runHealthCheck switches to the HTTPS path.
	test.Setenv("MCP_TLS_CERT_FILE", "/tmp/fake-cert.pem")

	var stderr bytes.Buffer
	// 127.0.0.1:1 — nothing is listening, so the HTTPS GET will fail immediately.
	exitCode := runHealthCheck("127.0.0.1:1", &stderr)
	if exitCode != 1 {
		test.Fatalf("expected exit code 1 for TLS health check to closed port, got %d", exitCode)
	}
	if !strings.Contains(stderr.String(), "health check failed") {
		test.Errorf("expected health check error message, got: %q", stderr.String())
	}
}

// --- TLS: runHTTPServer ---

// TestRun_HTTPTLSServeError verifies that when TLS cert and key paths are set but
// invalid, ServeTLS fails and runHTTPServer returns exit code 1.
func TestRun_HTTPTLSServeError(test *testing.T) {
	setupServerEnv(test)
	test.Setenv("MCP_TLS_CERT_FILE", "/tmp/nonexistent-cert.pem")
	test.Setenv("MCP_TLS_KEY_FILE", "/tmp/nonexistent-key.pem")

	listener, listenError := net.Listen("tcp", "127.0.0.1:0")
	if listenError != nil {
		test.Fatalf("failed to create listener: %v", listenError)
	}
	listenerAddress := listener.Addr().String()
	_ = listener.Close()

	withMockListener(test, nil, nil)

	original := listenFunc
	test.Cleanup(func() { listenFunc = original })
	listenFunc = func(_, _ string) (net.Listener, error) {
		return net.Listen("tcp", listenerAddress)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run([]string{"--transport", "http", "--addr", listenerAddress}, &stdout, &stderr)
	if exitCode != 1 {
		test.Fatalf("expected exit code 1 for TLS with invalid cert/key paths, got %d; stderr: %s", exitCode, stderr.String())
	}
}
