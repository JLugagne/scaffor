# joist AI Instructions

To help AI agents (like Claude, GPT-4, Cursor, or Copilot) use `joist` effectively in your project, copy the following instructions into your project's `.cursorrules`, `AI_INSTRUCTIONS.md`, or system prompt.

---

## Copy below this line

```markdown
# Go Code Editing Rules

You are managing a Go codebase. To read, navigate, and modify Go code, you MUST use the `joist` CLI. DO NOT use generic tools like `cat`, `sed`, `grep`, or output full file replacements using standard diffs, as they lead to indentation errors and context limit exhaustion.

`joist` is a deterministic AST-based byte-range editor. It automatically runs `goimports`, meaning you NEVER need to worry about formatting or import statements when writing code.

## 1. Orientation & Navigation
Always start by exploring the codebase using `joist` rather than reading full files. Use context window management flags to avoid blowing up your token budget on large projects.

- **List all packages:** `joist graph`
- **High-level overview:** `joist graph --summary --depth 2`
- **Zoom into one package:** `joist graph --focus <package_path>` (full detail for the target, path-only for the rest)
- **List all exported symbols in a package:** `joist graph -s -d <relative_dir>`
- **Exclude directories:** `joist graph --exclude vendor --exclude "*legacy*"`
- **Fit within token budget:** `joist graph --summary --deps --token-budget 2000`
- **Read a function, struct, or method:** `joist symbol <Name> --body` (Use `Receiver.Method` for precise method lookups).

## 2. Editing Code
When modifying code, stream the raw Go declaration (without `package` or `import` blocks) via stdin to the specific `joist` subcommand.

**Rules for Editing:**
- Always provide the FULL declaration (complete signature and body).
- Do not add `package` or `import` at the top of your snippets.
- Use `update-func` or `update-struct` to replace existing nodes.
- Use `add-func` or `add-struct` to append to a file.

**Example: Updating a Method**
```bash
cat <<'EOF' | joist update-func --file internal/app/service.go --id "(*Service).DoWork"
func (s *Service) DoWork(ctx context.Context) error {
    // your new logic here
    return nil
}
EOF
```

## 3. Interfaces & Mocks
When creating or modifying interfaces, always use the dedicated interface commands so the mock is generated and kept in sync automatically.

```bash
cat <<'EOF' | joist update-interface --file domain/repo.go --id Repository --mock-file domain/repotest/mock.go --mock-name MockRepository
type Repository interface {
    Save(ctx context.Context, item Item) error
}
EOF
```

To stub out missing methods for an interface you are implementing:
```bash
joist implement io.Reader --receiver "*MyReader" --file reader.go
```

## 4. Scaffolding
If asked to create new features or architectural layers, check if templates are available:
```bash
joist scaffold list-templates
```
If a template exists, use it. Lint it first to catch issues, then read its documentation:
```bash
joist scaffold lint <template_name>
joist scaffold doc <template_name> <command_name>
```
Then execute it, providing the variables:
```bash
joist scaffold execute <template> <command> --set AppName=myapp
```
**CRITICAL:** Scaffolding templates output "Hints" (Task Lists). You MUST read these hints carefully and execute the next steps they suggest.
```
