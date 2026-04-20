package commands

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/JLugagne/scaffor/internal/scaffor/config"
	"github.com/spf13/cobra"
)

// ConfigContextFactory returns the currently active resolver plus the
// loaded Config so `scaffor config` can introspect both.
type ConfigContextFactory func() (*config.Resolver, *config.Config, error)

// NewInitConfigCommand creates the `scaffor init-config` command, which
// writes a commented example config at ~/.config/scaffor/config.yml.
func NewInitConfigCommand() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "init-config",
		Short: "Create a commented example global config file",
		Long: `Create ~/.config/scaffor/config.yml (or $XDG_CONFIG_HOME/scaffor/config.yml)
with a commented example. The file declares no active template_sources by
default, so scaffor behavior is unchanged until you edit it.

Pass --force to overwrite an existing file.`,
		Example: `  # Create the config file if it doesn't exist
  scaffor init-config

  # Overwrite an existing config file
  scaffor init-config --force`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := config.ResolvePath()
			if err != nil {
				return err
			}
			if _, err := os.Stat(path); err == nil && !force {
				return fmt.Errorf("%s already exists (use --force to overwrite)", path)
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return fmt.Errorf("creating config directory: %w", err)
			}
			if err := os.WriteFile(path, []byte(config.ExampleConfigContents()), 0o644); err != nil {
				return fmt.Errorf("writing config: %w", err)
			}
			fmt.Printf("Wrote example config to %s\n", path)
			fmt.Println("Edit it to add template_sources entries, then run `scaffor config` to verify.")
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite an existing config file")
	return cmd
}

// NewEditConfigCommand creates the `scaffor edit-config` command, which opens
// the global config file in $EDITOR (falling back to $VISUAL). The file is
// created with the commented example if it doesn't already exist so the
// editor always has something to open.
func NewEditConfigCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "edit-config",
		Short: "Open the global config file in $EDITOR",
		Long: `Open ~/.config/scaffor/config.yml (or $XDG_CONFIG_HOME/scaffor/config.yml)
in $EDITOR (falling back to $VISUAL). If the file doesn't exist yet it is
created with the same commented example that init-config writes.

$EDITOR may include arguments (e.g. "code --wait", "emacs -nw").`,
		Example: `  # Open the config in the editor configured via $EDITOR
  EDITOR=vim scaffor edit-config`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := config.ResolvePath()
			if err != nil {
				return err
			}
			if _, err := os.Stat(path); os.IsNotExist(err) {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					return fmt.Errorf("creating config directory: %w", err)
				}
				if err := os.WriteFile(path, []byte(config.ExampleConfigContents()), 0o644); err != nil {
					return fmt.Errorf("writing config: %w", err)
				}
			}

			editor := os.Getenv("EDITOR")
			if editor == "" {
				editor = os.Getenv("VISUAL")
			}
			if editor == "" {
				return fmt.Errorf("neither $EDITOR nor $VISUAL is set; export one (e.g. EDITOR=vim) and retry")
			}

			// Support editor strings with arguments like "code --wait".
			parts := strings.Fields(editor)
			args := append(parts[1:], path)
			editorCmd := exec.CommandContext(cmd.Context(), parts[0], args...)
			editorCmd.Stdin = os.Stdin
			editorCmd.Stdout = os.Stdout
			editorCmd.Stderr = os.Stderr
			return editorCmd.Run()
		},
	}
}

// NewConfigCommand creates the `scaffor config` command, which prints the
// currently active config: file path, resolved sources, templates per source,
// collisions, and missing sources.
func NewConfigCommand(factory ConfigContextFactory) *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "Print the currently active scaffor configuration",
		Long: `Print scaffor's currently active configuration: the path of the config
file (if any), the resolved template sources, the templates discovered in each
source, any collisions (templates that appear in multiple sources), and any
missing source directories.

Use this to verify that your global config is being picked up as expected
before running list / doc / execute.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resolver, cfg, err := factory()
			if err != nil {
				return err
			}

			if cfg != nil {
				if cfg.Loaded {
					fmt.Printf("Config file: %s\n\n", cfg.Path)
				} else {
					fmt.Printf("Config file: %s (not found)\n\n", cfg.Path)
				}
			}

			sources := resolver.Sources()
			if len(sources) == 0 {
				fmt.Println("Template sources: none")
				return nil
			}

			fmt.Println("Template sources:")
			for i, src := range sources {
				line := fmt.Sprintf("  [%d] %s", i+1, src.Path)
				if src.Description != "" {
					line += fmt.Sprintf(" (%s)", src.Description)
				}
				fmt.Println(line)
				if len(src.Templates) == 0 {
					fmt.Println("      (no templates)")
					continue
				}
				for _, name := range src.Templates {
					fmt.Printf("      - %s\n", name)
				}
			}

			collisions := resolver.Collisions()
			fmt.Println()
			if len(collisions) == 0 {
				fmt.Println("No collisions.")
			} else {
				fmt.Println("Collisions:")
				for name, shadowed := range collisions {
					fmt.Printf("  %s also in: %v\n", name, shadowed)
				}
			}

			missing := resolver.Missing()
			if len(missing) == 0 {
				fmt.Println("No missing sources.")
			} else {
				fmt.Println("Missing sources:")
				for _, p := range missing {
					fmt.Printf("  %s\n", p)
				}
			}
			return nil
		},
	}
}
