# Package: internal/joist/domain/service

## Overview
Defines the service-level interface for the scaffolding system. This acts as the boundary between the domain and application layers.

## Types

### ScaffolderCommands
```go
type ScaffolderCommands interface {
	ListTemplates(ctx context.Context) ([]domain.Template, error)
	GetTemplate(ctx context.Context, templateName string) (domain.Template, error)
	Execute(ctx context.Context, templateName, commandName string, params map[string]string, runCommands bool) error
	Lint(ctx context.Context, templateName string) []domain.LintError
}
```

Defines the interface for scaffolding applications and features. This interface is implemented by `internal/joist/app/commands.ScaffolderHandler`.

#### Methods

**ListTemplates(ctx context.Context) ([]domain.Template, error)**

Returns all available scaffolding templates from the `.joist-templates/` directory.

**GetTemplate(ctx context.Context, templateName string) (domain.Template, error)**

Retrieves a single template by name.

**Execute(ctx context.Context, templateName, commandName string, params map[string]string, runCommands bool) error**

Executes a specific command within a template with the provided variables.

- `templateName`: Name of the template
- `commandName`: Name of the command within that template
- `params`: Variable substitutions (e.g., `{"ProjectName": "MyApp"}`)
- `runCommands`: If true, automatically executes post-generation shell commands; if false, prints them for manual review

**Lint(ctx context.Context, templateName string) []domain.LintError**

Validates a template manifest and returns any errors found.

## Implementations

- `internal/joist/app/commands.ScaffolderHandler` — Main application implementation

## Usage

```go
var scaffolder service.ScaffolderCommands = appcommands.NewScaffolderHandler(fs)

// List templates
templates, err := scaffolder.ListTemplates(ctx)

// Load a template
template, err := scaffolder.GetTemplate(ctx, "godoc-md")

// Execute a template command
err := scaffolder.Execute(
    ctx,
    "godoc-md",
    "init",
    map[string]string{
        "ProjectName": "joist",
        "ModulePath": "github.com/JLugagne/joist",
    },
    false, // show commands instead of running them
)
```

## Architecture Notes

This interface enables:
- Loose coupling between CLI commands and scaffolding logic
- Easy testing through mock implementations
- Clear separation between domain and application concerns
- Multiple implementations if needed (e.g., remote scaffolding service)
