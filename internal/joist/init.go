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
			Use:           "joist",
			Short:         "Template-driven scaffolding for Go projects",
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
