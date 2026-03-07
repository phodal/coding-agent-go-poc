package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	mcpcore "agent-platform/core/resource/mcp"
	"github.com/spf13/cobra"
)

func New(_ context.Context) *cobra.Command {
	cmd := &cobra.Command{Use: "mcp", Short: "Manage MCP servers"}
	cmd.AddCommand(newAddCommand())
	cmd.AddCommand(newListCommand())
	cmd.AddCommand(newGetCommand())
	cmd.AddCommand(newRemoveCommand())
	return cmd
}

func newAddCommand() *cobra.Command {
	var transport string
	var scope string
	var headers []string
	cmd := &cobra.Command{
		Use:   "add [name] [endpoint]",
		Short: "Add an MCP server",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			manager, err := managerFromWD()
			if err != nil {
				return err
			}
			server := mcpcore.Server{Name: args[0], Endpoint: args[1], Transport: transport, Scope: mcpcore.Scope(scope), Headers: parseHeaders(headers)}
			return manager.Add(server)
		},
	}
	cmd.Flags().StringVarP(&transport, "transport", "t", "stdio", "Transport: stdio, sse, http")
	cmd.Flags().StringVarP(&scope, "scope", "s", string(mcpcore.ScopeLocal), "Scope: local, user, project")
	cmd.Flags().StringArrayVarP(&headers, "header", "H", nil, "Header in key:value form")
	return cmd
}

func newListCommand() *cobra.Command {
	var scope string
	var output string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List MCP servers",
		RunE: func(_ *cobra.Command, _ []string) error {
			manager, err := managerFromWD()
			if err != nil {
				return err
			}
			servers, err := manager.List(mcpcore.Scope(scope))
			if err != nil {
				return err
			}
			if output == "json" {
				content, err := json.MarshalIndent(servers, "", "  ")
				if err != nil {
					return err
				}
				fmt.Println(string(content))
				return nil
			}
			if len(servers) == 0 {
				fmt.Println("no mcp servers found")
				return nil
			}
			for _, server := range servers {
				fmt.Printf("%s\t%s\t%s\n", server.Name, server.Transport, server.Endpoint)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&scope, "scope", "s", string(mcpcore.ScopeLocal), "Scope: local, user, project")
	cmd.Flags().StringVarP(&output, "output", "o", "text", "Output format: text or json")
	return cmd
}

func newGetCommand() *cobra.Command {
	var scope string
	cmd := &cobra.Command{
		Use:   "get [name]",
		Short: "Get an MCP server",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			manager, err := managerFromWD()
			if err != nil {
				return err
			}
			server, err := manager.Get(mcpcore.Scope(scope), args[0])
			if err != nil {
				return err
			}
			content, err := json.MarshalIndent(server, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(content))
			return nil
		},
	}
	cmd.Flags().StringVarP(&scope, "scope", "s", string(mcpcore.ScopeLocal), "Scope: local, user, project")
	return cmd
}

func newRemoveCommand() *cobra.Command {
	var scope string
	cmd := &cobra.Command{
		Use:   "remove [name]",
		Short: "Remove an MCP server",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			manager, err := managerFromWD()
			if err != nil {
				return err
			}
			return manager.Remove(mcpcore.Scope(scope), args[0])
		},
	}
	cmd.Flags().StringVarP(&scope, "scope", "s", string(mcpcore.ScopeLocal), "Scope: local, user, project")
	return cmd
}

func managerFromWD() (*mcpcore.Manager, error) {
	workspace, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return mcpcore.NewManager(workspace, home), nil
}

func parseHeaders(raw []string) map[string]string {
	headers := map[string]string{}
	for _, item := range raw {
		parts := strings.SplitN(item, ":", 2)
		if len(parts) != 2 {
			continue
		}
		headers[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	return headers
}
