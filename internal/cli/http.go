package cli

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	httpclient "github.com/MisterGrinvalds/sidequest/internal/client/http"
	httpserver "github.com/MisterGrinvalds/sidequest/internal/server/http"
)

func init() {
	rootCmd.AddCommand(httpCmd)
	httpCmd.AddCommand(httpServeCmd)
	httpCmd.AddCommand(newHTTPMethodCmd("get", http.MethodGet))
	httpCmd.AddCommand(newHTTPMethodCmd("post", http.MethodPost))
	httpCmd.AddCommand(newHTTPMethodCmd("put", http.MethodPut))
	httpCmd.AddCommand(newHTTPMethodCmd("delete", http.MethodDelete))
}

var httpCmd = &cobra.Command{
	Use:   "http",
	Short: "HTTP echo server and client",
	Long:  "Start an HTTP echo server or make HTTP requests.",
}

var httpServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the HTTP echo server",
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Flags().GetInt("port")
		srv := httpserver.New(port)

		fmt.Printf("HTTP echo server listening on :%d\n", port)
		fmt.Println("Endpoints: /echo, /headers, /ip, /delay/:s, /status/:code, /health, /ready")

		// Graceful shutdown on signal.
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

func init() {
	httpServeCmd.Flags().Int("port", 8080, "Port to listen on")
}

func newHTTPMethodCmd(name, method string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   fmt.Sprintf("%s <url>", name),
		Short: fmt.Sprintf("Make an HTTP %s request", method),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			url := args[0]

			headerSlice, _ := cmd.Flags().GetStringSlice("header")
			headers := parseHeaders(headerSlice)

			body, _ := cmd.Flags().GetString("body")
			timeout, _ := cmd.Flags().GetDuration("timeout")
			verbose, _ := cmd.Flags().GetBool("verbose")

			resp, err := httpclient.Do(httpclient.Request{
				Method:  method,
				URL:     url,
				Headers: headers,
				Body:    body,
				Timeout: timeout,
				Verbose: verbose,
			})
			if err != nil {
				return err
			}

			httpclient.PrintResponse(resp, verbose)

			if resp.StatusCode >= 400 {
				os.Exit(1)
			}
			return nil
		},
	}

	cmd.Flags().StringSliceP("header", "H", nil, "Request headers (repeatable, e.g. -H 'Content-Type: application/json')")
	cmd.Flags().StringP("body", "d", "", "Request body")
	cmd.Flags().Duration("timeout", 30*time.Second, "Request timeout")
	cmd.Flags().BoolP("verbose", "v", false, "Show response headers and timing")

	return cmd
}

func parseHeaders(raw []string) map[string]string {
	headers := make(map[string]string, len(raw))
	for _, h := range raw {
		parts := strings.SplitN(h, ":", 2)
		if len(parts) == 2 {
			headers[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return headers
}
