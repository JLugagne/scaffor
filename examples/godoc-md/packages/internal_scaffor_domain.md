# internal/scaffor/domain

Core domain models and business logic for scaffor.

## Overview

The `domain` package contains the fundamental data structures and interfaces that define what scaffor does. This is the heart of the application, independent of any specific implementation details (CLI, filesystem, etc.).

## Key Types

### Template

The core template definition:

```go
type Template struct {
    Name string                // Unique template identifier
    Description string         // Human-readable description
    Commands []TemplateCommand // Available scaffold commands
    ShellCommands []ShellCommand // Shell commands to run after scaffold
    Test []TestStep           // Test steps for validation
    Validate []string         // Validation shell commands
}
```

### TemplateCommand

Defines a single scaffold command within a template:

```go
type TemplateCommand struct {
    Command string              // Command name
    Description string          // What this command does
    Variables []TemplateVariable // Required/optional variables
    Files []TemplateFile        // Files to create/copy
    PostCommands []string       // Commands to run after this one
    ShellCommands []ShellCommand // Shell commands for this command
    Hint string                 // Helpful hint text
}
```

### TemplateVariable

Describes a variable required by a command:

```go
type TemplateVariable struct {
    Key string         // Variable name (must start with capital letter)
    Description string // What this variable is for
}
```

### TemplateFile

Specifies a file to create or copy:

```go
type TemplateFile struct {
    Source string     // Source template path
    Destination string // Target path (supports Go templates)
    OnConflict string  // Action when file exists (skip, force, fail)
}
```

### ShellCommand

Defines a shell command to execute:

```go
type ShellCommand struct {
    Command string // Shell command to run
    Mode string    // "all" or "per-file"
    Pattern string // Glob pattern for per-file mode
    Silent bool    // Suppress output
}
```

### ExecuteOptions

Options for executing a template command:

```go
type ExecuteOptions struct {
    DryRun bool // Preview without making changes
    Skip bool   // Skip existing files
    Force bool  // Overwrite existing files
}
```

### FileEvent

Records what happened to a file during execution:

```go
type FileEvent struct {
    Path string   // File path
    Action string // "created", "overwritten", or "skipped"
}
```

### LintError

Error that occurs during template validation:

```go
type LintError struct {
    Command string // Command where error occurred
    Field string   // Field name that caused the error
    Message string // Error description
}
```

## Subpackages

- `action` — Template definition and validation
- `options` — Execution options
- `repositories/filesystem` — Template repository abstraction
- `service` — Core scaffolding service
