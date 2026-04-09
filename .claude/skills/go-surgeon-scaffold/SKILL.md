---
name: go-surgeon-scaffold
description: Use this skill whenever the user wants to scaffold, bootstrap, or generate architectural components in a Go project that has `go-surgeon` installed with scaffolding templates (`.surgeon-templates/` directory). This includes bootstrapping new projects, adding features, creating new domain entities, generating HTTP handlers, adding repositories, or any request involving project structure generation. Trigger when the user mentions "scaffold", "bootstrap", "generate feature", "add a new entity/handler/repository", "create project structure", "hexagonal architecture", "DDD setup", or asks to create multiple related files following a pattern. Also trigger when hints from a previous scaffold execution suggest running another scaffold command. Always check for available templates before improvising file creation — the templates encode best practices and produce contextual hints that guide the next steps.
---

# go-surgeon: Scaffold Skill

You are working in a Go project that has `go-surgeon` scaffolding templates available. Before creating files manually, ALWAYS check if a template exists that can do it better.

The scaffold system is a **workflow engine** — not just a file generator. Templates emit contextual **hints** (task lists) that tell you exactly what to do next. You MUST read and follow these hints.

## Pre-flight

```bash
# Check if templates exist
go-surgeon scaffold list-templates

# If no templates found, .surgeon-templates/ doesn't exist yet — skip scaffolding
```

---

## 1. Discovering Templates

### List all available templates
```bash
go-surgeon scaffold list-templates
```
Returns all templates with their names, descriptions, and available commands.

### Read documentation for a template
```bash
# All commands in a template
go-surgeon scaffold doc <template_name>

# Specific command (shows required variables, files generated, hints)
go-surgeon scaffold doc <template_name> <command_name>
```

**Always read the doc before executing.** It tells you:
- What variables are required (`--set Key=Value`)
- What files will be created
- What `post_commands` will chain automatically
- What hints will be emitted

---

## 2. Executing Scaffolding

```bash
go-surgeon scaffold execute <template> <command> --set Key=Value [--set Key2=Value2 ...]
```

### Example workflow
```bash
# Step 1: Read what bootstrap does
go-surgeon scaffold doc hexagonal bootstrap

# Step 2: Execute with required variables
go-surgeon scaffold execute hexagonal bootstrap \
  --set AppName=catalog \
  --set ModulePath=github.com/myorg/myapp
```

### What happens during execution

1. **Pre-flight check**: All destination files are checked. If ANY already exists, the entire execution aborts — no partial writes.
2. **File generation**: Template files are rendered with Go `text/template` syntax and your variables.
3. **Post-command chaining**: If the command has `post_commands`, they execute automatically in sequence. Variables are forwarded. Diamond dependencies are deduplicated (a command never runs twice in the same chain).
4. **Go formatting**: All generated `.go` files are formatted with `goimports` automatically.
5. **Hints**: After all files are created, hints are printed. **READ THEM.**

---

## 3. Following Hints — THIS IS CRITICAL

After every scaffold execution, hints are printed to the console. They look like:

```
--- bootstrap ---
Project bootstrapped. The following commands were executed in sequence.

--- add_main ---
Main created at cmd/catalog/main.go.

--- add_domain ---
Domain layer initialized at internal/catalog/domain/.
You MUST now:
1. Define your domain entities using `go-surgeon add-struct`
2. Create repositories using: go-surgeon scaffold execute hexagonal add_repository --set AppName=catalog --set EntityName=Book
3. Wire the application in cmd/catalog/main.go
```

**You MUST follow these hints step by step.** They are the template author's designed workflow. Do not improvise a different approach.

When a hint tells you to run another scaffold command, run it. When it tells you to use `go-surgeon add-struct` or `go-surgeon update-func`, switch to using those commands (see the `go-surgeon-edit` skill).

---

## 4. Template Variables

Variables are passed via `--set Key=Value` flags. They're available in templates as `{{ .Key }}`.

### Available template functions
- `{{ .AppName | lower }}` — lowercase
- `{{ .AppName | upper }}` — UPPERCASE
- `{{ .AppName | title }}` — Title Case

