package cli

import (
	"context"
	"fmt"
	"net/http"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/MisterGrinvalds/sidequest/internal/config"
	dnsserver "github.com/MisterGrinvalds/sidequest/internal/server/dns"
	gqlserver "github.com/MisterGrinvalds/sidequest/internal/server/graphql"
	grpcserver "github.com/MisterGrinvalds/sidequest/internal/server/grpc"
	httpserver "github.com/MisterGrinvalds/sidequest/internal/server/http"
	idserver "github.com/MisterGrinvalds/sidequest/internal/server/identity"
	restserver "github.com/MisterGrinvalds/sidequest/internal/server/rest"
	"github.com/MisterGrinvalds/sidequest/internal/store"
)

func init() {
	rootCmd.AddCommand(serveCmd)
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start all enabled servers",
	Long: `Start all enabled servers concurrently based on environment variable configuration.

Enable/disable servers via environment variables:
  SIDEQUEST_HTTP_ENABLED=true       (port: SIDEQUEST_HTTP_PORT, default 8080)
  SIDEQUEST_REST_ENABLED=true       (port: SIDEQUEST_REST_PORT, default 8081)
  SIDEQUEST_GRPC_ENABLED=true       (port: SIDEQUEST_GRPC_PORT, default 9090)
  SIDEQUEST_GRAPHQL_ENABLED=true    (port: SIDEQUEST_GRAPHQL_PORT, default 8082)
  SIDEQUEST_DNS_ENABLED=false       (port: SIDEQUEST_DNS_PORT, default 5353)
  SIDEQUEST_IDENTITY_ENABLED=false  (port: SIDEQUEST_IDENTITY_PORT, default 8443)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		c := config.Load()
		s := store.New()

		fmt.Println("Sidequest - Multi-Tool Container")
		fmt.Println(strings.Repeat("=", 50))
		fmt.Println()

		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()

		var wg sync.WaitGroup
		errCh := make(chan error, 10)

		if c.HTTPEnabled {
			srv := httpserver.New(c.HTTPPort)
			fmt.Printf("  HTTP echo server   :%d\n", c.HTTPPort)
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := srv.Start(); err != nil && err != http.ErrServerClosed {
					errCh <- fmt.Errorf("HTTP server: %w", err)
				}
			}()
			go func() {
				<-ctx.Done()
				shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				srv.Shutdown(shutCtx)
			}()
		}

		if c.RESTEnabled {
			srv := restserver.New(c.RESTPort, s)
			fmt.Printf("  REST API server    :%d\n", c.RESTPort)
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := srv.Start(); err != nil && err != http.ErrServerClosed {
					errCh <- fmt.Errorf("REST server: %w", err)
				}
			}()
			go func() {
				<-ctx.Done()
				shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				srv.Shutdown(shutCtx)
			}()
		}

		if c.GRPCEnabled {
			srv := grpcserver.New(c.GRPCPort, s)
			fmt.Printf("  gRPC server        :%d\n", c.GRPCPort)
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := srv.Start(); err != nil {
					errCh <- fmt.Errorf("gRPC server: %w", err)
				}
			}()
			go func() {
				<-ctx.Done()
				srv.GracefulStop()
			}()
		}

		if c.GraphQLEnabled {
			srv, err := gqlserver.New(c.GraphQLPort, s)
			if err != nil {
				return fmt.Errorf("creating GraphQL server: %w", err)
			}
			fmt.Printf("  GraphQL server     :%d\n", c.GraphQLPort)
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := srv.Start(); err != nil && err != http.ErrServerClosed {
					errCh <- fmt.Errorf("GraphQL server: %w", err)
				}
			}()
			go func() {
				<-ctx.Done()
				shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				srv.Shutdown(shutCtx)
			}()
		}

		if c.DNSEnabled {
			srv := dnsserver.New(c.DNSPort)
			fmt.Printf("  DNS server         :%d\n", c.DNSPort)
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := srv.Start(); err != nil {
					errCh <- fmt.Errorf("DNS server: %w", err)
				}
			}()
			go func() {
				<-ctx.Done()
				srv.Shutdown()
			}()
		}

		if c.IdentityEnabled {
			srv, err := idserver.New(idserver.Config{
				Port:     c.IdentityPort,
				Issuer:   c.IdentityIssuer,
				TokenTTL: 1 * time.Hour,
			})
			if err != nil {
				return fmt.Errorf("creating identity server: %w", err)
			}
			fmt.Printf("  OIDC provider      :%d\n", c.IdentityPort)
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := srv.Start(); err != nil && err != http.ErrServerClosed {
					errCh <- fmt.Errorf("identity server: %w", err)
				}
			}()
			go func() {
				<-ctx.Done()
				shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				srv.Shutdown(shutCtx)
			}()
		}

		fmt.Println()
		fmt.Println("All servers started. Press Ctrl+C to stop.")

		// Wait for either an error or cancellation.
		select {
		case err := <-errCh:
			stop()
			wg.Wait()
			return err
		case <-ctx.Done():
			fmt.Println("\nShutting down all servers...")
			wg.Wait()
			fmt.Println("All servers stopped.")
		}

		return nil
	},
}
