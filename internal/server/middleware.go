package server

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/reshinto/mcp-banana/internal/config"
	"github.com/reshinto/mcp-banana/internal/gemini"
	"github.com/reshinto/mcp-banana/internal/oauth"
	"github.com/reshinto/mcp-banana/internal/security"
	"golang.org/x/time/rate"
)

const (
	maxBodyBytes       = 15 * 1024 * 1024 // 15 MB
	concurrencyTimeout = 5 * time.Second
	healthzPath        = "/healthz"
	contentTypeJSON    = "application/json"
	authHeaderPrefix   = "Bearer "
)

// middleware holds the configuration and state for the HTTP middleware chain.
type middleware struct {
	cfg        *config.Config
	logger     *slog.Logger
	limiter    *rate.Limiter
	semaphore  chan struct{}
	oauthStore *oauth.Store
}

// newMiddleware creates a middleware instance configured from cfg.
// Pass a non-nil oauthStore to enable OAuth access token validation as a
// fallback authentication path alongside the static bearer token check.
func newMiddleware(cfg *config.Config, logger *slog.Logger, oauthStore *oauth.Store) *middleware {
	tokensPerSecond := rate.Limit(float64(cfg.RateLimit) / 60.0)
	limiter := rate.NewLimiter(tokensPerSecond, cfg.RateLimit)

	semaphore := make(chan struct{}, cfg.GlobalConcurrency)
	for slotIndex := 0; slotIndex < cfg.GlobalConcurrency; slotIndex++ {
		semaphore <- struct{}{}
	}

	return &middleware{
		cfg:        cfg,
		logger:     logger,
		limiter:    limiter,
		semaphore:  semaphore,
		oauthStore: oauthStore,
	}
}

// writeJSONError writes a JSON error response with the given HTTP status code and error key.
// It does not use http.Error to avoid the plain-text Content-Type override.
func writeJSONError(writer http.ResponseWriter, statusCode int, errorKey string) {
	writer.Header().Set("Content-Type", contentTypeJSON)
	writer.WriteHeader(statusCode)
	writer.Write([]byte(`{"error":"` + errorKey + `"}`)) //nolint:errcheck
}

// loadTokensFromFile reads a tokens file and returns all non-empty, non-comment
// lines as a set. The file is re-read on every call so tokens can be updated
// without restarting the server.
func loadTokensFromFile(filePath string) map[string]struct{} {
	tokens := make(map[string]struct{})
	if filePath == "" {
		return tokens
	}
	data, readError := os.ReadFile(filePath)
	if readError != nil {
		return tokens
	}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		// Skip empty lines and comments
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		tokens[trimmed] = struct{}{}
	}
	return tokens
}

// authenticateRequest checks whether the incoming request has a valid bearer
// token. Authentication is optional -- if no AuthToken and no AuthTokensFile
// are configured, all requests pass through (SSH tunnel provides security).
//
// Token sources checked in order:
//  1. AuthTokensFile (re-read on each request for hot-reload)
//  2. AuthToken (single legacy token from env var)
//
// If either source is configured, the request must provide a matching bearer token.
func (mw *middleware) authenticateRequest(request *http.Request) bool {
	hasFileTokens := mw.cfg.AuthTokensFile != ""
	hasSingleToken := mw.cfg.AuthToken != ""

	// No auth configured -- rely on SSH tunnel for security
	if !hasFileTokens && !hasSingleToken {
		return true
	}

	// Extract bearer token from request
	authHeader := request.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, authHeaderPrefix) {
		return false
	}
	requestToken := authHeader[len(authHeaderPrefix):]
	if requestToken == "" {
		return false
	}

	// Check tokens file first (hot-reloaded on each request)
	if hasFileTokens {
		fileTokens := loadTokensFromFile(mw.cfg.AuthTokensFile)
		if _, exists := fileTokens[requestToken]; exists {
			return true
		}
	}

	// Fall back to single token from env var
	if hasSingleToken && requestToken == mw.cfg.AuthToken {
		return true
	}

	// SECURITY: OAuth access tokens are validated last, after static bearer tokens.
	// Only reached when static auth is configured but no static token matched.
	if mw.oauthStore != nil && mw.oauthStore.ValidateAccessToken(requestToken) {
		return true
	}

	return false
}

// WrapHTTP wraps next with the full middleware chain:
//  1. Panic recovery
//  2. Health check bypass
//  3. Bearer token auth (optional -- skipped if no tokens configured)
//  4. Rate limiting
//  5. Global concurrency semaphore
//  6. Oversized body enforcement
func (mw *middleware) WrapHTTP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		// 1. Panic recovery
		defer func() {
			if recovered := recover(); recovered != nil {
				mw.logger.Error("panic recovered in HTTP handler", "recovered", recovered)
				writeJSONError(writer, http.StatusInternalServerError, "server_error")
			}
		}()

		// 2. Health check bypass — no auth, rate-limit, or body checks
		if request.URL.Path == healthzPath {
			next.ServeHTTP(writer, request)
			return
		}

		// 3. Bearer token auth (optional -- skipped when no tokens configured)
		if !mw.authenticateRequest(request) {
			writeJSONError(writer, http.StatusUnauthorized, "unauthorized")
			return
		}

		// Extract per-request Gemini API key from header and store in context.
		// SECURITY: Register the key with the sanitizer so it is redacted from all
		// logs and error output, then attach it to the request context for downstream
		// tool handlers to resolve the correct Gemini client.
		geminiAPIKey := request.Header.Get("X-Gemini-API-Key")
		if geminiAPIKey != "" {
			security.RegisterSecret(geminiAPIKey)
			request = request.WithContext(gemini.WithAPIKey(request.Context(), geminiAPIKey))
		}

		// 4. Rate limiting
		if !mw.limiter.Allow() {
			retryAfterSecs := int(1.0 / float64(mw.limiter.Limit()))
			if retryAfterSecs < 1 {
				retryAfterSecs = 1
			}
			writer.Header().Set("Retry-After", itoa(retryAfterSecs))
			writeJSONError(writer, http.StatusTooManyRequests, "rate_limited")
			return
		}

		// 5. Global concurrency semaphore with 5-second queue timeout
		select {
		case slot := <-mw.semaphore:
			defer func() { mw.semaphore <- slot }()
		case <-time.After(concurrencyTimeout):
			writeJSONError(writer, http.StatusServiceUnavailable, "server_busy")
			return
		}

		// 6. Oversized body enforcement via pre-read
		request.Body = http.MaxBytesReader(writer, request.Body, maxBodyBytes)
		bodyBytes, readError := io.ReadAll(request.Body)
		if readError != nil {
			var maxBytesError *http.MaxBytesError
			if errors.As(readError, &maxBytesError) {
				writeJSONError(writer, http.StatusRequestEntityTooLarge, "request_too_large")
				return
			}
			writeJSONError(writer, http.StatusBadRequest, "bad_request")
			return
		}
		request.Body = io.NopCloser(bytes.NewReader(bodyBytes))

		next.ServeHTTP(writer, request)
	})
}

// itoa converts a non-negative integer to a decimal string without fmt.
func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := make([]byte, 0, 10)
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	return string(digits)
}
