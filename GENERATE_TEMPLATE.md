# Generate Scaffor Template from Existing Code

Step-by-step instructions for creating a `.scaffor-templates/<name>/manifest.yaml` and its `.tmpl` files by extracting patterns from existing source code.

## Overview

A scaffor template is a reusable scaffold. It lives in `.scaffor-templates/<template-name>/` and contains:

```
.scaffor-templates/<template-name>/
├── manifest.yaml          # Commands, variables, file mappings, hints
├── some/path/file.go.tmpl # Template files with Go text/template syntax
└── another/file.tmpl
```

The goal: take concrete code that already works, identify the parts that change between uses, and replace them with template variables.

---

## Step 1: Identify the Pattern

Read the existing code the user wants to templatize. Look for:

- **Repeated structure** — files that follow the same layout across packages or entities
- **Naming patterns** — identifiers, package names, file paths that follow a convention
- **Variable parts** — names, paths, import prefixes that change per use

Ask yourself: "If someone wanted to add another one of these, what would they copy-paste and find-replace?"

## Step 2: Choose Variables

Extract the moving parts into variables. Rules:

- Variable keys **must start with an uppercase letter** (Go `text/template` requirement)
- Keep variable names short and descriptive
- Use PascalCase: `AppName`, `ModulePath`, `Entity`, `Adapter`
- Every variable needs a `description` field explaining what it is

Common variables:

| Variable | Typical use |
|----------|-------------|
| `AppName` | Application/project name for folder paths and package names |
| `ModulePath` | Go module path for imports (e.g. `github.com/org/repo`) |
| `Entity` | Domain entity name in PascalCase (e.g. `User`, `Order`) |

## Step 3: Write Template Files (.tmpl)

Take each source file that's part of the pattern and create a `.tmpl` copy. Replace the variable parts with Go `text/template` placeholders.

### Available Functions

