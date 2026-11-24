package authentication

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDevelopmentAuthenticationBoundary(t *testing.T) {
	token := strings.Repeat("a", 64)
	a := NewDevelopment(token, "configured-player")
	for _, header := range []string{"", token, "Basic " + token, "Bearer wrong", "Bearer " + token + " extra"} {
		if _, ok := a.Authenticate(context.Background(), header); ok {
			t.Errorf("accepted invalid authentication header")
		}
	}
	principal, ok := a.Authenticate(context.Background(), "bearer "+token)
	if !ok || principal.PlayerID != "configured-player" {
		t.Fatal("valid bearer identity rejected")
	}
	request := httptest.NewRequest("GET", "/v1/me", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("X-Player-ID", "attacker-player")
	principal, ok = FromRequest(context.Background(), request, a)
	if !ok || principal.PlayerID != "configured-player" {
		t.Fatal("caller overrode authenticated identity")
	}
	request.Header.Add("Authorization", "Bearer "+token)
	if _, ok := FromRequest(context.Background(), request, a); ok {
		t.Fatal("multiple authorization headers accepted")
	}
}
