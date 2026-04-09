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

func NewDocCommand(scaffolder service.ScaffolderCommands) *cobra.Command {
	return &cobra.Command{
		Use:   "doc <template> [command]",
		Short: "Show documentation for a template or specific command",
		Long: `Show documentation for a template or one of its commands.

When called with only a template name, lists all commands the template exposes
along with their descriptions.

When called with a template name and a command name, shows the full details for
that command: its description, the variables you must supply via --set, and
the post_commands that will run after scaffolding.`,
		Example: `  # List all commands in the "service" template
  joist doc service

  # Show variables required by the "create" command in the "service" template
  joist doc service create`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			tmpl, err := scaffolder.GetTemplate(ctx, args[0])
			if err != nil {
				return err
			}

			if len(args) == 1 {
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
				fmt.Printf("\nRun 'joist doc %s <command>' for details.\n", tmpl.Name)
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

			fmt.Printf("Command: %s\n", targetCmd.Command)
			fmt.Printf("  %s\n\n", targetCmd.Description)

			if len(targetCmd.Variables) > 0 {
				fmt.Println("  Variables:")
				for _, v := range targetCmd.Variables {
					fmt.Printf("    --set %s\t%s\n", v.Key, v.Description)
				}
			} else {
				fmt.Println("  Variables: None")
			}

			if len(targetCmd.PostCommands) > 0 {
				fmt.Println("\n  Post-commands (chains to other template commands):")
				for _, pc := range targetCmd.PostCommands {
					fmt.Printf("    → %s\n", pc)
				}
			}

			return nil
		},
	}
}

func NewExecuteCommand(scaffolder service.ScaffolderCommands) *cobra.Command {
	var sets []string
	var runCommands bool

	cmd := &cobra.Command{
		Use:   "execute <template> <command> [--set Key=Value ...]",
		Short: "Execute a template command",
		Long: `Execute a command defined in a template manifest.

A template command copies and renders files, substituting any declared variables
with the values you supply via --set.

After files are written, post_commands defined in the manifest are displayed
so that you (or your LLM) can run them. Pass --run-commands to execute them
automatically via the shell instead.

Post-commands support two modes:
  all      – run once with {{ .Files }} expanded to all created files (default)
  per-file – run once per created file with {{ .File }} expanded to each path`,
		Example: `  # Scaffold a new service named "catalog" using the "service" template
  joist execute service create --set Name=catalog

  # Multiple variables
  joist execute service create --set Name=catalog --set Port=8080

  # Run post-commands automatically
  joist execute service create --set Name=catalog --run-commands`,
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

			return scaffolder.Execute(ctx, templateName, commandName, params, runCommands)
		},
	}

	cmd.Flags().StringSliceVar(&sets, "set", nil, "Set a variable value (format: Key=Value). Repeat for multiple variables.")
	cmd.Flags().BoolVar(&runCommands, "run-commands", false, "Execute post_commands automatically via the shell instead of printing them.")
	return cmd
}

func NewLintCommand(scaffolder service.ScaffolderCommands) *cobra.Command {
	var templateDir string

	cmd := &cobra.Command{
		Use:   "lint <template>",
		Short: "Lint a template manifest for issues",
		Long: `Validate a template manifest and report any issues.

Checks include:
  - Variables declared in commands actually exist
  - post_commands reference commands that exist in the same template
  - Required fields are present and non-empty`,
		Example: `  # Lint the "service" template
  joist lint service

  # Lint a template in a custom directory
  joist lint -d my-templates service`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
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
	return cmd
}
