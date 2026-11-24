package config

import (
	"strings"
	"testing"
)

func validEnv() map[string]string {
	return map[string]string{"COT_ENV": "local", "COT_AUTH_MODE": "dev-token", "COT_DATABASE_URL": "postgres://cot:private-password@127.0.0.1:55432/cot?sslmode=disable", "COT_DEV_AUTH_TOKEN": strings.Repeat("a", 64), "COT_DEV_PLAYER_ID": "local-player"}
}
func TestLoadDefaults(t *testing.T) {
	for service, address := range map[string]string{"game-service": "127.0.0.1:8080", "trading-service": "127.0.0.1:8081"} {
		t.Run(service, func(t *testing.T) {
			values := validEnv()
			got, err := Load(service, func(k string) string { return values[k] })
			if err != nil {
				t.Fatal(err)
			}
			if got.Address != address || got.Database.MaxConnections != 4 {
				t.Fatalf("unexpected defaults: address=%s connections=%d", got.Address, got.Database.MaxConnections)
			}
		})
	}
}
func TestRejectUnsafeConfiguration(t *testing.T) {
	cases := []struct{ name, key, value string }{
		{"missing environment", "COT_ENV", ""}, {"production cannot use development auth", "COT_ENV", "production"},
		{"unsupported auth", "COT_AUTH_MODE", "disabled"}, {"missing token", "COT_DEV_AUTH_TOKEN", ""},
		{"short token", "COT_DEV_AUTH_TOKEN", strings.Repeat("a", 31)}, {"long token", "COT_DEV_AUTH_TOKEN", strings.Repeat("a", 513)},
		{"placeholder token", "COT_DEV_AUTH_TOKEN", "replace-this-with-a-generated-secret-token"},
		{"token whitespace", "COT_DEV_AUTH_TOKEN", strings.Repeat("a", 32) + "\n"},
		{"missing database", "COT_DATABASE_URL", ""}, {"bad database scheme", "COT_DATABASE_URL", "https://db/cot"},
		{"invalid database URL redacted", "COT_DATABASE_URL", "postgres://cot:private-password@%bad/cot"},
		{"missing database name", "COT_DATABASE_URL", "postgres://localhost/"},
		{"pool zero", "COT_DB_MAX_CONNECTIONS", "0"}, {"pool oversized", "COT_DB_MAX_CONNECTIONS", "33"}, {"pool not numeric", "COT_DB_MAX_CONNECTIONS", "x"},
		{"invalid address", "COT_HTTP_ADDR", "localhost"}, {"port zero", "COT_HTTP_ADDR", "127.0.0.1:0"},
		{"port too large", "COT_HTTP_ADDR", "127.0.0.1:65536"}, {"LAN requires opt in", "COT_HTTP_ADDR", "0.0.0.0:8080"},
		{"invalid player", "COT_DEV_PLAYER_ID", "player\ninjected"}, {"missing player", "COT_DEV_PLAYER_ID", ""},
		{"invalid log level", "COT_LOG_LEVEL", "trace"},
		{"invalid LAN flag", "COT_ALLOW_LAN", "sometimes"},
		{"invalid database port", "COT_DATABASE_URL", "postgres://cot:private-password@127.0.0.1:99999/cot"},
		{"invalid database TLS mode", "COT_DATABASE_URL", "postgres://cot:private-password@127.0.0.1/cot?sslmode=invalid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			values := validEnv()
			values[tc.key] = tc.value
			_, err := Load("game-service", func(k string) string { return values[k] })
			if err == nil {
				t.Fatal("unsafe configuration accepted")
			}
			if strings.Contains(err.Error(), "private-password") {
				t.Fatal("error exposes database password")
			}
		})
	}
}
func TestAllowedOverrides(t *testing.T) {
	values := validEnv()
	values["COT_HTTP_ADDR"] = "0.0.0.0:9000"
	values["COT_ALLOW_LAN"] = "true"
	values["COT_DB_MAX_CONNECTIONS"] = "2"
	values["COT_LOG_LEVEL"] = "debug"
	got, err := Load("game-service", func(k string) string { return values[k] })
	if err != nil {
		t.Fatal(err)
	}
	if got.Address != "0.0.0.0:9000" || got.Database.MaxConnections != 2 || got.LogLevel != "debug" {
		t.Fatal("valid overrides ignored")
	}
	values["COT_HTTP_ADDR"] = "[::1]:8080"
	delete(values, "COT_ALLOW_LAN")
	if _, err := Load("game-service", func(k string) string { return values[k] }); err != nil {
		t.Fatal(err)
	}
}
func TestUnknownService(t *testing.T) {
	values := validEnv()
	if _, err := Load("arbitrary", func(k string) string { return values[k] }); err == nil {
		t.Fatal("unknown service accepted")
	}
}
