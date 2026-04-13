package commands

import (
	"github.com/JLugagne/scaffor/internal/scaffor/domain/service"
	scafformlcp "github.com/JLugagne/scaffor/internal/scaffor/inbound/mcp"
	"github.com/spf13/cobra"
)

func NewMCPCommand(scaffolder service.ScaffolderCommands) *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Start the scaffor MCP server over stdio",
		Long: `Start scaffor as an MCP (Model Context Protocol) server.

The server communicates over stdin/stdout using the MCP protocol and exposes
scaffor's scaffolding tools so that LLM clients can list, inspect, and execute
templates directly.

Tools exposed:
  list_templates   – list all templates in .scaffor-templates/
  doc_template     – show documentation for a template or command
  execute_template – execute a template command
  lint_template    – lint a template manifest

Configure your MCP client to run: scaffor mcp`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return scafformlcp.Serve(cmd.Context(), scaffolder)
		},
	}
}
