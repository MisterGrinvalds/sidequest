package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	gqlclient "github.com/MisterGrinvalds/sidequest/internal/client/graphql"
	gqlserver "github.com/MisterGrinvalds/sidequest/internal/server/graphql"
	"github.com/MisterGrinvalds/sidequest/internal/store"
)

func init() {
	rootCmd.AddCommand(graphqlCmd)
	graphqlCmd.AddCommand(graphqlServeCmd)
	graphqlCmd.AddCommand(graphqlQueryCmd)
	graphqlCmd.AddCommand(graphqlMutateCmd)
}

var graphqlCmd = &cobra.Command{
	Use:     "graphql",
	Aliases: []string{"gql"},
	Short:   "GraphQL server and client",
	Long:    "Start a GraphQL server with queries/mutations on items, or execute GraphQL operations.",
}

var graphqlServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the GraphQL server",
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Flags().GetInt("port")
		s := store.New()
		srv, err := gqlserver.New(port, s)
		if err != nil {
			return err
		}

		fmt.Printf("GraphQL server listening on :%d\n", port)
		fmt.Printf("  Query endpoint:  http://localhost:%d/graphql\n", port)
		fmt.Printf("  Playground:      http://localhost:%d/playground\n", port)

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

var graphqlQueryCmd = &cobra.Command{
	Use:   "query <url> <query>",
	Short: "Execute a GraphQL query",
	Long: `Execute a GraphQL query.

Examples:
  sidequest graphql query http://localhost:8082/graphql '{ items { items { id name } } }'
  sidequest graphql query http://localhost:8082/graphql '{ item(id: "abc") { id name version } }'`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return executeGraphQL(cmd, args)
	},
}

var graphqlMutateCmd = &cobra.Command{
	Use:   "mutate <url> <mutation>",
	Short: "Execute a GraphQL mutation",
	Long: `Execute a GraphQL mutation.

Examples:
  sidequest graphql mutate http://localhost:8082/graphql 'mutation { createItem(name: "test") { id name } }'`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return executeGraphQL(cmd, args)
	},
}

func init() {
	graphqlServeCmd.Flags().Int("port", 8082, "Port to listen on")

	for _, cmd := range []*cobra.Command{graphqlQueryCmd, graphqlMutateCmd} {
		cmd.Flags().StringSliceP("header", "H", nil, "Request headers (repeatable)")
		cmd.Flags().String("variables", "{}", "Variables as JSON")
		cmd.Flags().Duration("timeout", 30*time.Second, "Request timeout")
		cmd.Flags().BoolP("verbose", "v", false, "Show full response including errors")
	}
}

func executeGraphQL(cmd *cobra.Command, args []string) error {
	url := args[0]
	query := args[1]

	headerSlice, _ := cmd.Flags().GetStringSlice("header")
	headers := parseHeaders(headerSlice)

	varsStr, _ := cmd.Flags().GetString("variables")
	timeout, _ := cmd.Flags().GetDuration("timeout")
	verbose, _ := cmd.Flags().GetBool("verbose")

	var variables map[string]interface{}
	if varsStr != "{}" && varsStr != "" {
		if err := json.Unmarshal([]byte(varsStr), &variables); err != nil {
			return fmt.Errorf("invalid variables JSON: %w", err)
		}
	}

	resp, err := gqlclient.Do(url, gqlclient.Request{
		Query:     query,
		Variables: variables,
	}, headers, timeout)
	if err != nil {
		return err
	}

	if verbose && len(resp.Errors) > 0 {
		fmt.Println(gqlclient.PrettyPrint(resp))
		return fmt.Errorf("GraphQL errors returned")
	}

	fmt.Println(gqlclient.PrettyPrint(resp))
	return nil
}
