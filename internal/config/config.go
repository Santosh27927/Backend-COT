// Package config validates process configuration before application startup.
package config

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type Lookup func(string) string

type Database struct {
	URL            string
	MaxConnections int32
	Timeout        time.Duration
}

type Config struct {
	Environment         string
	Service             string
	Address             string
	LogLevel            string
	Database            Database
	DevelopmentToken    string
	DevelopmentPlayerID string
	ShutdownTimeout     time.Duration
}

var playerIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func LoadDatabase(get Lookup) (Database, error) {
	environment := get("COT_ENV")
	if environment != "local" && environment != "test" {
		return Database{}, fmt.Errorf("COT_ENV must explicitly be local or test for this foundation")
	}
	raw := get("COT_DATABASE_URL")
	u, err := url.Parse(raw)
	if err != nil || raw == "" || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Hostname() == "" || u.Path == "" || u.Path == "/" {
		return Database{}, fmt.Errorf("COT_DATABASE_URL must be a PostgreSQL URL with a host and database")
	}
	// Reject malformed driver settings without returning a credential-bearing error.
	if _, err := pgx.ParseConfig(raw); err != nil {
		return Database{}, fmt.Errorf("COT_DATABASE_URL contains invalid PostgreSQL connection settings")
	}
	for _, ch := range u.Hostname() {
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || strings.ContainsRune("._-:", ch)) {
			return Database{}, fmt.Errorf("COT_DATABASE_URL host must be a valid ASCII hostname or IP address")
		}
	}
	maxConnections := int64(4)
	if value := get("COT_DB_MAX_CONNECTIONS"); value != "" {
		maxConnections, err = strconv.ParseInt(value, 10, 32)
		if err != nil || maxConnections < 1 || maxConnections > 32 {
			return Database{}, fmt.Errorf("COT_DB_MAX_CONNECTIONS must be between 1 and 32")
		}
	}
	return Database{URL: raw, MaxConnections: int32(maxConnections), Timeout: 2 * time.Second}, nil
}

func Load(service string, get Lookup) (Config, error) {
	port := ""
	switch service {
	case "game-service":
		port = "8080"
	case "trading-service":
		port = "8081"
	default:
		return Config{}, fmt.Errorf("unsupported service name")
	}
	database, err := LoadDatabase(get)
	if err != nil {
		return Config{}, err
	}
	if get("COT_AUTH_MODE") != "dev-token" {
		return Config{}, fmt.Errorf("COT_AUTH_MODE must explicitly be dev-token; production authentication is not implemented")
	}
	token := get("COT_DEV_AUTH_TOKEN")
	if len(token) < 32 || len(token) > 512 || strings.ContainsAny(token, " \t\r\n") || strings.Contains(strings.ToLower(token), "replace") {
		return Config{}, fmt.Errorf("COT_DEV_AUTH_TOKEN must be a generated token of 32 to 512 characters without whitespace")
	}
	playerID := get("COT_DEV_PLAYER_ID")
	if !playerIDPattern.MatchString(playerID) {
		return Config{}, fmt.Errorf("COT_DEV_PLAYER_ID must contain 1 to 64 letters, digits, underscores, or hyphens")
	}
	address := get("COT_HTTP_ADDR")
	if address == "" {
		address = "127.0.0.1:" + port
	}
	host, portValue, err := net.SplitHostPort(address)
	if err != nil {
		return Config{}, fmt.Errorf("COT_HTTP_ADDR must be a host:port address")
	}
	number, err := strconv.Atoi(portValue)
	if err != nil || number < 1 || number > 65535 {
		return Config{}, fmt.Errorf("COT_HTTP_ADDR port must be between 1 and 65535")
	}
	if value := get("COT_ALLOW_LAN"); value != "" && value != "true" && value != "false" {
		return Config{}, fmt.Errorf("COT_ALLOW_LAN must be true or false")
	}
	ip := net.ParseIP(host)
	isLoopback := host == "localhost" || (ip != nil && ip.IsLoopback())
	if !isLoopback && get("COT_ALLOW_LAN") != "true" {
		return Config{}, fmt.Errorf("non-loopback binding requires COT_ALLOW_LAN=true for trusted development networks")
	}
	level := get("COT_LOG_LEVEL")
	if level == "" {
		level = "info"
	}
	switch level {
	case "debug", "info", "warn", "error":
	default:
		return Config{}, fmt.Errorf("COT_LOG_LEVEL must be debug, info, warn, or error")
	}
	return Config{Environment: get("COT_ENV"), Service: service, Address: address, LogLevel: level, Database: database, DevelopmentToken: token, DevelopmentPlayerID: playerID, ShutdownTimeout: 10 * time.Second}, nil
}
