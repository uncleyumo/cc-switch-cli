package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"cc-switch-cli/internal/mcp"
	"cc-switch-cli/internal/ui"
)

var (
	mcpAddScope   string
	mcpAddType    string
	mcpAddEnv     []string
	mcpAddHeaders []string
)

var mcpAddCmd = &cobra.Command{
	Use:   "add <name> [-- <command> [args...] | <url>]",
	Short: "Add an MCP server (default scope: local)",
	Long: `Add an MCP server entry.

Stdio (default):
  ccs mcp add myserver -- npx -y @example/mcp-server
  ccs mcp add myserver -e KEY=value -- /path/to/server --flag

HTTP / SSE / WS:
  ccs mcp add -t http myserver https://mcp.example.com/mcp
  ccs mcp add -t http myserver https://mcp.example.com/mcp -H "Authorization: Bearer TOKEN"
  ccs mcp add -t sse  myserver https://mcp.example.com/sse

Default scope is local (projects.<cwd>.mcpServers in ~/.claude.json), matching
claude mcp add. Pass --scope user or --scope project to target a different store.`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		if err := mcp.ValidateName(name); err != nil {
			return err
		}
		scope, err := parseAddScope(mcpAddScope)
		if err != nil {
			return err
		}
		dir, err := projectDirForMCP()
		if err != nil {
			return err
		}
		if _, err := mcp.Get(scope, dir, name); err == nil {
			return fmt.Errorf("mcp server %q already exists in %s scope", name, scope)
		}

		srv, err := buildServerFromFlags(args[1:])
		if err != nil {
			return err
		}
		if err := mcp.Set(scope, dir, name, srv); err != nil {
			return err
		}
		fmt.Println(ui.Success(fmt.Sprintf("added %s (%s, %s)", name, scope, srv.Transport())))
		return nil
	},
}

var mcpAddJSONCmd = &cobra.Command{
	Use:   "add-json <name> <json>",
	Short: "Add an MCP server from a raw JSON object",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		if err := mcp.ValidateName(name); err != nil {
			return err
		}
		scope, err := parseAddScope(mcpAddScope)
		if err != nil {
			return err
		}
		dir, err := projectDirForMCP()
		if err != nil {
			return err
		}
		if _, err := mcp.Get(scope, dir, name); err == nil {
			return fmt.Errorf("mcp server %q already exists in %s scope", name, scope)
		}
		srv, err := mcp.ParseServer([]byte(args[1]))
		if err != nil {
			return err
		}
		if err := mcp.Set(scope, dir, name, srv); err != nil {
			return err
		}
		fmt.Println(ui.Success(fmt.Sprintf("added %s (%s, %s)", name, scope, srv.Transport())))
		return nil
	},
}

func init() {
	for _, c := range []*cobra.Command{mcpAddCmd, mcpAddJSONCmd} {
		c.Flags().StringVar(&mcpAddScope, "scope", "local", "Scope: user, local or project")
	}
	mcpAddCmd.Flags().StringVarP(&mcpAddType, "type", "t", "", "Transport type: stdio, http, sse or ws")
	mcpAddCmd.Flags().StringArrayVarP(&mcpAddEnv, "env", "e", nil, "Environment variable KEY=VALUE (repeatable, stdio)")
	mcpAddCmd.Flags().StringArrayVarP(&mcpAddHeaders, "header", "H", nil, "HTTP header \"Name: value\" (repeatable, http/sse/ws)")
}

func parseAddScope(s string) (mcp.Scope, error) {
	if strings.TrimSpace(s) == "" {
		return mcp.ScopeLocal, nil
	}
	return mcp.ParseScope(s)
}

func buildServerFromFlags(rest []string) (*mcp.Server, error) {
	typ := strings.ToLower(strings.TrimSpace(mcpAddType))
	env, err := parseEnvFlags(mcpAddEnv)
	if err != nil {
		return nil, err
	}
	headers, err := parseHeaderFlags(mcpAddHeaders)
	if err != nil {
		return nil, err
	}

	// Detect whether `--` was used: cobra keeps everything after `--` in args.
	// Without `--`, a single remaining arg is treated as a URL when type is
	// http/sse/ws; otherwise it is an error (stdio needs `-- command`).
	switch typ {
	case "", "stdio":
		if len(rest) == 0 {
			return nil, fmt.Errorf("stdio server needs a command; use: ccs mcp add <name> -- <command> [args...]")
		}
		return &mcp.Server{
			Command: rest[0],
			Args:    rest[1:],
			Env:     env,
		}, nil
	case "http", "sse", "ws":
		if len(rest) != 1 {
			return nil, fmt.Errorf("%s server needs exactly one url argument", typ)
		}
		return &mcp.Server{
			Type:    typ,
			URL:     rest[0],
			Headers: headers,
		}, nil
	default:
		return nil, fmt.Errorf("unknown type %q (want stdio, http, sse or ws)", mcpAddType)
	}
}

func parseEnvFlags(flags []string) (map[string]string, error) {
	if len(flags) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(flags))
	for _, f := range flags {
		k, v, ok := strings.Cut(f, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("invalid --env %q (want KEY=VALUE)", f)
		}
		out[k] = v
	}
	return out, nil
}

func parseHeaderFlags(flags []string) (map[string]string, error) {
	if len(flags) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(flags))
	for _, f := range flags {
		k, v, ok := strings.Cut(f, ":")
		if !ok || strings.TrimSpace(k) == "" {
			return nil, fmt.Errorf("invalid --header %q (want \"Name: value\")", f)
		}
		out[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return out, nil
}
