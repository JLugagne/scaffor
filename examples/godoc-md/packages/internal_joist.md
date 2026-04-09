# Package: internal/joist

## Overview
The main package that initializes and wires up the entire joist application. It contains the entry point (`Setup()`) that creates and configures the CLI command structure.

## Architecture

Joist follows a **layered hexagonal architecture**:

```
┌─────────────────────────────────────────┐
│  CLI Layer (inbound/cli/commands)       │  ← User interface
├─────────────────────────────────────────┤
│  Application Layer (app/commands)       │  ← Business logic
├─────────────────────────────────────────┤
│  Domain Layer (domain/)                 │  ← Core entities
│    - Service interfaces (domain/service)│
│    - Repository interfaces (domain/...)│
├─────────────────────────────────────────┤
│  Adapter Layer (outbound/...)           │  ← External integrations
└─────────────────────────────────────────┘
```

## Key Components

### Type: Runner
```go
type Runner func(ctx context.Context, args []string) error
```

A function type representing an executable CLI runner. Takes:
- `ctx`: Context for cancellation and timeouts
- `args`: Command-line arguments (e.g., `["list"]`, `["execute", "template-name", "init"]`)

Returns an error if the command fails.

### Function: Setup()
```go
func Setup() Runner
```

Initializes the complete joist CLI application with:

1. **Filesystem Adapter**: Creates a concrete file system implementation
2. **Scaffolder Handler**: Initializes the main command handler with the filesystem
3. **CLI Commands**: Registers all Cobra commands:
   - `list` — List available templates
   - `doc` — Show template/command documentation
   - `execute` — Execute a template command
   - `lint` — Validate a template manifest
4. **Returns**: A `Runner` function ready to execute any joist command

## Data Flow

1. User runs: `joist execute my-template init --set Name=Value`
2. Cobra routes to `NewExecuteCommand()` handler
3. Handler calls `ScaffolderHandler.Execute()` with parsed arguments
4. Handler loads the template, validates, executes post-commands, writes files
5. Handler optionally runs shell commands or shows them for manual review
6. Summary of created files and status is printed

## Layer Responsibilities

| Layer | Responsibility |
|-------|-----------------|
| **CLI** | Parse flags, display output, handle user interaction |
| **App** | Orchestrate scaffold execution, manage file I/O, validate input |
| **Domain** | Define interfaces, core data structures (Template, TemplateCommand, etc.) |
| **Adapter** | Implement domain interfaces (file system operations) |

## Dependency Injection

All dependencies flow downward:
- CLI commands depend on `ScaffolderCommands` service interface
- Service implementation (`ScaffolderHandler`) depends on `FileSystem` interface
- Concrete file system implementation has no dependencies

This enables:
- Easy testing through mock implementations
- Swapping implementations without changing code
- Clear separation of concerns

## Integration Points

### Related Packages

- **[cmd/joist](packages/cmd_joist.md)** — CLI entry point that calls `Setup()`
- **[app/commands](packages/internal_joist_app_commands.md)** — Handler that implements core logic
- **[domain](packages/internal_joist_domain.md)** — Data structures and domain models
- **[domain/service](packages/internal_joist_domain_service.md)** — Service interface definition
- **[inbound/cli/commands](packages/internal_joist_inbound_cli_commands.md)** — CLI command factories
- **[outbound/filesystem](packages/internal_joist_outbound_filesystem.md)** — File system adapter

## Usage

```go
// In main.go or cmd/joist/main.go
func main() {
    ctx := context.Background()
    runner := joist.Setup()
    
    // Execute joist with command-line args
    if err := runner(ctx, os.Args[1:]); err != nil {
        log.Fatal(err)
    }
}
```

## Template Discovery

Templates are loaded from `.joist-templates/` in the current working directory:

```
.joist-templates/
├── godoc-md/
│   ├── manifest.yaml
│   ├── templates/
│   │   └── index.md.tpl
│   └── shell_commands.yaml
└── my-template/
    ├── manifest.yaml
    └── files/
        └── main.go.tpl
```

Each template is a directory containing a YAML manifest that defines commands, variables, and file templates.
