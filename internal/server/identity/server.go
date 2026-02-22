package identity

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/MisterGrinvalds/sidequest/internal/server/ui"
	"github.com/golang-jwt/jwt/v5"
)

// Server is a minimal OIDC-compatible identity provider.
type Server struct {
	port         int
	issuer       string
	privateKey   *rsa.PrivateKey
	keyID        string
	tokenTTL     time.Duration
	clients      map[string]string // client_id -> client_secret
	server       *http.Server
	explorerHTML []byte
}

// Config holds identity server configuration.
type Config struct {
	Port     int
	Issuer   string
	TokenTTL time.Duration
}

// New creates a new identity provider server.
func New(cfg Config) (*Server, error) {
	s := &Server{
		port:     cfg.Port,
		issuer:   cfg.Issuer,
		tokenTTL: cfg.TokenTTL,
		keyID:    "sidequest-key-1",
		clients: map[string]string{
			"sidequest-client": "sidequest-secret",
		},
	}

	// Load or generate RSA key.
	if keyPEM := os.Getenv("SIDEQUEST_IDENTITY_SIGNING_KEY"); keyPEM != "" {
		block, _ := pem.Decode([]byte(keyPEM))
		if block == nil {
			return nil, fmt.Errorf("failed to decode PEM signing key")
		}
		key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parsing signing key: %w", err)
		}
		s.privateKey = key
	} else {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return nil, fmt.Errorf("generating RSA key: %w", err)
		}
		s.privateKey = key
	}

	// Load additional clients from env.
	if clientsJSON := os.Getenv("SIDEQUEST_IDENTITY_CLIENTS"); clientsJSON != "" {
		var clients []struct {
			ID     string `json:"id"`
			Secret string `json:"secret"`
		}
		if err := json.Unmarshal([]byte(clientsJSON), &clients); err == nil {
			for _, c := range clients {
				s.clients[c.ID] = c.Secret
			}
		}
	}

	// Pre-render the OIDC explorer page.
	var buf bytes.Buffer
	if err := ui.RenderIdentity(&buf, ui.IdentityExplorerData{
		Port:   cfg.Port,
		Issuer: cfg.Issuer,
	}); err == nil {
		s.explorerHTML = buf.Bytes()
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", s.handleDiscovery)
	mux.HandleFunc("/jwks", s.handleJWKS)
	mux.HandleFunc("/token", s.handleToken)
	mux.HandleFunc("/introspect", s.handleIntrospect)
	mux.HandleFunc("/userinfo", s.handleUserinfo)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})
	mux.Handle("/static/", ui.StaticHandler())
	mux.HandleFunc("/", s.handleRoot)

	s.server = &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	return s, nil
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" && s.explorerHTML != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(s.explorerHTML)
		return
	}
	http.NotFound(w, r)
}

// Port returns the configured port.
func (s *Server) Port() int { return s.port }

// Start begins listening.
func (s *Server) Start() error { return s.server.ListenAndServe() }

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error { return s.server.Shutdown(ctx) }

// IssueToken creates a signed JWT.
func (s *Server) IssueToken(subject, clientID string, scopes []string) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"iss":       s.issuer,
		"sub":       subject,
		"aud":       clientID,
		"exp":       now.Add(s.tokenTTL).Unix(),
		"iat":       now.Unix(),
		"nbf":       now.Unix(),
		"scope":     strings.Join(scopes, " "),
		"token_use": "access",
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = s.keyID

	return token.SignedString(s.privateKey)
}

