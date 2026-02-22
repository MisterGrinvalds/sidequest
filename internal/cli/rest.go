package cli

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	httpclient "github.com/MisterGrinvalds/sidequest/internal/client/http"
	restserver "github.com/MisterGrinvalds/sidequest/internal/server/rest"
	"github.com/MisterGrinvalds/sidequest/internal/store"
)

func init() {
	rootCmd.AddCommand(restCmd)
	restCmd.AddCommand(restServeCmd)
	restCmd.AddCommand(newRESTClientCmd("get", http.MethodGet, "GET single resource"))
	restCmd.AddCommand(newRESTClientCmd("list", http.MethodGet, "GET collection (list)"))
	restCmd.AddCommand(newRESTClientCmd("create", http.MethodPost, "POST create resource"))
	restCmd.AddCommand(newRESTClientCmd("update", http.MethodPut, "PUT full replace"))
	restCmd.AddCommand(newRESTClientCmd("upsert", http.MethodPatch, "PATCH partial update / upsert"))
	restCmd.AddCommand(newRESTClientCmd("delete", http.MethodDelete, "DELETE resource"))
}

var restCmd = &cobra.Command{
	Use:   "rest",
	Short: "REST API server and client",
	Long:  "Start a REST API server with full CRUD on items, or make REST API requests.",
}

var restServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the REST API server",
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Flags().GetInt("port")
		s := store.New()
		srv := restserver.New(port, s)

		fmt.Printf("REST API server listening on :%d\n", port)
		fmt.Println("Endpoints: GET/POST /api/v1/items, GET/PUT/PATCH/DELETE /api/v1/items/:id")

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
	restServeCmd.Flags().Int("port", 8081, "Port to listen on")
}

func newRESTClientCmd(name, method, description string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   fmt.Sprintf("%s <url>", name),
		Short: description,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			url := args[0]

			headerSlice, _ := cmd.Flags().GetStringSlice("header")
			headers := parseHeaders(headerSlice)
			// Default to JSON content type for write methods.
			if method != http.MethodGet && method != http.MethodDelete {
				if _, ok := headers["Content-Type"]; !ok {
					headers["Content-Type"] = "application/json"
				}
			}

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

	cmd.Flags().StringSliceP("header", "H", nil, "Request headers (repeatable)")
	cmd.Flags().StringP("body", "d", "", "Request body (JSON)")
	cmd.Flags().Duration("timeout", 30*time.Second, "Request timeout")
	cmd.Flags().BoolP("verbose", "v", false, "Show response headers and timing")

	return cmd
}
