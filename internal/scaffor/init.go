package scaffor

import (
	"context"

	appcommands "github.com/JLugagne/scaffor/internal/scaffor/app/commands"
	clicommands "github.com/JLugagne/scaffor/internal/scaffor/inbound/cli/commands"
	"github.com/JLugagne/scaffor/internal/scaffor/outbound/filesystem"
	"github.com/spf13/cobra"
)

// Runner is a function that executes the CLI logic.
type Runner func(ctx context.Context, args []string) error

func Setup() Runner {
	return func(ctx context.Context, args []string) error {
		fs := filesystem.NewFileSystem()
		scaffolderHandler := appcommands.NewScaffolderHandler(fs)

		rootCmd := &cobra.Command{
			Use:   "scaffor",
			Short: "Template-driven scaffolding for Go projects",
			Long: `scaffor is a template-driven scaffolding tool for Go projects.

Templates live in a .scaffor-templates/ directory at the root of your project.
Each template is a YAML manifest that defines commands, variables, and file
transformations to scaffold new components from existing patterns.

Typical workflow:
  1. List available templates:      scaffor list
  2. Read a template's docs:        scaffor doc <template>
  3. See a specific command's vars: scaffor doc <template> <command>
  4. Execute a template command:    scaffor execute <template> <command> --set Key=Value

Use "scaffor <command> --help" for details on any command.`,
			SilenceErrors: true,
			SilenceUsage:  true,
		}

		rootCmd.AddCommand(
			clicommands.NewListTemplatesCommand(scaffolderHandler),
			clicommands.NewDocCommand(scaffolderHandler),
			clicommands.NewExecuteCommand(scaffolderHandler),
			clicommands.NewLintCommand(scaffolderHandler),
			clicommands.NewMCPCommand(scaffolderHandler),
			clicommands.NewTestCommand(scaffolderHandler),
		)

		rootCmd.SetArgs(args)
		return rootCmd.ExecuteContext(ctx)
	}
}
