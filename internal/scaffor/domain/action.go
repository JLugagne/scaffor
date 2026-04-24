package domain

import "fmt"

type Template struct {
	Name          string            `yaml:"name"`
	Description   string            `yaml:"description"`
	Commands      []TemplateCommand `yaml:"commands"`
	ShellCommands []ShellCommand    `yaml:"shell_commands"`
	Test          []TestStep        `yaml:"test"`
	Validate      []string          `yaml:"validate"`

	// Source is the directory this template was loaded from (e.g.
	// ".scaffor-templates" or "/home/me/work/scaffor-templates"). Set by the
	// loader, not from YAML. Used to show provenance in `scaffor list`.
	Source string `yaml:"-"`
}

type TemplateCommand struct {
	Command       string             `yaml:"command"`
	Description   string             `yaml:"description"`
	Variables     []TemplateVariable `yaml:"variables"`
	Files         []TemplateFile     `yaml:"files"`
	PostCommands  []string           `yaml:"post_commands"`
	ShellCommands []ShellCommand     `yaml:"shell_commands"`
	Hint          string             `yaml:"hint"`
}

type TemplateVariable struct {
	Key         string `yaml:"key"`
	Description string `yaml:"description"`
}

type TemplateFile struct {
	Source      string `yaml:"source"`
	Destination string `yaml:"destination"`
	OnConflict  string `yaml:"on_conflict"` // "default", "skip", or "force"; empty means "default"
}

// LintError represents a single linting issue found in a template manifest.
type LintError struct {
	Command string
	Field   string
	Message string
}

// Error implements the error interface.
func (e LintError) Error() string {
	if e.Command != "" {
		return fmt.Sprintf("command %q, field %q: %s", e.Command, e.Field, e.Message)
	}
	return fmt.Sprintf("field %q: %s", e.Field, e.Message)
}

// ShellCommand is a shell command to run after scaffolding files are written.
// Mode "all" runs the command once with all created files ({{ .Files }}).
// Mode "per-file" runs the command once per created file ({{ .File }}).
// Pattern is an optional comma-separated list of glob patterns (e.g. "*.go" or "*.js,*.tsx").
// When specified, only files matching the pattern(s) are included (default: all files).
// ShellCommand is a shell command to run after scaffolding files are written.
type ShellCommand struct {
	Command string `yaml:"command"`
	Mode    string `yaml:"mode"`    // "all" or "per-file"
	Pattern string `yaml:"pattern"` // optional: comma-separated glob patterns
	Silent  bool   `yaml:"silent"`  // when true, only show "Success" or the error
}

type TestStep struct {
	Command string            `yaml:"command"`
	Params  map[string]string `yaml:"params"`
	// when true, shell_commands are printed but not executed (same as Execute's DryRun)
	DryRun bool `yaml:"dry_run,omitempty"`
}
