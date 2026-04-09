# joist Usage Guide

This document details all available commands, their flags, and how to use them effectively.

All flags use the standard `--long-name` form.

---

## Scaffolding (`scaffold`)

Template-driven project orchestration. Templates define a set of commands, variables, and file generation rules. They also support `post_commands` (shell commands to run after files are written) and contextual `hints` to guide users or AI agents through the next steps of development.

Templates are stored in `.joist-templates/<template-name>/manifest.yaml`. See `README.md` for a guide on creating your own templates.

```bash
# List all available templates
joist list

# Lint a template manifest for issues before executing
joist lint <template>

# Show documentation for a template (lists all its commands)
joist doc <template>

# Show documentation for a specific command (shows required variables and post_commands)
joist doc <template> <command>

# Execute a command (post_commands are printed for manual review)
joist execute <template> <command> [--set Key=Value ...]

# Execute a command and run post_commands automatically via shell
joist execute <template> <command> [--set Key=Value ...] --run-commands
```

**Examples:**
```bash
# See what templates are available
joist list

# Validate the 'hexagonal' template before running it
joist lint hexagonal

# Read what the 'bootstrap' command does
joist doc hexagonal bootstrap

# Execute it, passing the required variables
joist execute hexagonal bootstrap --set AppName=catalog --set ModulePath=github.com/myorg/myapp

# Execute and run post_commands automatically
joist execute hexagonal bootstrap --set AppName=catalog --run-commands
```

### Lint

`joist lint <template>` validates a template manifest and reports all issues found:

- **Invalid post_commands** — a post_command has an empty `command` or an invalid `mode` (must be `"all"` or `"per-file"`)
- **Undeclared variables in destination paths** — `{{ .Foo }}` used in a `files.destination` but `Foo` is not in the command's `variables` list
- **Undeclared variables in source templates** — `{{ .Foo }}` used inside a template file but `Foo` is not declared

```
$ joist lint hexagonal
OK: hexagonal has no issues

$ joist lint broken-template
LINT ERRORS in broken-template:

  command "create", field "post_commands": post_command[0] has an empty command
  command "create", field "post_commands": post_command[1] has invalid mode "unknown" (must be "all" or "per-file")
  command "create", field "files.destination": variable "Missing" used but not declared
  command "create", field "files.source:file.tmpl": variable "AlsoMissing" used but not declared

4 issue(s) found
```

### Post-commands

After files are written, `post_commands` are resolved and either printed or executed:

**Default (print for review):**
```
Post-commands to run:
  go fmt ./...
  goimports -w internal/catalog/domain/domain.go internal/catalog/domain/repo.go

Run with --run-commands to execute them automatically.
```

**With `--run-commands`:**
```
Running post-commands:
  $ go fmt ./...
  $ goimports -w internal/catalog/domain/domain.go internal/catalog/domain/repo.go
```

**Post-command modes:**

| Mode | Behavior | Template variables |
|------|----------|--------------------|
| `all` *(default)* | Run once after all files are written | `{{ .Files }}` — space-joined list of all created files |
| `per-file` | Run once per created file | `{{ .File }}` — individual file path; `{{ .Files }}` — all files |

**Manifest example:**
```yaml
post_commands:
  - command: "go fmt {{ .Files }}"
    mode: all
  - command: "goimports -w {{ .File }}"
    mode: per-file
```

### Template manifest reference

```yaml
name: <template-name>
description: |
  Human-readable description of the template.

commands:
  - command: <command-name>
    description: What this command does
    variables:
      - key: AppName
        description: The application name
    files:
      - source: path/to/file.tmpl        # relative to template dir; omit for empty file
        destination: path/to/{{ .AppName }}/file  # rendered as a template
    post_commands:
      - command: "some-tool {{ .Files }}"
        mode: all        # or "per-file"
    hint: |
      Message printed after execution. Rendered as a template.
      Next: run joist execute <template> next-command --set AppName={{ .AppName }}
```

### Template functions

| Function | Description | Example |
|----------|-------------|---------|
| `lower` | Lowercase | `{{ .AppName \| lower }}` |
| `upper` | Uppercase | `{{ .AppName \| upper }}` |
| `title` | Capitalize first letter | `{{ .AppName \| title }}` |

### Best practices for AI agents

1. **Always lint first:** `joist lint <template>` before executing
2. **Read doc before execute:** `joist doc <template> <command>` shows required variables
3. **Follow hints:** After execution, hints tell you what to do next — read them carefully
4. **Granular commands:** Prefer many small commands over one large generator
5. **Safety:** Pre-flight checks abort if any destination file already exists
