# Package: cmd/joist

## Overview
The main command-line entry point for joist. Initializes and runs the joist CLI application.

## Types

### Runner
```go
type Runner func(ctx context.Context, args []string) error
```
A function type that represents a runnable command handler. Takes a context and command-line arguments, returns an error.

## Functions

### Setup()
```go
func Setup() Runner
```

Initializes and returns the joist CLI application runner. This function:

1. Creates a new filesystem adapter for file I/O operations
2. Initializes the scaffolder handler with the filesystem
3. Sets up the root Cobra command with:
   - `list` - List all available scaffolding templates
   - `doc` - Show documentation for a template or specific command
   - `execute` - Execute a template command
   - `lint` - Lint a template manifest for issues
4. Returns a Runner function that executes the Cobra command tree

The returned Runner can be called with command-line arguments to execute any joist command.

## Usage Example

```go
runner := Setup()
err := runner(ctx, []string{"list"})
```
