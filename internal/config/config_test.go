package config

import (
	"os"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	// Clear all env vars.
	for _, key := range []string{
		"SIDEQUEST_LOG_LEVEL", "SIDEQUEST_LOG_FORMAT",
		"SIDEQUEST_HTTP_ENABLED", "SIDEQUEST_HTTP_PORT",
		"SIDEQUEST_REST_ENABLED", "SIDEQUEST_REST_PORT",
		"SIDEQUEST_GRPC_ENABLED", "SIDEQUEST_GRPC_PORT",
		"SIDEQUEST_GRAPHQL_ENABLED", "SIDEQUEST_GRAPHQL_PORT",
		"SIDEQUEST_DNS_ENABLED", "SIDEQUEST_DNS_PORT",
		"SIDEQUEST_IDENTITY_ENABLED", "SIDEQUEST_IDENTITY_PORT",
		"SIDEQUEST_IDENTITY_ISSUER", "SIDEQUEST_IDENTITY_TOKEN_TTL",
	} {
		os.Unsetenv(key)
	}

	c := Load()

	if c.LogLevel != "info" {
		t.Errorf("Expected LogLevel=info, got %q", c.LogLevel)
	}
	if c.LogFormat != "text" {
		t.Errorf("Expected LogFormat=text, got %q", c.LogFormat)
	}
	if !c.HTTPEnabled {
		t.Error("Expected HTTPEnabled=true")
	}
	if c.HTTPPort != 8080 {
		t.Errorf("Expected HTTPPort=8080, got %d", c.HTTPPort)
	}
	if !c.RESTEnabled {
		t.Error("Expected RESTEnabled=true")
	}
	if c.RESTPort != 8081 {
		t.Errorf("Expected RESTPort=8081, got %d", c.RESTPort)
	}
	if !c.GRPCEnabled {
		t.Error("Expected GRPCEnabled=true")
	}
	if c.GRPCPort != 9090 {
		t.Errorf("Expected GRPCPort=9090, got %d", c.GRPCPort)
	}
	if !c.GraphQLEnabled {
		t.Error("Expected GraphQLEnabled=true")
	}
	if c.GraphQLPort != 8082 {
		t.Errorf("Expected GraphQLPort=8082, got %d", c.GraphQLPort)
	}
	if c.DNSEnabled {
		t.Error("Expected DNSEnabled=false")
	}
	if c.DNSPort != 5353 {
		t.Errorf("Expected DNSPort=5353, got %d", c.DNSPort)
	}
	if c.IdentityEnabled {
		t.Error("Expected IdentityEnabled=false")
	}
	if c.IdentityPort != 8443 {
		t.Errorf("Expected IdentityPort=8443, got %d", c.IdentityPort)
	}
	if c.IdentityIssuer != "http://localhost:8443" {
		t.Errorf("Expected IdentityIssuer default, got %q", c.IdentityIssuer)
	}
}

func TestLoadFromEnv(t *testing.T) {
	os.Setenv("SIDEQUEST_LOG_LEVEL", "debug")
	os.Setenv("SIDEQUEST_HTTP_PORT", "9999")
	os.Setenv("SIDEQUEST_DNS_ENABLED", "true")
	os.Setenv("SIDEQUEST_IDENTITY_ENABLED", "yes")
	os.Setenv("SIDEQUEST_IDENTITY_ISSUER", "https://custom.example.com")
	defer func() {
		os.Unsetenv("SIDEQUEST_LOG_LEVEL")
		os.Unsetenv("SIDEQUEST_HTTP_PORT")
		os.Unsetenv("SIDEQUEST_DNS_ENABLED")
		os.Unsetenv("SIDEQUEST_IDENTITY_ENABLED")
		os.Unsetenv("SIDEQUEST_IDENTITY_ISSUER")
	}()

	c := Load()

	if c.LogLevel != "debug" {
		t.Errorf("Expected LogLevel=debug, got %q", c.LogLevel)
	}
	if c.HTTPPort != 9999 {
		t.Errorf("Expected HTTPPort=9999, got %d", c.HTTPPort)
	}
	if !c.DNSEnabled {
		t.Error("Expected DNSEnabled=true")
	}
	if !c.IdentityEnabled {
		t.Error("Expected IdentityEnabled=true")
	}
	if c.IdentityIssuer != "https://custom.example.com" {
		t.Errorf("Expected custom issuer, got %q", c.IdentityIssuer)
	}
}

func TestEnvBoolVariants(t *testing.T) {
	tests := []struct {
		value    string
		expected bool
	}{
		{"true", true},
		{"TRUE", true},
		{"True", true},
		{"1", true},
		{"yes", true},
		{"YES", true},
		{"false", false},
		{"FALSE", false},
		{"0", false},
		{"no", false},
		{"NO", false},
	}

	for _, tt := range tests {
		os.Setenv("SIDEQUEST_HTTP_ENABLED", tt.value)
		c := Load()
		if c.HTTPEnabled != tt.expected {
			t.Errorf("envBool(%q): expected %v, got %v", tt.value, tt.expected, c.HTTPEnabled)
		}
		os.Unsetenv("SIDEQUEST_HTTP_ENABLED")
	}
}

func TestEnvIntInvalid(t *testing.T) {
	os.Setenv("SIDEQUEST_HTTP_PORT", "not-a-number")
	defer os.Unsetenv("SIDEQUEST_HTTP_PORT")

	c := Load()

	// Should fall back to default.
	if c.HTTPPort != 8080 {
		t.Errorf("Expected fallback to 8080, got %d", c.HTTPPort)
	}
}

func TestEnvBoolInvalid(t *testing.T) {
	os.Setenv("SIDEQUEST_HTTP_ENABLED", "maybe")
	defer os.Unsetenv("SIDEQUEST_HTTP_ENABLED")

	c := Load()

	// Should fall back to default (true).
	if !c.HTTPEnabled {
		t.Error("Expected fallback to true for invalid bool")
	}
}
