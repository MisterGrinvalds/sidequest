package config

import (
	"os"
	"strconv"
	"strings"
)

// Config holds all sidequest configuration, loaded from environment variables.
type Config struct {
	LogLevel  string
	LogFormat string

	// Server enable flags
	HTTPEnabled     bool
	RESTEnabled     bool
	GRPCEnabled     bool
	GraphQLEnabled  bool
	DNSEnabled      bool
	IdentityEnabled bool

	// Server ports
	HTTPPort     int
	RESTPort     int
	GRPCPort     int
	GraphQLPort  int
	DNSPort      int
	IdentityPort int

	// Identity provider settings
	IdentityIssuer   string
	IdentityTokenTTL string
}

// Load reads configuration from environment variables with sensible defaults.
func Load() *Config {
	return &Config{
		LogLevel:  envStr("SIDEQUEST_LOG_LEVEL", "info"),
		LogFormat: envStr("SIDEQUEST_LOG_FORMAT", "text"),

		HTTPEnabled:     envBool("SIDEQUEST_HTTP_ENABLED", true),
		RESTEnabled:     envBool("SIDEQUEST_REST_ENABLED", true),
		GRPCEnabled:     envBool("SIDEQUEST_GRPC_ENABLED", true),
		GraphQLEnabled:  envBool("SIDEQUEST_GRAPHQL_ENABLED", true),
		DNSEnabled:      envBool("SIDEQUEST_DNS_ENABLED", false),
		IdentityEnabled: envBool("SIDEQUEST_IDENTITY_ENABLED", false),

		HTTPPort:     envInt("SIDEQUEST_HTTP_PORT", 8080),
		RESTPort:     envInt("SIDEQUEST_REST_PORT", 8081),
		GRPCPort:     envInt("SIDEQUEST_GRPC_PORT", 9090),
		GraphQLPort:  envInt("SIDEQUEST_GRAPHQL_PORT", 8082),
		DNSPort:      envInt("SIDEQUEST_DNS_PORT", 5353),
		IdentityPort: envInt("SIDEQUEST_IDENTITY_PORT", 8443),

		IdentityIssuer:   envStr("SIDEQUEST_IDENTITY_ISSUER", "http://localhost:8443"),
		IdentityTokenTTL: envStr("SIDEQUEST_IDENTITY_TOKEN_TTL", "1h"),
	}
}

func envStr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	switch strings.ToLower(v) {
	case "true", "1", "yes":
		return true
	case "false", "0", "no":
		return false
	default:
		return fallback
	}
}

func envInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
