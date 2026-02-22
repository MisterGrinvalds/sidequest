package cli

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/spf13/cobra"

	idserver "github.com/MisterGrinvalds/sidequest/internal/server/identity"
)

func init() {
	rootCmd.AddCommand(identityCmd)
	identityCmd.AddCommand(identityServeCmd)
	identityCmd.AddCommand(identityTokenCmd)
	identityCmd.AddCommand(identityValidateCmd)
	identityCmd.AddCommand(identityInspectCmd)
}

var identityCmd = &cobra.Command{
	Use:     "identity",
	Aliases: []string{"id"},
	Short:   "OIDC identity provider and JWT tools",
	Long:    "Run an OIDC-compatible identity provider, issue tokens, validate JWTs against any JWKS endpoint.",
}

var identityServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the OIDC identity provider",
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Flags().GetInt("port")
		issuer, _ := cmd.Flags().GetString("issuer")

		srv, err := idserver.New(idserver.Config{
			Port:     port,
			Issuer:   issuer,
			TokenTTL: 1 * time.Hour,
		})
		if err != nil {
			return err
		}

		fmt.Printf("OIDC Identity Provider listening on :%d\n", port)
		fmt.Printf("  Discovery: %s/.well-known/openid-configuration\n", issuer)
		fmt.Printf("  JWKS:      %s/jwks\n", issuer)
		fmt.Printf("  Token:     %s/token\n", issuer)
		fmt.Println("  Default client: sidequest-client / sidequest-secret")

		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()

		errCh := make(chan error, 1)
		go func() {
			errCh <- srv.Start()
		}()

		select {
		case err := <-errCh:
			if err != nil && err != http.ErrServerClosed {
				return err
			}
		case <-ctx.Done():
			fmt.Println("\nShutting down...")
			shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			srv.Shutdown(shutCtx)
		}
		return nil
	},
}

var identityTokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Request a token from the built-in OIDC provider",
	RunE: func(cmd *cobra.Command, args []string) error {
		url, _ := cmd.Flags().GetString("url")
		clientID, _ := cmd.Flags().GetString("client-id")
		clientSecret, _ := cmd.Flags().GetString("client-secret")
		scope, _ := cmd.Flags().GetString("scope")

		body := fmt.Sprintf("grant_type=client_credentials&scope=%s", scope)
		req, _ := http.NewRequest("POST", url+"/token", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.SetBasicAuth(clientID, clientSecret)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("requesting token: %w", err)
		}
		defer resp.Body.Close()

		respBody, _ := io.ReadAll(resp.Body)

		var indented map[string]interface{}
		json.Unmarshal(respBody, &indented)
		out, _ := json.MarshalIndent(indented, "", "  ")
		fmt.Println(string(out))

		return nil
	},
}

var identityValidateCmd = &cobra.Command{
	Use:   "validate <token>",
	Short: "Validate a JWT against a JWKS endpoint",
	Long: `Validate a JWT token against any JWKS endpoint.

Examples:
  sidequest identity validate <token> --jwks-url http://localhost:8443/jwks
  sidequest identity validate <token> --jwks-url https://login.example.com/jwks`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		tokenStr := args[0]
		jwksURL, _ := cmd.Flags().GetString("jwks-url")
		issuer, _ := cmd.Flags().GetString("issuer")
		audience, _ := cmd.Flags().GetString("audience")

		// Fetch JWKS.
		resp, err := http.Get(jwksURL)
		if err != nil {
			return fmt.Errorf("fetching JWKS: %w", err)
		}
		defer resp.Body.Close()

		var jwks struct {
			Keys []struct {
				Kty string `json:"kty"`
				Kid string `json:"kid"`
				N   string `json:"n"`
				E   string `json:"e"`
				Alg string `json:"alg"`
				Use string `json:"use"`
			} `json:"keys"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
			return fmt.Errorf("parsing JWKS: %w", err)
		}

		// Build key map.
		keyMap := make(map[string]*rsa.PublicKey)
		for _, k := range jwks.Keys {
			if k.Kty != "RSA" {
				continue
			}
			nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
			if err != nil {
				continue
			}
			eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
			if err != nil {
				continue
			}
			n := new(big.Int).SetBytes(nBytes)
			e := int(new(big.Int).SetBytes(eBytes).Int64())
			keyMap[k.Kid] = &rsa.PublicKey{N: n, E: e}
		}

		// Parse options.
		var parserOpts []jwt.ParserOption
		if issuer != "" {
			parserOpts = append(parserOpts, jwt.WithIssuer(issuer))
		}
		if audience != "" {
			parserOpts = append(parserOpts, jwt.WithAudience(audience))
		}

		// Parse and validate.
		token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
			kid, ok := t.Header["kid"].(string)
			if !ok {
				return nil, fmt.Errorf("missing kid in token header")
			}
			key, ok := keyMap[kid]
			if !ok {
				return nil, fmt.Errorf("unknown kid %q", kid)
			}
			return key, nil
		}, parserOpts...)

		if err != nil {
			fmt.Printf("INVALID: %v\n", err)
			return fmt.Errorf("token validation failed")
		}

		if token.Valid {
			fmt.Println("VALID")
			fmt.Println()
			claims, _ := json.MarshalIndent(token.Claims, "", "  ")
			fmt.Println(string(claims))
		}

		return nil
	},
}

var identityInspectCmd = &cobra.Command{
	Use:   "inspect <token>",
	Short: "Decode and display JWT claims without validation",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		tokenStr := args[0]

		// Parse without validation.
		parser := jwt.NewParser(jwt.WithoutClaimsValidation())
		token, _, err := parser.ParseUnverified(tokenStr, jwt.MapClaims{})
		if err != nil {
			return fmt.Errorf("parsing token: %w", err)
		}

		fmt.Println("Header:")
		headerJSON, _ := json.MarshalIndent(token.Header, "", "  ")
		fmt.Println(string(headerJSON))

		fmt.Println("\nClaims:")
		claimsJSON, _ := json.MarshalIndent(token.Claims, "", "  ")
		fmt.Println(string(claimsJSON))

		// Check expiry.
		if claims, ok := token.Claims.(jwt.MapClaims); ok {
			if exp, ok := claims["exp"].(float64); ok {
				expTime := time.Unix(int64(exp), 0)
				if time.Now().After(expTime) {
					fmt.Printf("\nExpiry: EXPIRED (expired %s ago)\n", time.Since(expTime).Round(time.Second))
				} else {
					fmt.Printf("\nExpiry: valid (expires in %s)\n", time.Until(expTime).Round(time.Second))
				}
			}
		}

		return nil
	},
}

func init() {
	identityServeCmd.Flags().Int("port", 8443, "Port to listen on")
	identityServeCmd.Flags().String("issuer", "http://localhost:8443", "Issuer URL")

	identityTokenCmd.Flags().String("url", "http://localhost:8443", "OIDC provider URL")
	identityTokenCmd.Flags().String("client-id", "sidequest-client", "Client ID")
	identityTokenCmd.Flags().String("client-secret", "sidequest-secret", "Client secret")
	identityTokenCmd.Flags().String("scope", "openid", "Requested scopes")

	identityValidateCmd.Flags().String("jwks-url", "", "JWKS endpoint URL (required)")
	identityValidateCmd.Flags().String("issuer", "", "Expected issuer (optional)")
	identityValidateCmd.Flags().String("audience", "", "Expected audience (optional)")
	identityValidateCmd.MarkFlagRequired("jwks-url")
}
