package commands

import (
	"fmt"
	"strings"

	"github.com/JLugagne/joist/internal/joist/domain"
	"github.com/JLugagne/joist/internal/joist/domain/service"
	"github.com/spf13/cobra"
)

func NewListTemplatesCommand(scaffolder service.ScaffolderCommands) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all available scaffolding templates",
		Long: `List all templates found in the .joist-templates/ directory.

Each template is a YAML manifest (manifest.yaml) inside its own subdirectory.
The output shows each template's name and description so you can decide which
one fits your use case, then explore it further with "joist doc <template>".`,
		Example: `  # Show all templates in this project
  joist list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			templates, err := scaffolder.ListTemplates(ctx)
			if err != nil {
				return err
			}

			if len(templates) == 0 {
				fmt.Println("No templates found in .joist-templates/")
				return nil
			}

			fmt.Println("Available Templates:")
			for _, tmpl := range templates {
				fmt.Printf("\n- %s:\n", tmpl.Name)
				if len(tmpl.Commands) == 0 && tmpl.Description == "" {
					fmt.Println("    (manifest has errors — run joist lint to see details)")
					continue
				}
				desc := strings.TrimSpace(tmpl.Description)
				if desc != "" {
					for _, line := range strings.Split(desc, "\n") {
						fmt.Printf("    %s\n", line)
					}
				}
			}
			return nil
		},
	}
}

func printCommandDetail(cmd domain.TemplateCommand) {
	fmt.Printf("Command: %s\n", cmd.Command)
	fmt.Printf("  %s\n\n", cmd.Description)
	if len(cmd.Variables) > 0 {
		fmt.Println("  Variables:")
		for _, v := range cmd.Variables {
			fmt.Printf("    --set %s\t%s\n", v.Key, v.Description)
		}
	} else {
		fmt.Println("  Variables: None")
	}
	if len(cmd.PostCommands) > 0 {
		fmt.Println("\n  Post-commands (chains to other template commands):")
		for _, pc := range cmd.PostCommands {
			fmt.Printf("    → %s\n", pc)
		}
	}
	if len(cmd.ShellCommands) > 0 {
		fmt.Println("\n  Shell commands:")
		for _, sc := range cmd.ShellCommands {
			mode := sc.Mode
			if mode == "" {
				mode = "all"
			}
			fmt.Printf("    [%s] $ %s\n", mode, sc.Command)
		}
	}
}

func NewDocCommand(scaffolder service.ScaffolderCommands) *cobra.Command {
	var showAll bool

	cmd := &cobra.Command{
		Use:   "doc <template> [command]",
		Short: "Show documentation for a template or specific command",
		Long: `Show documentation for a template or one of its commands.

When called with only a template name, lists all commands the template exposes
along with their descriptions.

When called with --all/-a, shows full details (variables, post_commands) for
every command in the template at once.

When called with a template name and a command name, shows the full details for
that command: its description, the variables you must supply via --set, and
the post_commands that will run after scaffolding.`,
		Example: `  # List all commands in the "service" template
  joist doc service

  # Show full details for every command at once
  joist doc service --all

  # Show variables required by the "create" command in the "service" template
  joist doc service create`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			tmpl, err := scaffolder.GetTemplate(ctx, args[0])
			if err != nil {
				return err
			}

			if len(args) == 1 && !showAll {
				fmt.Printf("Template: %s\n", tmpl.Name)
				fmt.Printf("%s\n\n", tmpl.Description)
				fmt.Println("Commands:")
				for _, c := range tmpl.Commands {
					fmt.Printf("  %s - %s\n", c.Command, c.Description)
				}
				if len(tmpl.ShellCommands) > 0 {
					fmt.Println("\nShell commands (run after any execute):")
					for _, sc := range tmpl.ShellCommands {
						mode := sc.Mode
						if mode == "" {
							mode = "all"
						}
						fmt.Printf("  [%s] %s\n", mode, sc.Command)
					}
				}
				fmt.Printf("\nRun 'joist doc %s <command>' or 'joist doc %s --all' for details.\n", tmpl.Name, tmpl.Name)
				return nil
			}

			if showAll {
				fmt.Printf("Template: %s\n", tmpl.Name)
				fmt.Printf("%s\n\n", tmpl.Description)
				for i, c := range tmpl.Commands {
					if i > 0 {
						fmt.Println()
					}
					printCommandDetail(c)
				}
				if len(tmpl.ShellCommands) > 0 {
					fmt.Println("\nShell commands (run after any execute):")
					for _, sc := range tmpl.ShellCommands {
						mode := sc.Mode
						if mode == "" {
							mode = "all"
						}
						fmt.Printf("  [%s] %s\n", mode, sc.Command)
					}
				}
				return nil
			}

			commandName := args[1]
			cmdMap := make(map[string]domain.TemplateCommand)
			for _, c := range tmpl.Commands {
				cmdMap[c.Command] = c
			}

			targetCmd, ok := cmdMap[commandName]
			if !ok {
				return fmt.Errorf("command '%s' not found in template '%s'", commandName, tmpl.Name)
			}

			printCommandDetail(targetCmd)
			return nil
		},
	}

	cmd.Flags().BoolVarP(&showAll, "all", "a", false, "Show full details for every command in the template.")
	return cmd
}

func NewExecuteCommand(scaffolder service.ScaffolderCommands) *cobra.Command {
	var sets []string
	var dryRun bool
	var skip bool
	var force bool

	cmd := &cobra.Command{
		Use:   "execute <template> <command> [--set Key=Value ...]",
		Short: "Execute a template command",
		Long: `Execute a command defined in a template manifest.

