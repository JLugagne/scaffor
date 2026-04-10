package commands

import (
	"github.com/JLugagne/joist/internal/joist/domain/service"
	joistmlcp "github.com/JLugagne/joist/internal/joist/inbound/mcp"
	"github.com/spf13/cobra"
)

func NewMCPCommand(scaffolder service.ScaffolderCommands) *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Start the joist MCP server over stdio",
		Long: `Start joist as an MCP (Model Context Protocol) server.

The server communicates over stdin/stdout using the MCP protocol and exposes
joist's scaffolding tools so that LLM clients can list, inspect, and execute
templates directly.

Tools exposed:
  list_templates   – list all templates in .joist-templates/
  doc_template     – show documentation for a template or command
  execute_template – execute a template command
  lint_template    – lint a template manifest

Configure your MCP client to run: joist mcp`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return joistmlcp.Serve(cmd.Context(), scaffolder)
		},
	}
}
