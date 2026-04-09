# Package: internal/joist/inbound/cli/commands

## Overview
Inbound CLI command definitions. This package bridges the user-facing CLI (Cobra) with the underlying scaffolding service. Each function creates a Cobra command that wraps a specific scaffolding operation.

## Functions

### NewListTemplatesCommand
```go
func NewListTemplatesCommand(scaffolder service.ScaffolderCommands) *cobra.Command
```

Creates the `joist list` command that displays all available templates.

**Command**: `joist list`

**Output**: Lists template names with descriptions

**Example**:
```bash
$ joist list
Available Templates:

- godoc-md: Generates Markdown documentation...
- godoc-html: Generates HTML documentation...
```

### NewDocCommand
```go
func NewDocCommand(scaffolder service.ScaffolderCommands) *cobra.Command
```

Creates the `joist doc` command that displays template and command documentation.

**Commands**:
- `joist doc <template>` — Shows template overview and available commands
- `joist doc <template> <command>` — Shows detailed information about a specific command, including required variables

**Example**:
```bash
$ joist doc godoc-md
Template: godoc-md
Generates Markdown documentation from Go source code...

Commands:
  init - Generates the full documentation site in one shot
  ...

$ joist doc godoc-md init
Command: init
  Generates the full documentation site in one shot
  
  Variables:
    --set ProjectName    Display name of the project
    --set ModulePath     Go module path
  ...
```

### NewExecuteCommand
```go
func NewExecuteCommand(scaffolder service.ScaffolderCommands) *cobra.Command
```

Creates the `joist execute` command that runs template commands with variable substitution.

**Command**: `joist execute <template> <command> [--set KEY=VALUE]...`

**Flags**:
- `--set KEY=VALUE` — Provide template variables (repeatable)
- `--run-commands` — Execute post-generation shell commands automatically (default: show for manual review)

**Example**:
```bash
$ joist execute godoc-md init --set ProjectName=joist --set ModulePath=github.com/JLugagne/joist
Created files:
  examples/godoc-md/index.md
  examples/godoc-md/_sidebar.md
  ...

SUCCESS: Executed godoc-md/init (2 commands, 0 skipped)
```

### NewLintCommand
```go
func NewLintCommand(scaffolder service.ScaffolderCommands) *cobra.Command
```

Creates the `joist lint` command that validates template manifests.

**Command**: `joist lint <template>`

**Output**: Reports validation errors found in the template (missing required fields, invalid paths, etc.)

**Example**:
```bash
$ joist lint my-template
Template: my-template
- Command 'init': Missing required variable 'ProjectName'
- File 'templates/main.go': Source file not found
```

Returns exit code 0 if valid, non-zero if errors found.

## Usage

These functions are called during CLI setup in `internal/joist/init.go:Setup()` to register all commands with the Cobra root command.

```go
scaffolderHandler := appcommands.NewScaffolderHandler(fs)

rootCmd.AddCommand(
    NewListTemplatesCommand(scaffolderHandler),
    NewDocCommand(scaffolderHandler),
    NewExecuteCommand(scaffolderHandler),
    NewLintCommand(scaffolderHandler),
)
```

## Architecture Notes

Each command:
- Accepts a `ScaffolderCommands` interface for dependency injection
- Translates CLI flags and arguments to the service interface
- Handles user-facing output and error messages
- Enables testing by allowing mock service implementations