A template command copies and renders files, substituting any declared variables
with the values you supply via --set.

By default the command refuses to run if any target file already exists. Use
--skip to silently skip existing files or --force to overwrite them.

After files are written, shell_commands defined in the manifest are executed
automatically. Pass --dry-run to print them without executing.`,
		Example: `  # Scaffold a new service named "catalog" using the "service" template
  joist execute service create --set Name=catalog

  # Multiple variables
  joist execute service create --set Name=catalog --set Port=8080

  # Print shell commands without executing them
  joist execute service create --set Name=catalog --dry-run

  # Skip files that already exist
  joist execute service create --set Name=catalog --skip

  # Overwrite files that already exist
  joist execute service create --set Name=catalog --force`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			templateName := args[0]
			commandName := args[1]

			tmpl, err := scaffolder.GetTemplate(ctx, templateName)
			if err != nil {
				return err
			}

			cmdMap := make(map[string]domain.TemplateCommand)
			for _, c := range tmpl.Commands {
				cmdMap[c.Command] = c
			}

			if _, ok := cmdMap[commandName]; !ok {
				return fmt.Errorf("command '%s' not found in template '%s'", commandName, templateName)
			}

			targetCmd := cmdMap[commandName]
			params := make(map[string]string)
			for _, set := range sets {
				parts := strings.SplitN(set, "=", 2)
				if len(parts) == 2 {
					params[parts[0]] = parts[1]
				}
			}

			var missing []domain.TemplateVariable
			for _, v := range targetCmd.Variables {
				if _, ok := params[v.Key]; !ok {
					missing = append(missing, v)
				}
			}

			if len(missing) > 0 {
				fmt.Printf("ERROR: missing required variables for %s/%s:\n\n", templateName, commandName)
				for _, v := range missing {
					fmt.Printf("  --set %s\t%s\n", v.Key, v.Description)
				}
				fmt.Printf("\nUsage: joist execute %s %s", templateName, commandName)
				for _, v := range targetCmd.Variables {
					fmt.Printf(" --set %s=\"value\"", v.Key)
				}
				fmt.Println()
				return fmt.Errorf("missing required variables")
			}

			opts := domain.ExecuteOptions{
				DryRun: dryRun,
				Skip:   skip,
				Force:  force,
			}
			_, err = scaffolder.Execute(ctx, templateName, commandName, params, opts)
			return err
		},
	}

	cmd.Flags().StringSliceVar(&sets, "set", nil, "Set a variable value (format: Key=Value). Repeat for multiple variables.")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print shell commands without executing them.")
	cmd.Flags().BoolVar(&skip, "skip", false, "Skip files that already exist instead of failing.")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite files that already exist instead of failing.")
	return cmd
}

func NewLintCommand(scaffolder service.ScaffolderCommands) *cobra.Command {
	var templateDir string
	var lintAll bool

	cmd := &cobra.Command{
		Use:   "lint [template]",
		Short: "Lint a template manifest for issues",
		Long: `Validate a template manifest and report any issues.

Checks include:
  - Variables declared in commands actually exist
  - post_commands reference commands that exist in the same template
  - Required fields are present and non-empty`,
		Example: `  # Lint the "service" template
  joist lint service

  # Lint a template in a custom directory
  joist lint -d my-templates service

  # Lint all templates
  joist lint --all`,
		Args: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed("all") {
				return cobra.MaximumNArgs(0)(cmd, args)
			}
			return cobra.ExactArgs(1)(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			if lintAll {
				templates, err := scaffolder.ListTemplates(ctx)
				if err != nil {
					return fmt.Errorf("listing templates: %w", err)
				}

				totalIssues := 0
				failedTemplates := 0
				for _, t := range templates {
					errs := scaffolder.Lint(ctx, t.Name, templateDir)
					if len(errs) == 0 {
						fmt.Printf("OK: %s has no issues\n", t.Name)
						continue
					}

					failedTemplates++
					fmt.Printf("LINT ERRORS in %s:\n\n", t.Name)
					for _, e := range errs {
						fmt.Printf("  %s\n", e.Error())
					}
					fmt.Println()
					totalIssues += len(errs)
				}

				if totalIssues > 0 {
					return fmt.Errorf("lint failed: %d issue(s) in %d template(s)", totalIssues, failedTemplates)
				}
				fmt.Printf("\nAll %d template(s) OK\n", len(templates))
				return nil
			}

			templateName := args[0]

			errs := scaffolder.Lint(ctx, templateName, templateDir)
			if len(errs) == 0 {
				fmt.Printf("OK: %s has no issues\n", templateName)
				return nil
			}

			fmt.Printf("LINT ERRORS in %s:\n\n", templateName)
			for _, e := range errs {
				fmt.Printf("  %s\n", e.Error())
			}
			fmt.Printf("\n%d issue(s) found\n", len(errs))
			return fmt.Errorf("lint failed with %d issue(s)", len(errs))
		},
	}

	cmd.Flags().StringVarP(&templateDir, "dir", "d", "", "Directory containing templates (default: .joist-templates)")
	cmd.Flags().BoolVar(&lintAll, "all", false, "Lint all templates in the directory")
	return cmd
}
