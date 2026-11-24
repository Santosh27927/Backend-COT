package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"time"

	"github.com/Santosh27927/Backend-COT/internal/authentication"
	"github.com/Santosh27927/Backend-COT/internal/sessions"
	"github.com/Santosh27927/Backend-COT/internal/trading/execution"
	"github.com/jackc/pgx/v5/pgconn"
)

// LearningRoutes is mounted only by game-service. No client-supplied player ID
// is accepted in command bodies, headers, or query parameters.
func LearningRoutes(a authentication.Authenticator, store *sessions.Store, logger *slog.Logger) func(*http.ServeMux) {
	return func(mux *http.ServeMux) {
		mount := func(path, method string, fn func(http.ResponseWriter, *http.Request, string)) {
			mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != method {
					w.Header().Set("Allow", method)
					writeError(w, r, 405, "METHOD_NOT_ALLOWED", "Unsupported method for this endpoint.")
					return
				}
				principal, ok := authentication.FromRequest(r.Context(), r, a)
				if !ok {
					w.Header().Set("WWW-Authenticate", "Bearer")
					writeError(w, r, 401, "AUTH_REQUIRED", "A valid bearer token is required.")
					return
				}
				ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
				defer cancel()
				fn(w, r.WithContext(ctx), principal.PlayerID)
			})
		}
		fail := func(w http.ResponseWriter, r *http.Request, err error) {
			status, code, message := 500, "INTERNAL_ERROR", "The operation could not be completed."
			switch {
			case errors.Is(err, execution.ErrFunds):
				status, code, message = 409, "INSUFFICIENT_AVAILABLE_FUNDS", "Available cash, collateral, or unreserved holdings are insufficient."
			case errors.Is(err, execution.ErrClosed):
				status, code, message = 409, "SESSION_COMPLETED", "Retrieve the completed report or start a new session."
			case errors.Is(err, execution.ErrUnsupported):
				status, code, message = 400, "FEATURE_NOT_ENABLED", "This feature is not enabled in this lesson."
			case errors.Is(err, sessions.ErrLocked):
				status, code, message = 409, "LESSON_LOCKED", "Complete prerequisite practical lessons first."
			case errors.Is(err, sessions.ErrInvalid), errors.Is(err, execution.ErrInvalid):
				status, code, message = 400, "INVALID_COMMAND", "Check command values and the Idempotency-Key header."
			case errors.Is(err, sessions.ErrNotFound):
				status, code, message = 404, "NOT_FOUND", "Session or scenario not found."
			case errors.Is(err, sessions.ErrActive):
				status, code, message = 409, "ACTIVE_SESSION_EXISTS", "Retrieve the active session or explicitly restart it."
			case errors.Is(err, sessions.ErrKeyConflict):
				status, code, message = 409, "IDEMPOTENCY_KEY_CONFLICT", "Use a new key for a different command."
			case errors.Is(err, sessions.ErrVersionConflict):
				status, code, message = 503, "SCENARIO_VERSION_CONFLICT", "Scenario rules need a new version before use."
			}
			if status >= 500 {
				state := ""
				var pgError *pgconn.PgError
				if errors.As(err, &pgError) {
					state = pgError.Code
				}
				logger.ErrorContext(r.Context(), "learning operation failed", "request_id", requestID(r.Context()), "operation", r.Pattern, "code", code, "sqlstate", state)
			}
			writeError(w, r, status, code, message)
		}
		decode := func(w http.ResponseWriter, r *http.Request, destination any) bool {
			media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil || media != "application/json" {
				writeError(w, r, 415, "JSON_REQUIRED", "Use Content-Type: application/json.")
				return false
			}
			decoder := json.NewDecoder(r.Body)
			decoder.DisallowUnknownFields()
			var raw json.RawMessage
			err = decoder.Decode(&raw)
			if err == nil {
				var extra any
				err = decoder.Decode(&extra)
				if err == io.EOF {
					err = nil
				} else if err == nil {
					err = sessions.ErrInvalid
				}
			}
			if err == nil {
				trimmed := bytes.TrimSpace(raw)
				if len(trimmed) == 0 || trimmed[0] != '{' {
					err = sessions.ErrInvalid
				} else {
					object := json.NewDecoder(bytes.NewReader(trimmed))
					object.DisallowUnknownFields()
					err = object.Decode(destination)
				}
			}
			if err != nil {
				var tooLarge *http.MaxBytesError
				if errors.As(err, &tooLarge) {
					writeError(w, r, 413, "BODY_TOO_LARGE", "Request body exceeds the size limit.")
				} else {
					writeError(w, r, 400, "INVALID_JSON", "Use one JSON object with only documented fields.")
				}
				return false
			}
			return true
		}
		key := func(r *http.Request) string {
			if len(r.Header.Values("Idempotency-Key")) != 1 {
				return ""
			}
			return r.Header.Get("Idempotency-Key")
		}
		mount("/v1/scenarios", "GET", func(w http.ResponseWriter, r *http.Request, _ string) {
			writeJSON(w, 200, map[string]any{"scenarios": store.Scenarios(), "requestId": requestID(r.Context())})
		})
		mount("/v1/progress", "GET", func(w http.ResponseWriter, r *http.Request, player string) {
			result, err := store.Progress(r.Context(), player)
			if err != nil {
				fail(w, r, err)
				return
			}
			writeJSON(w, 200, map[string]any{"progress": result, "requestId": requestID(r.Context())})
		})
		// A single path serves list and create, with explicit method dispatch.
		mux.HandleFunc("/v1/sessions", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "GET" && r.Method != "POST" {
				w.Header().Set("Allow", "GET, POST")
				writeError(w, r, 405, "METHOD_NOT_ALLOWED", "Use GET or POST.")
				return
			}
			principal, ok := authentication.FromRequest(r.Context(), r, a)
			if !ok {
				w.Header().Set("WWW-Authenticate", "Bearer")
				writeError(w, r, 401, "AUTH_REQUIRED", "A valid bearer token is required.")
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()
			r = r.WithContext(ctx)
			if r.Method == "GET" {
				result, err := store.List(ctx, principal.PlayerID)
				if err != nil {
					fail(w, r, err)
					return
				}
				writeJSON(w, 200, map[string]any{"sessions": result, "requestId": requestID(ctx)})
				return
			}
			var input struct {
				ScenarioID string `json:"scenarioId"`
			}
			if !decode(w, r, &input) {
				return
			}
			result, replayed, err := store.Start(ctx, principal.PlayerID, key(r), sessions.StartCommand{ScenarioID: input.ScenarioID})
			if err != nil {
				fail(w, r, err)
				return
			}
			w.Header().Set("Idempotency-Replayed", strconv.FormatBool(replayed))
			logger.InfoContext(ctx, "session command committed", "request_id", requestID(ctx), "session_id", result.Session.ID, "account_id", result.Account.ID, "replayed", replayed, "operation", "start")
			writeJSON(w, 201, map[string]any{"data": result, "requestId": requestID(ctx)})
		})
		mount("/v1/sessions/{id}", "GET", func(w http.ResponseWriter, r *http.Request, player string) {
			result, err := store.Get(r.Context(), player, r.PathValue("id"))
			if err != nil {
				fail(w, r, err)
				return
			}
			writeJSON(w, 200, map[string]any{"data": result, "requestId": requestID(r.Context())})
		})
		trade := func(w http.ResponseWriter, r *http.Request, player string, input execution.Command) {
			result, replayed, err := store.Trade(r.Context(), player, key(r), r.PathValue("id"), input)
			if err != nil {
				fail(w, r, err)
				return
			}
			w.Header().Set("Idempotency-Replayed", strconv.FormatBool(replayed))
			writeJSON(w, 200, map[string]any{"data": result, "requestId": requestID(r.Context())})
		}
		mount("/v1/sessions/{id}/orders", "POST", func(w http.ResponseWriter, r *http.Request, player string) {
			var input execution.Command
			if !decode(w, r, &input) {
				return
			}
			if input.Action != "" || input.OrderID != "" {
				fail(w, r, execution.ErrInvalid)
				return
			}
			trade(w, r, player, input)
		})
		mount("/v1/sessions/{id}/orders/{orderId}/cancel", "POST", func(w http.ResponseWriter, r *http.Request, player string) {
			var input struct{}
			if !decode(w, r, &input) {
				return
			}
			trade(w, r, player, execution.Command{Action: "cancel", OrderID: r.PathValue("orderId")})
		})
		mount("/v1/sessions/{id}/finish", "POST", func(w http.ResponseWriter, r *http.Request, player string) {
			var input struct{}
			if !decode(w, r, &input) {
				return
			}
			trade(w, r, player, execution.Command{Action: "finish"})
		})
		mount("/v1/sessions/{id}/restart", "POST", func(w http.ResponseWriter, r *http.Request, player string) {
			var input struct{}
			if !decode(w, r, &input) {
				return
			}
			result, replayed, err := store.Start(r.Context(), player, key(r), sessions.StartCommand{RestartSessionID: r.PathValue("id")})
			if err != nil {
				fail(w, r, err)
				return
			}
			w.Header().Set("Idempotency-Replayed", strconv.FormatBool(replayed))
			logger.InfoContext(r.Context(), "session command committed", "request_id", requestID(r.Context()), "session_id", result.Session.ID, "account_id", result.Account.ID, "replayed", replayed, "operation", "restart")
			writeJSON(w, 201, map[string]any{"data": result, "requestId": requestID(r.Context())})
		})
		mount("/v1/sessions/{id}/account", "GET", func(w http.ResponseWriter, r *http.Request, player string) {
			result, err := store.Get(r.Context(), player, r.PathValue("id"))
			if err != nil {
				fail(w, r, err)
				return
			}
			writeJSON(w, 200, map[string]any{"account": result.Account, "requestId": requestID(r.Context())})
		})
		mount("/v1/sessions/{id}/ledger", "GET", func(w http.ResponseWriter, r *http.Request, player string) {
			result, err := store.Ledger(r.Context(), player, r.PathValue("id"))
			if err != nil {
				fail(w, r, err)
				return
			}
			writeJSON(w, 200, map[string]any{"entries": result, "requestId": requestID(r.Context())})
		})
		mount("/v1/sessions/{id}/events", "GET", func(w http.ResponseWriter, r *http.Request, player string) {
			after, limit := int64(0), 100
			valid := true
			for name, values := range r.URL.Query() {
				if len(values) != 1 {
					valid = false
					break
				}
				switch name {
				case "after":
					value, err := strconv.ParseInt(values[0], 10, 64)
					if err != nil {
						valid = false
					}
					after = value
				case "limit":
					value, err := strconv.Atoi(values[0])
					if err != nil {
						valid = false
					}
					limit = value
				default:
					valid = false
				}
			}
			if !valid {
				fail(w, r, sessions.ErrInvalid)
				return
			}
			result, err := store.Events(r.Context(), player, r.PathValue("id"), after, limit)
			if err != nil {
				fail(w, r, err)
				return
			}
			next := after
			if len(result) > 0 {
				next = result[len(result)-1].Sequence
			}
			writeJSON(w, 200, map[string]any{"events": result, "nextAfter": next, "requestId": requestID(r.Context())})
		})
	}
}