func (s *Server) handleDiscovery(w http.ResponseWriter, r *http.Request) {
	doc := map[string]interface{}{
		"issuer":                                s.issuer,
		"authorization_endpoint":                s.issuer + "/authorize",
		"token_endpoint":                        s.issuer + "/token",
		"userinfo_endpoint":                     s.issuer + "/userinfo",
		"jwks_uri":                              s.issuer + "/jwks",
		"introspection_endpoint":                s.issuer + "/introspect",
		"response_types_supported":              []string{"code", "token"},
		"grant_types_supported":                 []string{"client_credentials", "authorization_code"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
		"token_endpoint_auth_methods_supported": []string{"client_secret_basic", "client_secret_post"},
		"scopes_supported":                      []string{"openid", "profile", "email"},
	}

	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(doc)
}

func (s *Server) handleJWKS(w http.ResponseWriter, r *http.Request) {
	pub := &s.privateKey.PublicKey

	jwks := map[string]interface{}{
		"keys": []map[string]interface{}{
			{
				"kty": "RSA",
				"use": "sig",
				"kid": s.keyID,
				"alg": "RS256",
				"n":   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(jwks)
}

func (s *Server) handleToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeIdentityError(w, http.StatusMethodNotAllowed, "invalid_request", "POST required")
		return
	}
	r.ParseForm()

	grantType := r.Form.Get("grant_type")
	if grantType != "client_credentials" {
		writeIdentityError(w, http.StatusBadRequest, "unsupported_grant_type", "Only client_credentials is supported")
		return
	}

	// Authenticate client (Basic auth or form post).
	clientID, clientSecret, ok := r.BasicAuth()
	if !ok {
		clientID = r.Form.Get("client_id")
		clientSecret = r.Form.Get("client_secret")
	}

	expectedSecret, exists := s.clients[clientID]
	if !exists || expectedSecret != clientSecret {
		writeIdentityError(w, http.StatusUnauthorized, "invalid_client", "Invalid client credentials")
		return
	}

	scope := r.Form.Get("scope")
	scopes := strings.Fields(scope)
	if len(scopes) == 0 {
		scopes = []string{"openid"}
	}

	token, err := s.IssueToken(clientID, clientID, scopes)
	if err != nil {
		writeIdentityError(w, http.StatusInternalServerError, "server_error", "Failed to issue token")
		return
	}

	resp := map[string]interface{}{
		"access_token": token,
		"token_type":   "Bearer",
		"expires_in":   int(s.tokenTTL.Seconds()),
		"scope":        strings.Join(scopes, " "),
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(resp)
}

func (s *Server) handleIntrospect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeIdentityError(w, http.StatusMethodNotAllowed, "invalid_request", "POST required")
		return
	}
	r.ParseForm()

	tokenStr := r.Form.Get("token")
	if tokenStr == "" {
		writeIdentityError(w, http.StatusBadRequest, "invalid_request", "token parameter required")
		return
	}

	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		return &s.privateKey.PublicKey, nil
	})

	if err != nil || !token.Valid {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"active": false})
		return
	}

	claims := token.Claims.(jwt.MapClaims)
	resp := map[string]interface{}{
		"active":    true,
		"sub":       claims["sub"],
		"iss":       claims["iss"],
		"aud":       claims["aud"],
		"exp":       claims["exp"],
		"iat":       claims["iat"],
		"scope":     claims["scope"],
		"token_use": claims["token_use"],
	}

	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(resp)
}

func (s *Server) handleUserinfo(w http.ResponseWriter, r *http.Request) {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		writeIdentityError(w, http.StatusUnauthorized, "invalid_token", "Bearer token required")
		return
	}

	tokenStr := strings.TrimPrefix(auth, "Bearer ")
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		return &s.privateKey.PublicKey, nil
	})

	if err != nil || !token.Valid {
		writeIdentityError(w, http.StatusUnauthorized, "invalid_token", "Invalid or expired token")
		return
	}

	claims := token.Claims.(jwt.MapClaims)
	resp := map[string]interface{}{
		"sub":   claims["sub"],
		"scope": claims["scope"],
	}

	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(resp)
}

func writeIdentityError(w http.ResponseWriter, status int, errCode, description string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{
		"error":             errCode,
		"error_description": description,
	})
}
