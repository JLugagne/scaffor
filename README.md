# joist: Template-Driven Scaffolding for AI Agents

**joist** is a CLI scaffolding tool designed for AI agents and developers. It generates files from local templates, validates manifests, and guides step-by-step construction of complex project structures through contextual `hints`.

## Why joist

- **Workflow Orchestration:** The `scaffold` command provides a built-in workflow engine. Templates emit context-aware `hints` (Task Lists) that guide the AI step-by-step through building complex architectures instead of letting it improvise.
- **Safe by Default:** Pre-flight checks abort execution if any destination file already exists, preventing partial overwrites.
- **Lint Before Execute:** Validate templates before running them to catch issues early.
- **Language-Agnostic:** Works with any tech stack — templates are plain files rendered with `text/template`.

## Quick Start

### Build
```bash
go build -o joist ./cmd/joist
```

### Install
```bash
go install github.com/JLugagne/joist/cmd/joist@latest
```

### Shell completion (optional)
```bash
joist completion bash > /etc/bash_completion.d/joist   # bash
joist completion zsh > "${fpath[1]}/_joist"             # zsh
```

### Usage Overview
```bash
# List scaffolding templates, lint, and read documentation
joist scaffold list-templates
joist scaffold lint hexagonal
joist scaffold doc hexagonal bootstrap

# Execute a scaffolding workflow
joist scaffold execute hexagonal bootstrap --set AppName=catalog
```

See `USAGE.md` for detailed documentation on all commands and flags.

## Template-Driven Scaffolding

Templates live in `.joist-templates/` at the root of your project:

```
.joist-templates/
└── hexagonal/
    ├── manifest.yaml
    ├── .gitignore.tmpl
    └── cmd/appname/main.go.tmpl
```

### `manifest.yaml`

```yaml
name: hexagonal
description: |
  Hexagonal architecture template.

commands:
  - command: bootstrap
    description: Bootstraps a complete hexagonal project structure
    variables:
      - key: AppName
        description: folder name for the main app
    files:
      - source: .gitignore.tmpl
        destination: .gitignore
      - source: cmd/appname/main.go.tmpl
        destination: cmd/{{ .AppName }}/main.go
    post_commands:
      - command: go fmt ./...
        mode: all
    hint: |
      Project bootstrapped. Run: joist scaffold execute hexagonal add_domain --set AppName={{ .AppName }}

  - command: add_domain
    description: Creates the domain layer
    variables:
      - key: AppName
        description: folder name for the main app
    files:
      - source: domain/domain.go.tmpl
        destination: internal/{{ .AppName }}/domain/domain.go
```

### Command components

- **`variables`** — declared inputs, passed via `--set Key=Value`. Used in templates as `{{ .Key }}`.
- **`files`** — files to generate. `destination` is itself a template (e.g. `cmd/{{ .AppName }}/main.go`). Omit `source` to create an empty file.
- **`post_commands`** — shell commands to run after files are written. Each entry has a `command` (shell string) and an optional `mode`:
  - `all` *(default)* — run once; `{{ .Files }}` expands to all created files
  - `per-file` — run once per created file; `{{ .File }}` expands to each path
- **`hint`** — a message printed after execution, rendered as a template. Use this to tell AI agents what to do next.

### Template functions

| Function | Description | Example |
|----------|-------------|---------|
| `lower` | Lowercase | `{{ .AppName \| lower }}` |
| `upper` | Uppercase | `{{ .AppName \| upper }}` |
| `title` | Capitalize first letter | `{{ .AppName \| title }}` |

### Linting templates

Before executing a template, validate it with `lint`:

```bash
joist scaffold lint hexagonal
```

This checks:
- All `post_commands` have a non-empty `command` and a valid `mode` (`all` or `per-file`)
- All `{{ .VarName }}` used in destination paths are declared in `variables`
- All `{{ .VarName }}` used in template source files are declared in `variables`

### Running post-commands

By default, post-commands are printed after scaffolding so you can review and run them manually:

```
Post-commands to run:
  go fmt ./...

Run with --run-commands to execute them automatically.
```

Pass `--run-commands` to execute them automatically via the shell:

```bash
joist scaffold execute hexagonal bootstrap --set AppName=catalog --run-commands
```

### Best practices for AI agents

1. **Granular commands:** Break down architecture into small commands (`add_repository`, `add_usecase`, `add_handler`) rather than one massive generator.
2. **Actionable hints:** Use `hint` to print a Task List of the next commands the agent should run. LLMs process these hints and act on them iteratively.
3. **Lint before execute:** Run `joist scaffold lint <template>` to catch issues before execution.
4. **Safety first:** `joist` performs a strict pre-flight check before executing. If any destination file already exists, it aborts the entire execution to prevent partial overwrites.
