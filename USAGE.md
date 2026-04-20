# scaffor Usage Guide

This document details all available commands, their flags, and how to use them effectively.

All flags use the standard `--long-name` form. Short forms are noted where available.

Commands:

| Command | Purpose |
|---------|---------|
| [`list`](#list) | list all templates available (from config sources + `.scaffor-templates/`) |
| [`doc`](#doc) | show documentation for a template or a specific command |
| [`execute`](#execute) | execute a template command to scaffold files |
| [`lint`](#lint) | validate a template manifest statically |
| [`test`](#test) | run a template's `test:` block in a temp directory |
| [`mcp`](#mcp) | start scaffor as an MCP server over stdio |
| [`init-config`](#init-config) | create the global config file with a commented example |
| [`edit-config`](#edit-config) | open the global config file in `$EDITOR` |
| [`config`](#config) | print the currently active configuration |

**Global flags** (available on every command):
- `--templates-dir <path>` — use only this directory for template lookup; overrides the global config.
- `--ignore-missing-sources` — tolerate config `template_sources` entries whose directories don't exist.

Templates live in `.scaffor-templates/<template-name>/manifest.yaml` inside each project, or in directories declared by your [global config](#configuration). See [`GENERATE_TEMPLATE.md`](./GENERATE_TEMPLATE.md) for the full authoring guide.

---

## `list`

List all templates available to scaffor. Templates are discovered from every source in your [global config](#configuration) and, when no config is present, from `.scaffor-templates/` in the current directory. Each entry shows the template name, its origin directory, and its description.

```bash
scaffor list
```

Example output:
```
Available Templates:

- hexagonal-cli (from /home/me/work/scaffor-templates):
    Hexagonal architecture template for Go CLI applications.

- godoc-md (from .scaffor-templates):
    Generates Markdown documentation from Go source code.
```

Templates whose manifest fails to parse are reported with a hint to run `scaffor lint`. When the same template name appears in more than one configured source, a warning is written to stderr and the first source wins.

---

## `doc`

Show documentation for a template or a specific command.

```bash
scaffor doc <template>              # list commands + descriptions
scaffor doc <template> <command>    # show variables + post_commands + shell_commands for one command
scaffor doc <template> --all        # show full details for every command at once
scaffor doc <template> -a           # short form of --all
```

**Flags:**
- `--all, -a` — show full details (variables, post_commands, shell_commands) for every command in the template.

Output for a specific command lists:
- Description
- Required variables (as `--set Key=Value`)
- `post_commands` that will chain to other template commands
- `shell_commands` scoped to this command (with their mode)

---

## `execute`

Execute a command defined in a template manifest. Renders all `.tmpl` files to their destinations, then runs any `shell_commands` declared at the command or template level.

```bash
scaffor execute <template> <command> [--set Key=Value ...] [flags]
```

**Flags:**
- `--set Key=Value` — set a variable value. Repeat for each variable. Keys must match those declared in the command's `variables:` block.
- `--dry-run` — render files normally, but **print** `shell_commands` instead of executing them.
- `--skip` — silently skip files that already exist instead of failing.
- `--force` — overwrite files that already exist instead of failing.

**Behavior:**
- If any required variable is missing, execution aborts and prints the full `--set` usage line.
- Pre-flight check: by default, execution aborts if any destination file already exists. Use `--skip`, `--force`, or per-file `on_conflict` in the manifest.
- After files are written, `shell_commands` run automatically (unless `--dry-run`).
- A rendered `hint:` is printed after a successful run to guide the next step.

**Conflict resolution (`on_conflict` per file):**

| Value | Behavior |
|-------|----------|
| `default` (or omitted) | follows global `--skip` / `--force`; blocks if neither is set |
| `skip` | always skip this file silently, regardless of flags |
| `force` | always overwrite this file, regardless of flags |

**Examples:**
```bash
# Scaffold the "service" template's "create" command
scaffor execute service create --set Name=catalog

# Multiple variables
scaffor execute service create --set Name=catalog --set Port=8080

# Print shell_commands without running them
scaffor execute service create --set Name=catalog --dry-run

# Skip or overwrite pre-existing files
scaffor execute service create --set Name=catalog --skip
scaffor execute service create --set Name=catalog --force
```

---

## `lint`

Statically validate a template manifest and report all issues found. Runs in milliseconds — put it in CI.

```bash
scaffor lint <template>
scaffor lint --all
scaffor lint -d <dir> <template>
```

**Flags:**
- `--all` — lint every template in the templates directory; omit the `<template>` argument.
- `--dir, -d <path>` — use a custom templates directory (default: `.scaffor-templates`).

**Checks:**
- All `.tmpl` files, destination paths, hints, and shell commands parse as valid Go `text/template`.
- Every `{{ .Variable }}` used in a destination, `.tmpl` file, or hint is declared in the command's `variables:`.
- Variable keys start with an uppercase letter (required by Go `text/template`).
- `post_commands` reference commands that exist in the same template.
- No cycles in `post_commands` chains.
- `shell_commands.mode` is `all` or `per-file`.
- `shell_commands.pattern` values are valid glob patterns.
- Typo detection via Levenshtein distance (e.g. `ApName` → suggests `AppName`).

**Example:**
```
$ scaffor lint broken-template
LINT ERRORS in broken-template:

  command "create", field "files.destination": variable "Nme" used but not declared (did you mean "Name"?)
  command "create", field "post_commands": references undefined command "add_mock"

2 issue(s) found
```

Non-zero exit code on any issue; suitable for CI.

---

## `test`

Execute a template's `test:` block in a fresh temporary directory, then run its `validate:` block to assert the generated output is correct. Reports command coverage.

```bash
scaffor test <template>
scaffor test --all
```

**Flags:**
- `--all` — test every template that has a `test:` block. Templates without one are skipped silently.

**How it works:**
1. A temp directory is created (`scaffor-test-*`).
2. `.scaffor-templates/<template>/` is copied into it.
3. The working directory changes to the temp dir.
4. Each `test:` step (`{command, params}`) is executed with `force: true` so re-runs are clean.
5. Each `validate:` entry is run via `sh -c` in the temp dir. Any non-zero exit fails the test.
6. Coverage is printed as `N of M commands exercised (P%)`.
7. The temp dir is removed regardless of outcome.

**Example manifest block:**
```yaml
test:
  - command: bootstrap
    params:
      AppName: testapp
      ModulePath: github.com/test/testapp
  - command: add_entity
    params:
      AppName: testapp
      ModulePath: github.com/test/testapp
      Entity: Book

validate:
  - "go mod tidy"
  - "go build ./..."
```

**Typical validation commands:**
- Go: `go mod tidy`, `go build ./...`, `go vet ./...`
- Any: `test -f path/to/expected_file` to assert a file was generated.

Non-zero exit on any failure — use this in CI alongside `scaffor lint --all`.

---

## `mcp`

Start scaffor as a [Model Context Protocol](https://modelcontextprotocol.io) server over stdio. MCP-aware clients (Claude Code, Cursor, etc.) can then call scaffor's tools directly.

```bash
scaffor mcp
```

**Tools exposed:**

| Tool | Purpose |
|------|---------|
| `list` | list all templates in `.scaffor-templates/` |
| `doc` | show documentation for a template or a specific command (supports `all=true`) |
| `execute` | execute a template command (supports `dry_run`, `skip`, `force`) |
| `lint` | validate a template manifest (supports `all=true`, custom `dir`) |
| `test` | run a template's `test:` block in a temp directory |
| `status` | review the session log of tool calls and file events |

Each session is logged to `.scaffor/<session-id>.jsonl` — the `status` tool lets the agent review which files were created, overwritten, or skipped.

**Client configuration (Claude Code example):**
```json
{
  "mcpServers": {
    "scaffor": {
      "command": "scaffor",
      "args": ["mcp"]
    }
  }
}
```

Upon connection, the server advertises instructions telling the agent to always call `list` before writing files manually.

---

## `init-config`

Create the global config file with a commented example. Writes to `$XDG_CONFIG_HOME/scaffor/config.yml` when set, otherwise `~/.config/scaffor/config.yml`.

```bash
scaffor init-config
scaffor init-config --force   # overwrite an existing config
```

**Flags:**
- `--force` — overwrite an existing config file. Without this flag, the command fails if the file already exists.

The generated file declares no active `template_sources`, so scaffor behavior is unchanged until you edit it.

---

## `edit-config`

Open the global config file in `$EDITOR` (falling back to `$VISUAL`). If the file doesn't exist yet, it's created with the same commented example that `init-config` writes, so the editor always has something to open.

```bash
EDITOR=vim scaffor edit-config
EDITOR="code --wait" scaffor edit-config
```

The editor string may include arguments — they are passed to the editor process verbatim, with the config path appended as the final argument. Fails with an explicit error if neither `$EDITOR` nor `$VISUAL` is set.

---

## `config`

Print the currently active configuration: the config file path (or that it was not found), the resolved template sources, the templates discovered in each source, collisions, and any missing source directories.

```bash
scaffor config
```

Example output:
```
Config file: /home/me/.config/scaffor/config.yml

Template sources:
  [1] /home/me/work/scaffor-templates (Personal templates)
      - hexagonal-cli
      - go-service
  [2] /home/me/work/team-scaffor-templates (Shared)
      - react-component

No collisions.
No missing sources.
```

Use `scaffor config` to verify that a newly-edited config is being picked up as expected before running `list` / `doc` / `execute`.

---

## Configuration

scaffor reads a global configuration file that declares directories to scan for templates.

**Location:**
- `$XDG_CONFIG_HOME/scaffor/config.yml` when `XDG_CONFIG_HOME` is set.
- `~/.config/scaffor/config.yml` otherwise.

**Format:**
```yaml
template_sources:
  - path: ~/work/scaffor-templates
    description: Personal templates
  - path: ~/work/team-scaffor-templates
    description: Shared team templates
  - path: $SCAFFOR_SHARED/templates
```

**Path rules:**
- `~` expands to `$HOME`.
- `$VAR` and `${VAR}` expand from the environment.
- Relative paths are rejected with an explicit error (no implicit base directory).

**Resolution order:**
1. If `--templates-dir <path>` is passed on the command line, use it exclusively.
2. Otherwise, scan `template_sources` in declaration order. First match wins.
3. Otherwise (no config file or empty `template_sources`), fall back to `.scaffor-templates/` in the current directory — unchanged legacy behavior.

**Collisions:** when the same template name is found in more than one source, scaffor uses the first one and writes a warning to stderr listing the shadowed sources. Check `scaffor config` to see collisions without running anything.

**Missing sources:** by default, scaffor fails at startup if any declared source directory is missing. Pass `--ignore-missing-sources` to skip missing entries (useful if one of your template repos is on an unmounted drive).

### Troubleshooting

- **Config file not loading.** Run `scaffor config`. The first line shows the resolved path and whether scaffor found a file there. If the file exists but isn't being read, check `XDG_CONFIG_HOME` — it takes precedence over `~/.config`.
- **"template sources missing" error.** A `path:` entry points at a directory that doesn't exist on disk. Fix the path or pass `--ignore-missing-sources`.
- **"path must be absolute after expansion" error.** A relative path slipped through — remember, relative paths are rejected. Use `~`, `$HOME/…`, or an absolute path.
- **Unexpected template chosen on collision.** Reorder `template_sources` so the preferred source comes first. `scaffor config` lists the winning source for each name.
- **`--templates-dir` doesn't seem to override.** It must be passed **before** the subcommand: `scaffor --templates-dir ./mytpls list`.

---

## Template manifest reference

Full schema with every supported field:

```yaml
name: <template-name>
description: |
  Human-readable description of the template.

commands:
  - command: <command-name>
    description: What this command does
    variables:
      - key: AppName                 # Must start with an uppercase letter
        description: The application name
    files:
      - source: path/to/file.tmpl    # relative to template dir; omit for empty file
        destination: path/to/{{ .AppName }}/file  # rendered as a template
        on_conflict: default         # "default" (follows --skip/--force), "skip", or "force"
    post_commands:
      - other_command_name           # Chain to another command in this template
    shell_commands:                  # Per-command, rendered with this command's variables
      - command: "goimports -w internal/{{ .AppName | lower }}/file.go"
        mode: all                    # "all" (default) or "per-file"
        pattern: "*.go"              # Optional glob filter
        silent: false                # When true, only "Success" or error is printed
    hint: |
      Message printed after execution. Rendered as a template.
      Next: run scaffor execute <template> next-command --set AppName={{ .AppName }}

shell_commands:                      # Template-level, runs after every execute in this template
  - command: "goimports -w {{ .Files }}"
    mode: all                        # "all" (default) — run once; or "per-file"
    pattern: "*.go"                  # Optional: comma-separated glob patterns
    silent: false

test:                                # Executed by `scaffor test <template>`
  - command: bootstrap
    params:
      AppName: testapp
      ModulePath: github.com/test/testapp

validate:                            # Shell commands run after all test steps in the temp dir
  - "go mod tidy"
  - "go build ./..."
```

### Shell command modes

| Mode | Behavior | Template variables |
|------|----------|--------------------|
| `all` *(default)* | Run once after all files are written | `{{ .Files }}` — space-joined list of matching files |
| `per-file` | Run once per matching file | `{{ .File }}` — individual path; `{{ .Files }}` — all matching |

**`pattern`** filters which files the shell command sees. Patterns match against the **filename** only (not the full path). Multiple globs are comma-separated: `"*.go"`, `"*.js,*.tsx"`, `"*_test.go"`. If no files match, the command is skipped silently.

**`silent: true`** suppresses the usual `Execute command: …` / `Result: …` output and prints only `Success` on success or the error on failure.

### Template functions

scaffor uses the full [Sprig](https://masterminds.github.io/sprig/) library — 100+ pipe functions. Commonly useful:

| Function | Effect | Example |
|----------|--------|---------|
| `lower` | lowercase | `{{ .Entity \| lower }}` → `user` |
| `upper` | UPPERCASE | `{{ .Entity \| upper }}` → `USER` |
| `title` | Title Case | `{{ .Command \| title }}` → `Create` |
| `camelcase` | camelCase | `{{ .Entity \| camelcase }}` → `orderItem` |
| `snakecase` | snake_case | `{{ .Entity \| snakecase }}` → `order_item` |
| `kebabcase` | kebab-case | `{{ .Entity \| kebabcase }}` → `order-item` |
| `plural` | pluralize | `{{ .Entity \| plural }}` → `orders` |
| `default` | fallback | `{{ .Port \| default "8080" }}` |
| `splitList` | split to list (ordered) | `{{ range (splitList "," .Events) }}…{{ end }}` |

> **Gotcha:** use `splitList` (returns `[]string`, order-preserving), **not** `split` (returns `map[string]string`, non-deterministic).

See https://masterminds.github.io/sprig/ for the full function list.

---

## Best practices for AI agents

1. **Discover first.** Always call `scaffor list` before writing files manually — a template may already exist.
2. **Read before execute.** `scaffor doc <template>` or `scaffor doc <template> --all` shows required variables.
3. **Lint in CI.** `scaffor lint --all` catches typos, undeclared variables, and broken references in milliseconds.
4. **Test in CI.** `scaffor test --all` exercises every template end-to-end in a temp dir.
5. **Follow hints.** After execution, hints tell you exactly what to do next — read them carefully.
6. **Prefer granular commands.** Many small commands (one `add_entity`, one `add_command`) beat one monolithic generator.
7. **Use `on_conflict` for idempotency.** Mark editable files as `skip` and fully-generated ones as `force` so re-runs never clobber user work.
