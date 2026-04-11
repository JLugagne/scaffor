package domain

import "fmt"

type Template struct {
	Name          string            `yaml:"name"`
	Description   string            `yaml:"description"`
	Commands      []TemplateCommand `yaml:"commands"`
	ShellCommands []ShellCommand    `yaml:"shell_commands"`
}

type TemplateCommand struct {
	Command       string             `yaml:"command"`
	Description   string             `yaml:"description"`
	Variables     []TemplateVariable `yaml:"variables"`
	Files         []TemplateFile     `yaml:"files"`
	PostCommands  []string           `yaml:"post_commands"`
	ShellCommands []string           `yaml:"shell_commands"`
	Hint          string             `yaml:"hint"`
}

type TemplateVariable struct {
	Key         string `yaml:"key"`
	Description string `yaml:"description"`
}

type TemplateFile struct {
	Source      string `yaml:"source"`
	Destination string `yaml:"destination"`
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
type ShellCommand struct {
	Command string `yaml:"command"`
	Mode    string `yaml:"mode"`    // "all" or "per-file"
	Pattern string `yaml:"pattern"` // optional: comma-separated glob patterns
}
