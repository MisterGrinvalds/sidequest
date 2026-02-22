package identity

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv, err := New(Config{
		Port:     0,
		Issuer:   "http://test-issuer",
		TokenTTL: 1 * time.Hour,
	})
	if err != nil {
		t.Fatalf("creating server: %v", err)
	}
	return httptest.NewServer(srv.server.Handler)
}

func TestOIDCDiscovery(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/.well-known/openid-configuration")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("Expected 200, got %d", resp.StatusCode)
	}

	var doc map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&doc)

	if doc["issuer"] != "http://test-issuer" {
		t.Errorf("Expected issuer http://test-issuer, got %v", doc["issuer"])
	}
	if doc["jwks_uri"] != "http://test-issuer/jwks" {
		t.Errorf("Expected jwks_uri http://test-issuer/jwks, got %v", doc["jwks_uri"])
	}
	if doc["token_endpoint"] != "http://test-issuer/token" {
		t.Errorf("Expected token_endpoint, got %v", doc["token_endpoint"])
	}

	// Check required OIDC fields.
	requiredFields := []string{"issuer", "authorization_endpoint", "token_endpoint", "jwks_uri",
		"response_types_supported", "subject_types_supported", "id_token_signing_alg_values_supported"}
	for _, field := range requiredFields {
		if doc[field] == nil {
			t.Errorf("Missing required OIDC field: %s", field)
		}
	}
}

