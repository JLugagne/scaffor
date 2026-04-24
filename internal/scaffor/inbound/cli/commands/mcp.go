package commands

import (
	scafformlcp "github.com/JLugagne/scaffor/internal/scaffor/inbound/mcp"
	"github.com/spf13/cobra"
)

func NewMCPCommand(factory ScaffolderFactory) *cobra.Command {
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
  batch_execute    – execute multiple commands in one call (cuts N round-trips to 1)
  lint_template    – lint a template manifest
  test_template    – run a template's test block in a temp directory

Configure your MCP client to run: scaffor mcp`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			scaffolder, err := factory()
			if err != nil {
				return err
			}
			return scafformlcp.Serve(cmd.Context(), scaffolder)
		},
	}
}
