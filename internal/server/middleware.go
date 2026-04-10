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
	cfg       *config.Config
	logger    *slog.Logger
	limiter   *rate.Limiter
	semaphore chan struct{}
}

// newMiddleware creates a middleware instance configured from cfg.
func newMiddleware(cfg *config.Config, logger *slog.Logger) *middleware {
	tokensPerSecond := rate.Limit(float64(cfg.RateLimit) / 60.0)
	limiter := rate.NewLimiter(tokensPerSecond, cfg.RateLimit)

	semaphore := make(chan struct{}, cfg.GlobalConcurrency)
	for slotIndex := 0; slotIndex < cfg.GlobalConcurrency; slotIndex++ {
		semaphore <- struct{}{}
	}

	return &middleware{
		cfg:       cfg,
		logger:    logger,
		limiter:   limiter,
		semaphore: semaphore,
	}
}

// writeJSONError writes a JSON error response with the given HTTP status code and error key.
// It does not use http.Error to avoid the plain-text Content-Type override.
func writeJSONError(writer http.ResponseWriter, statusCode int, errorKey string) {
	writer.Header().Set("Content-Type", contentTypeJSON)
	writer.WriteHeader(statusCode)
	writer.Write([]byte(`{"error":"` + errorKey + `"}`)) //nolint:errcheck
}

// WrapHTTP wraps next with the full middleware chain:
//  1. Panic recovery
//  2. Health check bypass
//  3. Bearer token auth
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

		// 3. Bearer token auth
		if mw.cfg.AuthToken != "" {
			authHeader := request.Header.Get("Authorization")
			var token string
			if strings.HasPrefix(authHeader, authHeaderPrefix) {
				token = authHeader[len(authHeaderPrefix):]
			}
			if token != mw.cfg.AuthToken {
				writeJSONError(writer, http.StatusUnauthorized, "unauthorized")
				return
			}
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
