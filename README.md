# joist: The AI-Native AST Code Editor for Go

AI agents and LLMs waste massive amounts of context window tokens and time trying to modify existing Go code using generic text replacement tools (like diff blocks or regex). These methods are inherently fragile, prone to indentation hallucinations, and often trap the AI in an endless loop of syntax errors.

**joist** solves this by providing a deterministic, CLI-first interface built on Go's Abstract Syntax Tree (go/ast). It acts as a lightweight Language Server and a byte-range replacement engine, empowering AI agents to read, explore, and modify code with absolute surgical precision.

## Why your AI agent needs joist

- **Zero Indentation Errors:** We locate the exact byte offsets in the AST. Your agent just streams the raw code block, and `goimports` handles formatting and imports automatically.
- **Perfect Context Preservation:** Standard AST mutations strip away internal comments. Our byte-range engine preserves all surrounding comments and seamlessly updates Godoc blocks.
- **Maximized Context Window:** Acts as a CLI-based LSP (`graph` and `symbol` commands). Agents can query function signatures, docs, and bodies without loading entire 2000-line files into their context window.
- **Drastic Turn Reduction:** Atomically update complex methods, interfaces, or structs in a single shot. No more "hunk matching" failures.
- **Workflow Orchestration:** The `scaffold` command provides a built-in workflow engine. Templates emit context-aware `hints` (Task Lists) that guide the AI step-by-step through building complex architectures (like Hexagonal/DDD) instead of letting it improvise.

## Core Features

### 1. Package & Symbol Graph (`graph`)
Walk all Go packages and print their import paths. With `--symbols --dir`, list every exported type, function, and method in a subtree — a structural map in one command. Context window management flags (`--depth`, `--focus`, `--exclude`, `--token-budget`) let agents progressively zoom in without overwhelming their token budget.

### 2. Code Exploration (`symbol`)
Query the AST to extract function signatures, documentation, or full bodies with empty lines stripped to save LLM tokens. Supports precise `Receiver.Method` lookups to cut through noise.

### 3. Surgical Editing (per-action subcommands)
Individual subcommands (`add-func`, `update-func`, `delete-func`, `add-struct`, etc.) each accept raw Go source on stdin and metadata via `--flags`. Every mutation runs `goimports` automatically.

### 4. Interface Management (`add-interface` / `update-interface` / `delete-interface`)
Create or update an interface and its function-field mock in one command. The mock is auto-generated and kept in sync.

### 5. Interface Implementation (`implement`)
Automatically generates missing method stubs on a struct to satisfy any interface — stdlib, third-party, or project-local. Scans the package to prevent cross-file duplicates.

### 6. Standalone Mock Generation (`mock`)
Generate a function-field mock for any interface you don't own without modifying the interface file.

### 7. Template-Driven Scaffolding (`scaffold`)
Generates standard architecture components from local templates. Features a built-in workflow engine with `post_commands` chaining and contextual `hints` to guide AI agents step-by-step through project construction. Use `joist scaffold lint` to validate templates before execution.

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
# Orient yourself — packages map, then symbols in a subtree
joist graph
joist graph --symbols --dir internal/catalog/domain

# Progressive discovery — zoom in without blowing up context
joist graph --summary --depth 2
joist graph --focus internal/catalog/domain
joist graph --summary --deps --token-budget 2000

# Read a symbol before editing it
joist symbol BookHandler.Handle --body

# Edit: pipe raw Go source, pass metadata as flags
cat <<'EOF' | joist update-func --file internal/catalog/domain/book.go --id NewBook
func NewBook(title, author string) (*Book, error) {
    return &Book{Title: title, Author: author}, nil
}
EOF

# Generate interface stubs on a struct
joist implement io.ReadCloser --receiver "*MyReader" --file internal/pkg/reader.go

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
  Hexagonal architecture template for Go services.

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
      - add_domain
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
- **`post_commands`** — commands to chain automatically after this one. Variables are forwarded; commands are deduplicated (diamond-dependency safe).
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
- All `post_commands` reference existing commands
- All `{{ .VarName }}` used in destination paths are declared in `variables`
- All `{{ .VarName }}` used in template source files are declared in `variables`

### Best practices for AI agents

1. **Granular commands:** Break down architecture into small commands (`add_repository`, `add_usecase`, `add_http_handler`) rather than one massive generator.
2. **Actionable hints:** Use `hint` to print a Task List of the next commands the agent should run. LLMs process these hints and act on them iteratively.
3. **Lint before execute:** Run `joist scaffold lint <template>` to catch issues before execution.
4. **Safety first:** `joist` performs a strict pre-flight check before executing any chain. If any destination file already exists, it aborts the entire execution to prevent partial overwrites.