Variables are used in:
- **File content** (`.tmpl` files)
- **Destination paths** (e.g., `cmd/{{ .AppName }}/main.go`)
- **Hints** (e.g., "Domain layer at `internal/{{ .AppName }}/domain/`")

---

## 5. Creating Your Own Templates

If no template exists for what the user wants, and they'd benefit from a reusable pattern, you can create one.

Templates live in `.surgeon-templates/<name>/manifest.yaml`:

```yaml
name: hexagonal
description: |
  Hexagonal architecture template for Go services.

commands:
  - command: bootstrap
    description: Bootstraps a complete hexagonal project structure
    variables:
      - key: AppName
        description: folder name for the main app
      - key: ModulePath
        description: Go module path
    files:
      - source: .gitignore.tmpl
        destination: .gitignore
      - source: Makefile.tmpl
        destination: Makefile
    post_commands:
      - add_main
    hint: |
      Project bootstrapped successfully.

  - command: add_main
    description: Creates the main.go entrypoint
    variables:
      - key: AppName
        description: folder name for the main app
    files:
      - source: cmd/appname/main.go.tmpl
        destination: cmd/{{ .AppName }}/main.go
    hint: |
      Main created at cmd/{{ .AppName }}/main.go.
      Next: define domain entities and repositories.
```

### Best practices for templates

1. **Granular commands**: Break into small commands (`add_repository`, `add_usecase`, `add_http_handler`) rather than one massive generator.
2. **Actionable hints**: Print task lists that guide step-by-step. LLMs read and execute these iteratively.
3. **Chain with `post_commands`**: Use workflow chaining so `bootstrap` → `add_main` → `add_domain` runs in one shot.
4. **Safety**: Pre-flight check prevents overwriting existing files. Always safe to run.
5. **Variables in paths**: Use `{{ .EntityName | lower }}` in destination paths for dynamic directory creation.
6. **Empty files**: Omit `source` to create an empty file (useful for `.gitkeep` or placeholder packages).

### Template file structure
```
.surgeon-templates/
└── hexagonal/
    ├── manifest.yaml
    ├── .gitignore.tmpl
    ├── Makefile.tmpl
    └── cmd/
        └── appname/
            └── main.go.tmpl
```

Template files use Go `text/template` syntax:
```go
// main.go.tmpl
package main

func main() {
    app := {{ .AppName | lower }}.NewApp()
    app.Run()
}
```

---

## 6. Workflow Decision Tree

When the user asks to create something in a Go project:

```
1. Is go-surgeon available?
   NO  → Fall back to manual file creation
   YES → Continue

2. Does the task involve creating multiple related files / architectural layers?
   NO  → Use go-surgeon edit commands directly (add-func, add-struct, etc.)
   YES → Continue

3. Run: go-surgeon scaffold list-templates
   Templates exist?
   NO  → Either create a template (if reusable) or use edit commands
   YES → Continue

4. Read: go-surgeon scaffold doc <template> <command>
   Command matches the need?
   YES → Execute it and follow hints
   NO  → Use edit commands or create a new template command
```

---

## 7. Common Patterns

### Bootstrap a new service
```bash
go-surgeon scaffold doc hexagonal bootstrap
go-surgeon scaffold execute hexagonal bootstrap --set AppName=catalog --set ModulePath=github.com/myorg/catalog
# → Follow hints to add entities, repos, handlers
```

### Add a new domain entity
```bash
go-surgeon scaffold doc hexagonal add_entity
go-surgeon scaffold execute hexagonal add_entity --set AppName=catalog --set EntityName=Book
# → Follow hints to define fields, create repository, wire in app
```

### Add a repository
```bash
go-surgeon scaffold doc hexagonal add_repository
go-surgeon scaffold execute hexagonal add_repository --set AppName=catalog --set EntityName=Book
# → Follow hints to implement the adapter
```

### Chain: the hints drive you
Each scaffold execution produces hints → you follow them → some hints say to run another scaffold → you run it → it produces more hints → repeat until the architecture is complete.

This is the power of the scaffold workflow engine: **the template author designs the entire build sequence, and you just follow the hints.**
