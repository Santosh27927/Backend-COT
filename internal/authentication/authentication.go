// Package authentication provides the phase-one local-only identity boundary.
package authentication

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"
)

type Principal struct {
	PlayerID string `json:"playerId"`
}
type Authenticator interface {
	Authenticate(context.Context, string) (Principal, bool)
}
type Development struct {
	tokenHash [32]byte
	playerID  string
}

func NewDevelopment(token, playerID string) *Development {
	return &Development{tokenHash: sha256.Sum256([]byte(token)), playerID: playerID}
}

func (a *Development) Authenticate(_ context.Context, header string) (Principal, bool) {
	fields := strings.Fields(header)
	if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") {
		return Principal{}, false
	}
	digest := sha256.Sum256([]byte(fields[1]))
	if subtle.ConstantTimeCompare(digest[:], a.tokenHash[:]) != 1 {
		return Principal{}, false
	}
	return Principal{PlayerID: a.playerID}, true
}

// FromRequest deliberately ignores caller-supplied identity headers and bodies.
func FromRequest(ctx context.Context, r *http.Request, a Authenticator) (Principal, bool) {
	if len(r.Header.Values("Authorization")) != 1 {
		return Principal{}, false
	}
	return a.Authenticate(ctx, r.Header.Get("Authorization"))
}
