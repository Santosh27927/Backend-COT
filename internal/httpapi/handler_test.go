package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Santosh27927/Backend-COT/internal/authentication"
)

func testHandler(logs *bytes.Buffer, ready Readiness) http.Handler {
	return Handler("game-service", slog.New(slog.NewJSONHandler(logs, nil)), authentication.NewDevelopment(strings.Repeat("a", 64), "local-player"), ready)
}
func TestHealthAndAuthentication(t *testing.T) {
	cases := []struct {
		name, method, path, token string
		ready                     bool
		code                      string
		status                    int
	}{
		{"live independent of database", "GET", "/health/live", "", false, "database_unavailable", 200},
		{"ready", "GET", "/health/ready", "", true, "ready", 200},
		{"database down", "GET", "/health/ready", "", false, "database_unavailable", 503},
		{"schema missing", "GET", "/health/ready", "", false, "migrations_required", 503},
		{"schema mismatch", "GET", "/health/ready", "", false, "migration_mismatch", 503},
		{"missing auth", "GET", "/v1/me", "", true, "ready", 401},
		{"bad auth", "GET", "/v1/me", "wrong", true, "ready", 401},
		{"valid auth", "GET", "/v1/me", strings.Repeat("a", 64), true, "ready", 200},
		{"unknown route", "GET", "/missing", "", true, "ready", 404},
		{"wrong method", "POST", "/v1/me", "", true, "ready", 405},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			handler := testHandler(&logs, func(context.Context) (bool, string) { return tc.ready, tc.code })
			request := httptest.NewRequest(tc.method, tc.path, nil)
			if tc.token != "" {
				request.Header.Set("Authorization", "Bearer "+tc.token)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != tc.status {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if response.Header().Get("Content-Type") != "application/json" {
				t.Fatal("response is not JSON")
			}
			if !requestIDPattern.MatchString(response.Header().Get("X-Request-ID")) {
				t.Fatal("missing diagnostic reference")
			}
			var body map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if tc.path == "/v1/me" && tc.status == 200 {
				principal := body["principal"].(map[string]any)
				if principal["playerId"] != "local-player" {
					t.Fatal("wrong player")
				}
			}
		})
	}
}
func TestDiagnosticsRedactSensitiveInputs(t *testing.T) {
	var logs bytes.Buffer
	handler := testHandler(&logs, func(context.Context) (bool, string) { return true, "ready" })
	request := httptest.NewRequest("GET", "/v1/me?token=query-secret-value", nil)
	token := strings.Repeat("a", 64)
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("X-Player-ID", "spoofed-player")
	request.Header.Set("X-Request-ID", strings.Repeat("b", 32))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Header().Get("X-Request-ID") != strings.Repeat("b", 32) {
		t.Fatal("valid correlation ID was not preserved")
	}
	for _, secret := range []string{token, "query-secret-value", "spoofed-player"} {
		if strings.Contains(logs.String(), secret) {
			t.Fatal("sensitive input appeared in logs")
		}
	}
	if !strings.Contains(logs.String(), strings.Repeat("b", 32)) {
		t.Fatal("request is not correlated")
	}
	request = httptest.NewRequest("GET", "/health/live", nil)
	request.Header.Set("X-Request-ID", "untrusted-id")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Header().Get("X-Request-ID") == "untrusted-id" {
		t.Fatal("invalid request ID trusted")
	}
}
func TestPanicRecoveryDoesNotExposePayload(t *testing.T) {
	var logs bytes.Buffer
	handler := testHandler(&logs, func(context.Context) (bool, string) { panic("secret-panic-payload") })
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("GET", "/health/ready", nil))
	if response.Code != 500 {
		t.Fatalf("status=%d", response.Code)
	}
	if strings.Contains(response.Body.String()+logs.String(), "secret-panic-payload") {
		t.Fatal("panic payload exposed")
	}
}
