package scaffor

import (
	"context"
	"fmt"
	"os"

	appcommands "github.com/JLugagne/scaffor/internal/scaffor/app/commands"
	"github.com/JLugagne/scaffor/internal/scaffor/config"
	"github.com/JLugagne/scaffor/internal/scaffor/domain/service"
	clicommands "github.com/JLugagne/scaffor/internal/scaffor/inbound/cli/commands"
	"github.com/JLugagne/scaffor/internal/scaffor/outbound/filesystem"
	"github.com/spf13/cobra"
)

// Runner is a function that executes the CLI logic.
type Runner func(ctx context.Context, args []string) error

func Setup() Runner {
	return func(ctx context.Context, args []string) error {
		var (
			templatesDir         string
			ignoreMissingSources bool
		)

		// Build the ScaffolderCommands factory once. It reads the resolved
		// flags at call time, so PersistentPreRunE can populate them before
		// any subcommand runs.
		factory := func() (service.ScaffolderCommands, error) {
			resolver, err := buildResolver(templatesDir, ignoreMissingSources)
			if err != nil {
				return nil, err
			}
			// Collision warnings are informational — surface on stderr so
			// they don't pollute command output parsed by tooling.
			resolver.WriteCollisionWarnings(os.Stderr)
			fs := filesystem.NewFileSystem()
			return appcommands.NewScaffolderHandler(fs, resolver), nil
		}

		rootCmd := &cobra.Command{
			Use:   "scaffor",
			Short: "Template-driven scaffolding for Go projects",
			Long: `scaffor is a template-driven scaffolding tool for Go projects.

Templates live in a .scaffor-templates/ directory at the root of your project,
or in directories declared by your global config at ~/.config/scaffor/config.yml.
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

		rootCmd.PersistentFlags().StringVar(&templatesDir, "templates-dir", "", "Directory containing templates (overrides global config sources)")
		rootCmd.PersistentFlags().BoolVar(&ignoreMissingSources, "ignore-missing-sources", false, "Tolerate missing template sources declared in the global config")

		rootCmd.AddCommand(
			clicommands.NewListTemplatesCommand(factory),
			clicommands.NewDocCommand(factory),
			clicommands.NewExecuteCommand(factory),
			clicommands.NewLintCommand(factory),
			clicommands.NewMCPCommand(factory),
			clicommands.NewTestCommand(factory),
			clicommands.NewInitConfigCommand(),
			clicommands.NewEditConfigCommand(),
			clicommands.NewConfigCommand(func() (*config.Resolver, *config.Config, error) {
				cfg, err := config.Load()
				if err != nil {
					return nil, nil, err
				}
				r, err := buildResolver(templatesDir, true)
				if err != nil {
					return nil, cfg, err
				}
				return r, cfg, nil
			}),
		)

		rootCmd.SetArgs(args)
		return rootCmd.ExecuteContext(ctx)
	}
}

// buildResolver produces a resolver from the current CLI flags and global
// config, applying this precedence: --templates-dir > config sources >
// .scaffor-templates/ (cwd).
func buildResolver(templatesDir string, ignoreMissing bool) (*config.Resolver, error) {
	if templatesDir != "" {
		return config.NewResolverForDir(templatesDir), nil
	}
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}
	if !cfg.Loaded || len(cfg.TemplateSources) == 0 {
		return config.NewResolverForDir(config.FindLocalTemplatesDir()), nil
	}
	sources, err := cfg.ResolveSources()
	if err != nil {
		return nil, err
	}
	// Prepend the local templates dir (walked up from cwd) so local templates
	// always shadow global ones with the same name.
	if localDir := config.FindLocalTemplatesDir(); localDir != "" {
		sources = append([]config.Source{{Path: localDir}}, sources...)
	}
	return config.NewResolverFromSources(sources, ignoreMissing)
}
