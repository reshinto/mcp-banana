package server

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/reshinto/mcp-banana/internal/config"
	"github.com/reshinto/mcp-banana/internal/credentials"
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
	credStore  *credentials.Store
}

// newMiddleware creates a middleware instance configured from cfg.
// Pass a non-nil oauthStore to enable OAuth access token validation.
// Pass a non-nil credStore to enable credentials-file-based auth and Gemini key resolution.
func newMiddleware(cfg *config.Config, logger *slog.Logger, oauthStore *oauth.Store, credStore *credentials.Store) *middleware {
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
		credStore:  credStore,
	}
}

// writeJSONError writes a JSON error response with the given HTTP status code and error key.
// It does not use http.Error to avoid the plain-text Content-Type override.
func writeJSONError(writer http.ResponseWriter, statusCode int, errorKey string) {
	writer.Header().Set("Content-Type", contentTypeJSON)
	writer.WriteHeader(statusCode)
	writer.Write([]byte(`{"error":"` + errorKey + `"}`)) //nolint:errcheck
}

// authenticateRequest resolves the client's identity and Gemini API key.
// Returns the Gemini API key and true if authenticated, or empty string and false if rejected.
//
// Resolution priority:
//  1. OAuth access token → resolve to provider:email → lookup in credentials file
//  2. Bearer token → lookup directly in credentials file
//  3. Unknown token + X-Gemini-API-Key header → self-register and proceed
//  4. Unknown token + no header → reject
//
// Auth is enforced if MCP_CREDENTIALS_FILE is set or OAuth is configured.
func (mw *middleware) authenticateRequest(request *http.Request) (string, bool) {
	hasCredentials := mw.credStore != nil
	hasOAuth := mw.oauthStore != nil

	// No auth configured — SSH tunnel mode
	if !hasCredentials && !hasOAuth {
		return "", true
	}

	// Extract bearer token
	authHeader := request.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, authHeaderPrefix) {
		return "", false
	}
	requestToken := authHeader[len(authHeaderPrefix):]
	if requestToken == "" {
		return "", false
	}

	// Priority 1: OAuth access token
	if hasOAuth {
		tokenData := mw.oauthStore.GetAccessTokenData(requestToken)
		if tokenData != nil && tokenData.ProviderIdentity != "" {
			if hasCredentials {
				geminiKey := mw.credStore.Lookup(tokenData.ProviderIdentity)
				if geminiKey != "" {
					security.RegisterSecret(geminiKey)
					return geminiKey, true
				}
			}
			// OAuth valid but no Gemini key in credentials file
			return "", false
		}
	}

	// Priority 2: Static bearer token in credentials file
	if hasCredentials {
		geminiKey := mw.credStore.Lookup(requestToken)
		if geminiKey != "" {
			// SECURITY: Register both the bearer token and Gemini key with the
			// sanitizer so neither appears in logs or error output.
			security.RegisterSecret(requestToken)
			security.RegisterSecret(geminiKey)
			return geminiKey, true
		}
	}

	// Priority 3: Self-registration via X-Gemini-API-Key header
	geminiAPIKey := request.Header.Get("X-Gemini-API-Key")
	geminiAPIKey = strings.TrimSpace(geminiAPIKey)
	if geminiAPIKey != "" && hasCredentials {
		// SECURITY: Require minimum token entropy to prevent trivial token guessing
		// or accidental pre-registration of short/weak tokens.
		const minTokenLength = 32
		if len(requestToken) < minTokenLength {
			mw.logger.Warn("self-registration rejected: bearer token too short", "length", len(requestToken))
			return "", false
		}
		// SECURITY: Validate the key before registering to prevent storing invalid keys
		validateError := credentials.ValidateGeminiKey(request.Context(), geminiAPIKey)
		if validateError != nil {
			mw.logger.Warn("self-registration rejected: invalid Gemini API key")
			return "", false
		}
		registerError := mw.credStore.Register(requestToken, geminiAPIKey)
		if registerError != nil {
			mw.logger.Error("failed to register credentials", "error", registerError)
			return "", false
		}
		security.RegisterSecret(requestToken)
		security.RegisterSecret(geminiAPIKey)
		return geminiAPIKey, true
	}

	// Priority 4: Reject
	return "", false
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

		// 2. Bearer token auth + Gemini key resolution
		geminiKey, authenticated := mw.authenticateRequest(request)
		if !authenticated {
			if mw.cfg.OAuthBaseURL != "" {
				writer.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+mw.cfg.OAuthBaseURL+`/.well-known/oauth-protected-resource"`)
			}
			writeJSONError(writer, http.StatusUnauthorized, "unauthorized")
			return
		}

		// Store resolved Gemini key in context for downstream tool handlers
		if geminiKey != "" {
			request = request.WithContext(gemini.WithAPIKey(request.Context(), geminiKey))
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
