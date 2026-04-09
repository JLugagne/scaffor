package joist

import (
	"context"

	appcommands "github.com/JLugagne/joist/internal/joist/app/commands"
	clicommands "github.com/JLugagne/joist/internal/joist/inbound/cli/commands"
	"github.com/JLugagne/joist/internal/joist/outbound/filesystem"
	"github.com/spf13/cobra"
)

// Runner is a function that executes the CLI logic.
type Runner func(ctx context.Context, args []string) error

func Setup() Runner {
	return func(ctx context.Context, args []string) error {
		fs := filesystem.NewFileSystem()
		scaffolderHandler := appcommands.NewScaffolderHandler(fs)

		rootCmd := &cobra.Command{
			Use:   "joist",
			Short: "Template-driven scaffolding for Go projects",
			Long: `joist is a template-driven scaffolding tool for Go projects.

Templates live in a .joist-templates/ directory at the root of your project.
Each template is a YAML manifest that defines commands, variables, and file
transformations to scaffold new components from existing patterns.

Typical workflow:
  1. List available templates:      joist list-templates
  2. Read a template's docs:        joist doc <template>
  3. See a specific command's vars: joist doc <template> <command>
  4. Execute a template command:    joist execute <template> <command> --set Key=Value

Use "joist <command> --help" for details on any command.`,
			SilenceErrors: true,
			SilenceUsage:  true,
		}

		rootCmd.AddCommand(
			clicommands.NewListTemplatesCommand(scaffolderHandler),
			clicommands.NewDocCommand(scaffolderHandler),
			clicommands.NewExecuteCommand(scaffolderHandler),
			clicommands.NewLintCommand(scaffolderHandler),
		)

		rootCmd.SetArgs(args)
		return rootCmd.ExecuteContext(ctx)
	}
}
