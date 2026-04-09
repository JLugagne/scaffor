# joist: Structural Support for AI-Driven Development

A joist carries the load so the architect doesn't have to. **joist** carries the scaffolding so your LLM doesn't have to.

**joist** is a CLI scaffolding tool designed for AI agents. One command generates the entire file structure, pre-flight checks prevent overwrites, and `hints` tell the agent exactly what to do next — so the LLM spends its tokens on reasoning, not boilerplate.

## Why joist

When an LLM scaffolds a project from scratch it:
- burns a large portion of its context window writing boilerplate files
- guesses at folder structures and naming conventions, introducing drift
- loses track of what it has already written across long sessions

joist solves this by moving scaffolding out of the LLM entirely. The agent calls one deterministic command, gets back a list of files created and a `hint` describing the next step, and moves on. The LLM never has to think about file layout again.

- **Offloads boilerplate completely:** one command writes every file — the LLM only needs to call it, not invent it.
- **Guided execution via hints:** each command prints a context-aware Task List that tells the agent what to run next, eliminating improvisation.
- **Safe by default:** pre-flight checks abort if any destination file already exists, preventing partial overwrites.
- **Lint before execute:** validate templates before running them to catch issues early.
- **Language-agnostic:** works with any tech stack — templates are plain files rendered with `text/template`.

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
joist scaffold list
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

1. **One command per concern:** Define small, composable commands (`add_repository`, `add_usecase`, `add_handler`) rather than one massive generator. The agent chains them; joist chains the files.
2. **Use hints as a task queue:** Write `hint` as an ordered list of `joist scaffold execute` commands the agent should run next. The agent reads the hint output and acts on it — no planning required.
3. **Never ask the LLM to write boilerplate:** if a file is structural (main.go, Makefile, CI config), put it in a template. Reserve the LLM for files that require actual reasoning.
4. **Lint in CI:** Run `joist scaffold lint <template>` in your pipeline so broken templates never reach the agent.

## Examples

The templates in [`/examples/`](./examples/) were generated entirely by Claude Haiku using only `joist --help` as context — no custom instructions, no prompt engineering, no skills.
