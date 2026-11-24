package identity

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"strings"
)

func New() string { var b [16]byte; _, _ = rand.Read(b[:]); return format(b) }
func Stable(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	var b [16]byte
	copy(b[:], sum[:16])
	return format(b)
}
func format(b [16]byte) string {
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}
