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
	Command       string              `yaml:"command"`
	Description   string              `yaml:"description"`
	Variables     []TemplateVariable  `yaml:"variables"`
	Files         []TemplateFile      `yaml:"files"`
	Injections    []TemplateInjection `yaml:"injections"`
	PostCommands  []string            `yaml:"post_commands"`
	ShellCommands []ShellCommand      `yaml:"shell_commands"`
	Hint          string              `yaml:"hint"`
}

type TemplateVariable struct {
	// Key is the variable name (must start with a capital letter — text/template requirement).
	Key string `yaml:"key"`
	// Description is the human-readable explanation surfaced by `scaffor doc` and the
	// "missing required variables" error.
	Description string `yaml:"description"`
	// Optional marks the variable as not required at execute time. When omitted and
	// Default is empty, the variable's template references render as the empty string
	// (text/template's missing-key default). `scaffor doc` tags optional variables
	// with "(optional)" in the listing.
	Optional bool `yaml:"optional,omitempty"`
	// Default is the value injected into params when the user does not pass --set
	// for this variable. Implies Optional (a defaulted variable is never "missing").
	// Use for stable cross-cutting defaults like "pg" for an Adapter; for derived
	// defaults that need other variables (e.g. printf "internal/%s/...") use sprig's
	// `default` inside the template instead.
	Default string `yaml:"default,omitempty"`
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

// TemplateInjection describes a deterministic modification of an existing
// file: Content is inserted as full lines relative to the first line of the
// target that contains Anchor. Target, Anchor, Content, and SkipIf are
// rendered as Go templates with the command's variables before use.
type TemplateInjection struct {
	// Target is the path of the file to modify, rendered like a destination.
	Target string `yaml:"target"`
	// Anchor selects the insertion point: the first line containing this
	// string (after rendering). Injection fails if no line matches, unless
	// OnMissing is "skip".
	Anchor string `yaml:"anchor"`
	// Position is "after" (default) or "before" the anchor line.
	Position string `yaml:"position"`
	// Content is inserted verbatim as full lines; a trailing newline is
	// ensured. Indentation is the template author's responsibility.
	Content string `yaml:"content"`
	// SkipIf is an idempotency guard: when the rendered value is non-empty
	// and already present in the target, the injection is skipped. When
	// empty, the injection is skipped if the rendered Content is already
	// present.
	SkipIf string `yaml:"skip_if"`
	// OnMissing is "fail" (default) or "skip": what to do when the target
	// file does not exist or the anchor is not found.
	OnMissing string `yaml:"on_missing"`
}