func TestJWKSEndpoint(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/jwks")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("Expected 200, got %d", resp.StatusCode)
	}

	// Check Cache-Control.
	cc := resp.Header.Get("Cache-Control")
	if !strings.Contains(cc, "max-age") {
		t.Errorf("Expected Cache-Control with max-age, got %q", cc)
	}

	var jwks struct {
		Keys []struct {
			Kty string `json:"kty"`
			Use string `json:"use"`
			Kid string `json:"kid"`
			Alg string `json:"alg"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	json.NewDecoder(resp.Body).Decode(&jwks)

	if len(jwks.Keys) != 1 {
		t.Fatalf("Expected 1 key, got %d", len(jwks.Keys))
	}

	key := jwks.Keys[0]
	if key.Kty != "RSA" {
		t.Errorf("Expected kty=RSA, got %q", key.Kty)
	}
	if key.Use != "sig" {
		t.Errorf("Expected use=sig, got %q", key.Use)
	}
	if key.Alg != "RS256" {
		t.Errorf("Expected alg=RS256, got %q", key.Alg)
	}
	if key.Kid == "" {
		t.Error("Expected non-empty kid")
	}
	if key.N == "" || key.E == "" {
		t.Error("Expected non-empty N and E values")
	}
}

func TestTokenEndpointClientCredentials(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	// Request with Basic auth.
	req, _ := http.NewRequest("POST", ts.URL+"/token",
		strings.NewReader("grant_type=client_credentials&scope=openid"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth("sidequest-client", "sidequest-secret")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected 200, got %d: %s", resp.StatusCode, string(body))
	}

	// Check Cache-Control: no-store.
	if cc := resp.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Expected Cache-Control: no-store, got %q", cc)
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
		Scope       string `json:"scope"`
	}
	json.NewDecoder(resp.Body).Decode(&tokenResp)

	if tokenResp.AccessToken == "" {
		t.Error("Expected non-empty access_token")
	}
	if tokenResp.TokenType != "Bearer" {
		t.Errorf("Expected token_type=Bearer, got %q", tokenResp.TokenType)
	}
	if tokenResp.ExpiresIn != 3600 {
		t.Errorf("Expected expires_in=3600, got %d", tokenResp.ExpiresIn)
	}
	if tokenResp.Scope != "openid" {
		t.Errorf("Expected scope=openid, got %q", tokenResp.Scope)
	}
}

func TestTokenEndpointFormPost(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, _ := http.Post(ts.URL+"/token", "application/x-www-form-urlencoded",
		strings.NewReader("grant_type=client_credentials&client_id=sidequest-client&client_secret=sidequest-secret"))
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("Expected 200 with form post auth, got %d", resp.StatusCode)
	}
}

func TestTokenEndpointInvalidClient(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	req, _ := http.NewRequest("POST", ts.URL+"/token",
		strings.NewReader("grant_type=client_credentials"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth("bad-client", "bad-secret")

	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()

	if resp.StatusCode != 401 {
		t.Errorf("Expected 401, got %d", resp.StatusCode)
	}

	var errResp struct {
		Error string `json:"error"`
	}
	json.NewDecoder(resp.Body).Decode(&errResp)
	if errResp.Error != "invalid_client" {
		t.Errorf("Expected error=invalid_client, got %q", errResp.Error)
	}
}

func TestTokenEndpointUnsupportedGrant(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	req, _ := http.NewRequest("POST", ts.URL+"/token",
		strings.NewReader("grant_type=authorization_code"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth("sidequest-client", "sidequest-secret")

	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()

	if resp.StatusCode != 400 {
		t.Errorf("Expected 400, got %d", resp.StatusCode)
	}
}

func TestTokenEndpointMethodNotAllowed(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, _ := http.Get(ts.URL + "/token")
	defer resp.Body.Close()

	if resp.StatusCode != 405 {
		t.Errorf("Expected 405, got %d", resp.StatusCode)
	}
}

func TestIntrospectEndpoint(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	// First get a token.
	req, _ := http.NewRequest("POST", ts.URL+"/token",
		strings.NewReader("grant_type=client_credentials&scope=openid"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth("sidequest-client", "sidequest-secret")

	tokenResp, _ := http.DefaultClient.Do(req)
	var tr struct {
		AccessToken string `json:"access_token"`
	}
	json.NewDecoder(tokenResp.Body).Decode(&tr)
	tokenResp.Body.Close()

	// Introspect it.
	resp, _ := http.Post(ts.URL+"/introspect", "application/x-www-form-urlencoded",
		strings.NewReader("token="+tr.AccessToken))
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	if result["active"] != true {
		t.Error("Expected active=true")
	}
	if result["iss"] != "http://test-issuer" {
		t.Errorf("Expected iss=http://test-issuer, got %v", result["iss"])
	}
}

func TestIntrospectInvalidToken(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, _ := http.Post(ts.URL+"/introspect", "application/x-www-form-urlencoded",
		strings.NewReader("token=invalid.jwt.token"))
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	if result["active"] != false {
		t.Error("Expected active=false for invalid token")
	}
}

func TestUserinfoEndpoint(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	// Get a token.
	tokenReq, _ := http.NewRequest("POST", ts.URL+"/token",
		strings.NewReader("grant_type=client_credentials"))
	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tokenReq.SetBasicAuth("sidequest-client", "sidequest-secret")

	tokenResp, _ := http.DefaultClient.Do(tokenReq)
	var tr struct {
		AccessToken string `json:"access_token"`
	}
	json.NewDecoder(tokenResp.Body).Decode(&tr)
	tokenResp.Body.Close()

	// Call userinfo.
	req, _ := http.NewRequest("GET", ts.URL+"/userinfo", nil)
	req.Header.Set("Authorization", "Bearer "+tr.AccessToken)
	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("Expected 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	if result["sub"] == nil {
		t.Error("Expected sub claim in userinfo")
	}
}

func TestUserinfoNoBearer(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, _ := http.Get(ts.URL + "/userinfo")
	defer resp.Body.Close()

	if resp.StatusCode != 401 {
		t.Errorf("Expected 401 without Bearer, got %d", resp.StatusCode)
	}
}

func TestHealthEndpoint(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, _ := http.Get(ts.URL + "/health")
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}
}

func TestIssueToken(t *testing.T) {
	srv, err := New(Config{
		Port:     0,
		Issuer:   "http://test",
		TokenTTL: 1 * time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}

	token, err := srv.IssueToken("test-subject", "test-client", []string{"openid", "profile"})
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	if token == "" {
		t.Error("Expected non-empty token")
	}

	// Token should have 3 parts.
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Errorf("Expected 3 JWT parts, got %d", len(parts))
	}
}
