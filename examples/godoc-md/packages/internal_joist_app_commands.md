# Package: internal/joist/app/commands

## Overview
Application layer command handlers. `ScaffolderHandler` implements the core business logic for template loading, validation, and execution.

## Types

### ScaffolderHandler
```go
type ScaffolderHandler struct {
	fs filesystem.FileSystem
}
```

The main command handler that orchestrates template-based scaffolding. It manages:
- Loading template manifests from disk
- Validating templates
- Executing template commands with variable substitution
- Managing file creation and post-execution hooks

#### Constructor

**NewScaffolderHandler(fs filesystem.FileSystem) \*ScaffolderHandler**
```go
func NewScaffolderHandler(fs filesystem.FileSystem) *ScaffolderHandler
```
Creates a new handler with the given filesystem adapter.

#### Methods

**ListTemplates(ctx context.Context) ([]domain.Template, error)**
```go
func (h *ScaffolderHandler) ListTemplates(ctx context.Context) ([]domain.Template, error)
```

Scans the `.joist-templates/` directory and returns all valid templates. Silently skips invalid template directories.

Returns an empty slice if `.joist-templates/` does not exist (not an error).

**GetTemplate(ctx context.Context, templateName string) (domain.Template, error)**
```go
func (h *ScaffolderHandler) GetTemplate(ctx context.Context, templateName string) (domain.Template, error)
```

Loads a single template by name. Parses the template's YAML manifest from `.joist-templates/<templateName>/`.

**Execute(ctx context.Context, templateName, commandName string, params map[string]string, runCommands bool) error**
```go
func (h *ScaffolderHandler) Execute(ctx context.Context, templateName, commandName string, params map[string]string, runCommands bool) error
```

Executes a template command with the following workflow:

1. **Load & Validate**: Loads the template and verifies the command exists
2. **Pre-flight Checks**: Ensures all required variables are provided
3. **Execution**: Recursively processes the command and its post-commands (with deduplication):
   - Renders file paths and content using Go templates with provided variables
   - Creates directories as needed
   - Writes files to disk
   - Collects hint messages for user guidance
4. **Post-commands**: Chains any post-commands defined in the command spec
5. **Summary**: Outputs created files and deduplication information
6. **Shell Commands**: 
   - If `runCommands=true`, executes all resolved shell commands directly
   - If `runCommands=false`, prints shell commands for manual execution

**Variable Substitution**: Uses Go's `text/template` package with support for custom functions.

**File Modes**:
- `source=""` creates an empty file
- `source="path/to/template"` reads and renders the template file

**Shell Command Modes**:
- `mode="all"`: Runs once with all created files available as `{{ .Files }}`
- `mode="per-file"`: Runs once per created file (available as `{{ .File }}`)

**Lint(ctx context.Context, templateName string) []domain.LintError**
```go
func (h *ScaffolderHandler) Lint(ctx context.Context, templateName string) []domain.LintError
```

Validates a template manifest for common issues (missing required fields, invalid paths, etc.). Returns a list of all linting errors found.

## Usage Example

```go
handler := NewScaffolderHandler(filesystem.NewFileSystem())

// List all templates
templates, err := handler.ListTemplates(ctx)

// Execute a template
err := handler.Execute(
    ctx,
    "my-template",
    "init",
    map[string]string{
        "ProjectName": "MyProject",
        "Author": "John Doe",
    },
    true, // run shell commands automatically
)
```

## Architecture Notes

The handler:
- Performs AST-based template validation
- Supports variable substitution at both file path and content levels
- Deduplicates post-command execution to avoid redundant scaffolding
- Collects user guidance (hints) and outputs them after successful execution
- Can optionally run post-generation shell commands or present them for manual review
