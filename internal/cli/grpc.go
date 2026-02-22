package cli

import (
	"context"
	"fmt"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	grpcclient "github.com/MisterGrinvalds/sidequest/internal/client/grpc"
	grpcserver "github.com/MisterGrinvalds/sidequest/internal/server/grpc"
	"github.com/MisterGrinvalds/sidequest/internal/store"
)

func init() {
	rootCmd.AddCommand(grpcCmd)
	grpcCmd.AddCommand(grpcServeCmd)
	grpcCmd.AddCommand(grpcCallCmd)
	grpcCmd.AddCommand(grpcListCmd)
}

var grpcCmd = &cobra.Command{
	Use:   "grpc",
	Short: "gRPC server and client",
	Long:  "Start a gRPC server with CRUD on items, or invoke gRPC methods.",
}

var grpcServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the gRPC server",
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Flags().GetInt("port")
		s := store.New()
		srv := grpcserver.New(port, s)

		fmt.Printf("gRPC server listening on :%d\n", port)
		fmt.Println("Services: sidequest.v1.ItemService (reflection enabled)")

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
			srv.GracefulStop()
		}
		return nil
	},
}

var grpcCallCmd = &cobra.Command{
	Use:   "call <addr> <method>",
	Short: "Invoke a gRPC method",
	Long: `Invoke a gRPC method on the ItemService.

Available methods: GetItem, ListItems, CreateItem, UpdateItem, DeleteItem

Examples:
  sidequest grpc call localhost:9090 CreateItem --data '{"name":"test"}'
  sidequest grpc call localhost:9090 GetItem --data '{"id":"abc123"}'
  sidequest grpc call localhost:9090 ListItems --data '{}'`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		addr := args[0]
		method := args[1]

		plaintext, _ := cmd.Flags().GetBool("plaintext")
		timeout, _ := cmd.Flags().GetDuration("timeout")
		data, _ := cmd.Flags().GetString("data")

		if data == "" {
			data = "{}"
		}

		client, err := grpcclient.Connect(addr, plaintext, timeout)
		if err != nil {
			return err
		}
		defer client.Close()

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		result, err := client.Call(ctx, method, data)
		if err != nil {
			return err
		}

		fmt.Println(result)
		return nil
	},
}

var grpcListCmd = &cobra.Command{
	Use:   "list <addr>",
	Short: "List available gRPC services via reflection",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		addr := args[0]
		plaintext, _ := cmd.Flags().GetBool("plaintext")
		timeout, _ := cmd.Flags().GetDuration("timeout")

		services, err := grpcclient.ListServices(context.Background(), addr, plaintext, timeout)
		if err != nil {
			return err
		}

		fmt.Println("Available services:")
		for _, svc := range services {
			fmt.Printf("  %s\n", svc)
		}
		return nil
	},
}

func init() {
	grpcServeCmd.Flags().Int("port", 9090, "Port to listen on")

	for _, cmd := range []*cobra.Command{grpcCallCmd, grpcListCmd} {
		cmd.Flags().Bool("plaintext", true, "Use plaintext (no TLS)")
		cmd.Flags().Duration("timeout", 30*time.Second, "Request timeout")
	}
	grpcCallCmd.Flags().StringP("data", "d", "{}", "Request data (JSON)")

}
