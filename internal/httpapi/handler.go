package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/Santosh27927/Backend-COT/internal/authentication"
)

type Readiness func(context.Context) (bool, string)
type requestIDKey struct{}

var requestIDPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

const Version = "0.5.0-dev"

func requestID(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey{}).(string)
	return value
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}, "requestId": requestID(r.Context())})
}
func getOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			writeError(w, r, 405, "METHOD_NOT_ALLOWED", "Use GET for this endpoint.")
			return
		}
		next(w, r)
	}
}

func Handler(service string, logger *slog.Logger, a authentication.Authenticator, ready Readiness, routes ...func(*http.ServeMux)) http.Handler {
	logger = logger.With("service", service)
	mux := http.NewServeMux()
	mux.HandleFunc("/health/live", getOnly(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]string{"status": "live", "service": service, "version": Version})
	}))
	mux.HandleFunc("/health/ready", getOnly(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		ok, code := ready(ctx)
		status := http.StatusOK
		if !ok {
			status = http.StatusServiceUnavailable
		}
		writeJSON(w, status, map[string]any{"ready": ok, "service": service, "dependencies": map[string]string{"database": code}, "requestId": requestID(r.Context())})
	}))
	mux.HandleFunc("/v1/me", getOnly(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := authentication.FromRequest(r.Context(), r, a)
		if !ok {
			w.Header().Set("WWW-Authenticate", "Bearer")
			writeError(w, r, 401, "AUTH_REQUIRED", "A valid bearer token is required.")
			return
		}
		writeJSON(w, 200, map[string]any{"principal": principal, "service": service, "requestId": requestID(r.Context())})
	}))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, r, 404, "NOT_FOUND", "Endpoint not found.")
	})
	for _, register := range routes {
		register(mux)
	}
	return observe(logger, mux)
}

type recorder struct {
	http.ResponseWriter
	status int
}

func (w *recorder) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
		w.ResponseWriter.WriteHeader(code)
	}
}
func (w *recorder) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

func observe(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if !requestIDPattern.MatchString(id) {
			var raw [16]byte
			if _, err := rand.Read(raw[:]); err != nil {
				writeJSON(w, 500, map[string]string{"error": "request_id_unavailable"})
				return
			}
			id = hex.EncodeToString(raw[:])
		}
		r = r.WithContext(context.WithValue(r.Context(), requestIDKey{}, id))
		w.Header().Set("X-Request-ID", id)
		wrapped := &recorder{ResponseWriter: w}
		start := time.Now()
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.ErrorContext(r.Context(), "request panic recovered", "request_id", id, "panic_type", fmt.Sprintf("%T", recovered))
				if wrapped.status == 0 {
					writeError(wrapped, r, 500, "INTERNAL_ERROR", "The request could not be completed.")
				}
			}
			status := wrapped.status
			if status == 0 {
				status = 200
			}
			// Do not log credentials, raw query strings, request bodies, or raw database errors.
			logger.InfoContext(r.Context(), "http request", "request_id", id, "method", r.Method, "route", r.Pattern, "status", status, "duration_ms", time.Since(start).Milliseconds())
		}()
		r.Body = http.MaxBytesReader(wrapped, r.Body, 64*1024)
		next.ServeHTTP(wrapped, r)
	})
}
