# joist

**Make LLM-driven development deterministic.**
Save 10–50 LLM turns per feature by offloading boilerplate and structure to executable templates.

```bash
joist execute hexagonal-cli add_entity --set Entity=User
```
```
Created files:
  internal/myapp/domain/user.go
  internal/myapp/domain/repositories/user/user.go
  internal/myapp/outbound/user/user.go
  internal/myapp/app/commands/user_handler.go

--- add_entity ---
NEXT STEPS:
  1. Define User fields in domain/user.go
  2. Implement the adapter in outbound/user/
  3. Wire the handler in init.go

SUCCESS: Executed hexagonal-cli/add_entity
```

Structure enforced. Files in place. No planning required — the LLM just fills in the blanks.

## Works with small models

The examples in [`/examples/`](./examples/) were generated entirely by Claude Haiku using only `joist --help` as context — no custom instructions, no prompt engineering, no skills.

Well-written templates reduce the reasoning burden enough to drop from Opus to Sonnet or Haiku for the majority of tasks.

## The problem

LLMs are good at reasoning. They are inefficient at generating repetitive code, maintaining consistent structure across files, and planning multi-step scaffolding without drifting. This wastes tokens, adds unnecessary iterations, and produces inconsistent outputs.

Without joist, the LLM plans the structure, creates files one by one, fixes mistakes, iterates, loses context, drifts from conventions. In a known codebase, that's 10–15 extra turns. In a project the agent discovers cold, easily 40–50.

With joist, the LLM calls one command, structure is generated instantly, hints tell it what's next, and it focuses only on business logic.

## How it works

The LLM discovers what's available, executes, then writes logic.

```bash
# 1. Discover
joist list
joist doc hexagonal-cli bootstrap

# 2. Execute
joist execute hexagonal-cli bootstrap --set AppName=myapp --set ModulePath=github.com/org/myapp

# 3. The LLM completes the generated code (business logic only)
```

Each command creates files deterministically and prints a `hint` — a structured task list telling the LLM exactly what to do next. No planning required.

**Feed it to your agent:** copy the contents of [`AI_INSTRUCTIONS.md`](./AI_INSTRUCTIONS.md) into your agent's system prompt (`.cursorrules`, `.clinerules`, AGENTS.md) to make it instantly aware of joist.

## Templates

A template is a directory with a `manifest.yaml` that declares commands, variables, files, and hints:

```yaml
name: hexagonal-cli
commands:
  - command: add_entity
    variables:
      - key: Entity
        description: entity name (PascalCase)
    files:
      - source: domain/entity.go.tmpl
        destination: internal/{{ .AppName }}/domain/{{ .Entity | lower }}.go
      - source: domain/repositories/entity/entity.go.tmpl
        destination: internal/{{ .AppName }}/domain/repositories/{{ .Entity | lower }}/{{ .Entity | lower }}.go
    hint: |
      Entity {{ .Entity }} added.
      Now wire the handler in init.go and add a CLI command:
        joist execute hexagonal-cli add_command --set Command={{ .Entity | lower }}

shell_commands:
  - command: "goimports -w {{ .Files }}"
    mode: all
```

Templates are machine-readable, LLM-friendly, and composable via chained commands. They work with any language — Go, Python, TypeScript, Terraform, documentation, anything that's a text file.

## Linting

`joist lint` validates templates statically before execution:

```
$ joist lint broken-template
LINT ERRORS in broken-template:

  command "create", field "files.destination": variable "Nme" used but not declared (did you mean "Name"?)
  command "create", field "post_commands": references undefined command "add_mock"

2 issue(s) found
```

Catches undeclared variables, broken references, invalid modes, and suggests corrections via Levenshtein distance. Runs in milliseconds — put it in CI.

## Safety

Shell commands are printed after execution but **never run** unless you explicitly pass `--run-commands`. The LLM sees the commands, asks the user for confirmation, and the human stays in the loop. Pre-flight checks abort if any destination file already exists. Directory traversal (`..`) is rejected.

## Installation

```bash
go install github.com/JLugagne/joist/cmd/joist@latest
```

Or build from source:

```bash
git clone https://github.com/JLugagne/joist.git
cd joist
go build -o joist ./cmd/joist
```

Shell completion:

```bash
joist completion bash > /etc/bash_completion.d/joist
joist completion zsh > "${fpath[1]}/_joist"
```

## Documentation

See [`USAGE.md`](./USAGE.md) for the full command reference and [`AI_INSTRUCTIONS.md`](./AI_INSTRUCTIONS.md) for instructions to add to your agent's system prompt.