Templates use the full [Sprig](https://masterminds.github.io/sprig/) function library, giving you 100+ pipe functions out of the box. Some commonly useful ones:

| Function | Effect | Example |
|----------|--------|---------|
| `lower` | lowercase | `{{ .Entity \| lower }}` → `user` |
| `upper` | UPPERCASE | `{{ .Entity \| upper }}` → `USER` |
| `title` | Title Case | `{{ .Command \| title }}` → `Create` |
| `camelcase` | camelCase | `{{ .Entity \| camelcase }}` → `orderItem` |
| `snakecase` | snake_case | `{{ .Entity \| snakecase }}` → `order_item` |
| `kebabcase` | kebab-case | `{{ .Entity \| kebabcase }}` → `order-item` |
| `replace` | string replace | `{{ .Name \| replace "-" "_" }}` |
| `trim` | strip whitespace | `{{ .Name \| trim }}` |
| `default` | fallback value | `{{ .Port \| default "8080" }}` |
| `plural` | pluralize | `{{ .Entity \| plural }}` → `orders` |
| `contains` | substring test | `{{ if contains "admin" .Role }}...{{ end }}` |

See the full list at https://masterminds.github.io/sprig/ — all string, math, date, regex, list, and dict functions are available.

> **`splitList` vs `split` gotcha:** To iterate over a comma-separated variable, use `splitList` (returns `[]string`, order-preserving), **not** `split` (returns `map[string]string`, non-deterministic order). Also note that sprig functions take the **separator first**: `splitList "," .Events`, not `splitList .Events ","`. The pipe form `.Events | splitList ","` also works.
>
> ```
> {{ range (splitList "," .Events) }}{{ . | trim }}{{ end }}
> ```

### Conversion Example

**Before** — concrete source file `internal/myapp/domain/order.go`:
```go
package domain

type Order struct {
    ID OrderID
}

type OrderID string

func NewOrder() (*Order, error) {
    return &Order{}, nil
}
```

**After** — template file `internal/appname/domain/entity.go.tmpl`:
```go
package domain

type {{ .Entity }} struct {
    ID {{ .Entity }}ID
}

type {{ .Entity }}ID string

func New{{ .Entity }}() (*{{ .Entity }}, error) {
    return &{{ .Entity }}{}, nil
}
```

### Guidelines for .tmpl Files

- Keep the file structure mirroring the real output structure (use generic placeholder names in the path like `appname/`, `entity/`)
- Template both identifiers **and** import paths: `"{{ .ModulePath }}/internal/{{ .AppName }}/domain"`
- Use pipe functions in package declarations: `package {{ .Entity | lower }}`
- Do NOT template things that stay constant across uses (standard library imports, framework boilerplate)
- `.go` files are automatically formatted with `goimports` after generation — don't worry about import ordering

## Step 4: Write the manifest.yaml

### Full Schema

```yaml
name: template-name
description: |
  What this template produces and when to use it.
  Multi-line description.

commands:
  - command: command_name
    description: What this command does
    variables:
      - key: VariableName        # Must start uppercase
        description: what this variable represents
    files:
      - source: path/to/file.tmpl           # Relative to template dir; omit for empty file
        destination: path/to/{{ .Var }}/out  # Output path, supports template syntax
        on_conflict: default                 # "default", "skip", or "force" (optional)
    post_commands:
      - other_command_name       # Chain to another command in this template
    shell_commands:              # Optional, per-command shell commands (same format as template-level)
      - command: "goimports -w internal/{{ .AppName | lower }}/domain/{{ .Entity | lower }}.go"
    hint: |
      Message printed after execution.
      Supports {{ .Variables }} too.

  - command: another_command
    # ...

shell_commands:                  # Optional, template-level, runs after all files are written
  - command: "goimports -w {{ .Files }}"
    mode: all                    # "all" (default) or "per-file"
    pattern: "*.go"              # Optional: comma-separated glob patterns
```

### Key Fields

**`commands`** — Each command is an independent scaffolding action. A template can have multiple commands (e.g. `bootstrap` for initial setup, `add_entity` for incremental additions).

**`files`** — Maps template source to output destination.
- `source`: path to the `.tmpl` file relative to the template directory. Omit to create an empty file.
- `destination`: output file path. Supports full template syntax including pipe functions.
  - Example: `internal/{{ .AppName }}/domain/{{ .Entity | lower }}.go`
- `on_conflict`: controls what happens when the destination file already exists. Values:
  - `default` (or omitted) — follows the global `--skip`/`--force` flags; **blocks** if neither is set
  - `skip` — always skip this file silently, regardless of flags (pre-flight check is also skipped)
  - `force` — always overwrite this file, regardless of flags (pre-flight check is also skipped)

  **Choosing the right value per file kind:**
  - Use `skip` for files the user is expected to edit after generation (domain entities, app services, adapters) — re-running the template should never clobber their work.
  - Use `force` for files that are fully generated from a source of truth (mocks, contract test stubs, generated config).
  - Use `default` (or omit) when neither rule applies and the user should decide at execution time.

**`variables`** — Declared per-command. Each variable used in destinations, sources, or hints must be declared here.

**`post_commands`** — List of other command names in the same template to execute after this one. Executed depth-first. No cycles allowed.

**`shell_commands`** (per-command) — Same format as template-level `shell_commands` (with `command`, `mode`, `pattern` fields), but rendered with the command's variables instead of `{{ .File }}`/`{{ .Files }}`. Runs after this command's files are written. Useful for formatting, code generation, or any post-processing specific to a command.

**`hint`** — Rendered after execution. Use it to tell the user (or an AI agent) what was created and what to do next. Supports template syntax.

**`shell_commands`** — Template-level (not per-command). Runs after all files are written.
- `command`: The shell command to run. Supports `{{ .File }}` and `{{ .Files }}` placeholders.
- `mode`: Execution strategy.
  - `all` (default) — runs once; `{{ .Files }}` expands to space-separated list of matching files.
  - `per-file` — runs once per matching file; `{{ .File }}` is the current file.
- `pattern` (optional) — Comma-separated glob patterns to filter which created files the command operates on. Examples: `*.go`, `*.js,*.tsx`, `*_test.go`. If omitted, all created files are included.
  - Patterns match against the **filename** (not the full path).
  - If no files match, the shell command is skipped silently.
- Shell commands are printed for review by default. Only executed when the user passes `--run-commands`.

#### Shell command examples

Format only Go files:
```yaml
shell_commands:
  - command: "gofmt -w {{ .Files }}"
    pattern: "*.go"
```

Run prettier per-file on JS/TS:
```yaml
shell_commands:
  - command: "prettier --write {{ .File }}"
    mode: per-file
    pattern: "*.js,*.jsx,*.ts,*.tsx"
```

Run on all files (default when no pattern):
```yaml
shell_commands:
  - command: "echo {{ .Files }}"
```

### Design Commands Around User Actions

Think about how the template will be used over time:

1. **Bootstrap command** — one-time setup that creates the initial skeleton
2. **Incremental commands** — add a single entity, adapter, route, etc. to an existing project
3. **Chain with post_commands** — if a bootstrap always needs a sub-step, chain it

## Step 5: Write Useful Hints

Hints are the most underrated part of a template. They tell the user (or an AI agent acting on their behalf) exactly what to do after scaffolding.

A good hint includes:

1. **What was created** — list the generated files with a short description of each
2. **Next steps** — concrete actions to take, in order
3. **Follow-up commands** — if the template has incremental commands, show how to use them

Hints support template variables, so personalize them:

```yaml
hint: |
  Entity {{ .Entity }} added to {{ .AppName }}.

  Created:
    internal/{{ .AppName }}/domain/{{ .Entity | lower }}.go             — entity definition
    internal/{{ .AppName }}/outbound/{{ .Entity | lower }}/             — repository adapter stub

  NEXT STEPS:
    1. Define {{ .Entity }} fields in domain/{{ .Entity | lower }}.go
    2. Implement the adapter in outbound/{{ .Entity | lower }}/
    3. Wire the handler in internal/{{ .AppName }}/init.go
```

## Step 6: Validate with Lint

**IMPORTANT: Always run the linter before considering a template done.**

```bash
scaffor lint <template-name>
```

The linter checks:

- All `.tmpl` files, destination paths, hints, and shell commands parse as valid Go text/templates
- Variable keys start with uppercase
- All variables used in destinations, `.tmpl` files, and hints are declared
- Post-commands reference existing commands in the same template
- No cycles in post_command chains
- Shell command `mode` is `all` or `per-file`
- Shell command `pattern` values are valid glob patterns
- Typo detection with suggestions (e.g. "ApName" → did you mean "AppName"?)

Fix every lint error before considering the template done.

## Step 7: Test Execution

Dry-run the template:

```bash
scaffor doc <template-name>                    # See available commands and variables
scaffor doc <template-name> <command>           # See required variables for a command
scaffor execute <template-name> <command> --set Key=Value --set Key2=Value2
```

Verify:
- All files are created at the expected paths
- Template variables are substituted correctly
- Import paths are valid
- Hints display the right information
- Post-commands execute in the right order

---

## Complete Workflow Example

**Goal**: the user has a Go project with `internal/myapp/domain/order.go`, `internal/myapp/domain/repositories/order/order.go`, and `internal/myapp/outbound/order/order.go`. They want a template so they can add new entities with the same structure.

1. Read all three files, identify what changes per entity (`Order` → variable, `myapp` → variable, module path → variable)
2. Create `.scaffor-templates/my-template/manifest.yaml` with an `add_entity` command declaring `AppName`, `ModulePath`, `Entity`
3. Copy each file as `.tmpl`, replacing `Order` with `{{ .Entity }}`, `order` with `{{ .Entity | lower }}`, `myapp` with `{{ .AppName }}`, and the module path with `{{ .ModulePath }}`
4. Set destinations using template syntax: `internal/{{ .AppName }}/domain/{{ .Entity | lower }}.go`
5. Write a hint listing what was created and next steps
6. Run `scaffor lint my-template` and fix any issues
7. Test with `scaffor execute my-template add_entity --set AppName=myapp --set ModulePath=github.com/org/repo --set Entity=Product`

---

## Common Pitfalls

- **Forgetting to declare a variable** — every `{{ .Something }}` in a destination path, `.tmpl` file, or hint must have a matching entry in `variables`. The linter catches this.
- **Lowercase variable keys** — `{{ .entity }}` won't work. Use `{{ .Entity }}` and pipe to `lower` when needed.
- **Over-templating** — don't replace constants with variables. If every entity lives under `internal/`, leave `internal/` hardcoded.
- **Path traversal** — destinations containing `..` are rejected. All paths must be relative and downward.
- **Existing files** — execution aborts if any destination file already exists (pre-flight check). Use `--skip` to skip, `--force` to overwrite, or set `on_conflict` per file in the manifest to control behaviour at the template level.
- **Skipping the linter** — always run `scaffor lint <template-name>` after writing or modifying a template. It catches syntax errors, undeclared variables, invalid patterns, and typos before execution.
