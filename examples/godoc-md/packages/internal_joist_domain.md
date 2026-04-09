# Package: internal/joist/domain

## Overview
Defines the core domain models for joist template scaffolding system. Contains the data structures that represent templates, commands, variables, and file transformations.

## Types

### Template
```go
type Template struct {
	Name          string            `yaml:"name"`
	Description   string            `yaml:"description"`
	Commands      []TemplateCommand `yaml:"commands"`
	ShellCommands []ShellCommand    `yaml:"shell_commands"`
}
```

Represents a complete scaffolding template. A template is a reusable blueprint for generating files and running commands with variable substitution.

- **Name**: Identifier for the template (used in CLI commands)
- **Description**: Human-readable description of what the template does
- **Commands**: List of named commands that can be executed within this template
- **ShellCommands**: Post-execution shell commands to run after file scaffolding

### TemplateCommand
```go
type TemplateCommand struct {
	Command      string             `yaml:"command"`
	Description  string             `yaml:"description"`
	Variables    []TemplateVariable `yaml:"variables"`
	Files        []TemplateFile     `yaml:"files"`
	PostCommands []string           `yaml:"post_commands"`
	Hint         string             `yaml:"hint"`
}
```

Defines a single executable command within a template.

- **Command**: Name of the command
- **Description**: What this command does
- **Variables**: Input variables required from the user (e.g., `--set ProjectName=MyApp`)
- **Files**: Files to create/modify with variable substitution
- **PostCommands**: Other template commands to chain/execute after this one
- **Hint**: Guidance text shown to users after execution

### TemplateVariable
```go
type TemplateVariable struct {
	Key         string `yaml:"key"`
	Description string `yaml:"description"`
}
```

Describes a variable that can be set when executing a template command. Users provide values via `--set Key=Value`.

### TemplateFile
```go
type TemplateFile struct {
	Source      string `yaml:"source"`
	Destination string `yaml:"destination"`
}
```

Maps a template source file (with variable placeholders) to a destination path in the project. Both paths support variable substitution using `{{ .VariableName }}`.

### LintError
```go
type LintError struct {
	Command string `yaml:"command"`
	Field   string `yaml:"field"`
	Message string `yaml:"message"`
}
```

Represents a validation error found in a template manifest.

#### Methods

**Error() string**
```go
func (e LintError) Error() string
```
Implements the error interface, returning a formatted error message.

### ShellCommand
```go
type ShellCommand struct {
	Command string `yaml:"command"`
	Mode    string `yaml:"mode"` // "all" or "per-file"
}
```

A shell command to run after scaffolding files are written.

- **Command**: The shell command to execute
- **Mode**: Execution strategy:
  - `"all"`: Runs once with all created files available via `{{ .Files }}`
  - `"per-file"`: Runs once per created file (available via `{{ .File }}`)

Example: `go fmt {{ .File }}` with `mode: per-file` formats each generated file individually.

## Usage

These types are loaded from YAML template manifests in `.joist-templates/` directories. The scaffolder service uses them to:

1. Parse and validate template definitions
2. Process variable substitutions
3. Generate files at specified destinations
4. Execute post-generation shell commands
