# joist AI Instructions

To help AI agents (like Claude, GPT-4, Cursor, or Copilot) use `joist` effectively in your project, copy the following instructions into your project's `.cursorrules`, `AI_INSTRUCTIONS.md`, or system prompt.

---

## Copy below this line

```markdown
# Scaffolding Rules

You are working in a project that uses `joist` for template-driven scaffolding. Before creating files manually, ALWAYS check if a template exists that can do it better.

## 1. Discovering Templates

Always start by listing available templates:
```bash
joist list
```

Read the documentation for a template before executing it:
```bash
# All commands in a template
joist doc <template_name>

# Specific command (shows required variables and post_commands)
joist doc <template_name> <command_name>
```

## 2. Linting Before Executing

Always lint a template before running it:
```bash
joist lint <template_name>
```

## 3. Executing Scaffolding

```bash
joist execute <template> <command> --set Key=Value [--set Key2=Value2 ...]
```

Example:
```bash
joist doc hexagonal bootstrap
joist execute hexagonal bootstrap \
  --set AppName=catalog \
  --set ModulePath=github.com/myorg/myapp
```

After files are written, post_commands are printed for review. Pass `--run-commands` to execute them automatically via the shell.

## 4. Following Hints — THIS IS CRITICAL

After every scaffold execution, hints are printed to the console. **You MUST read these hints carefully and execute the next steps they suggest.**

Hints are the template author's designed workflow. Do not improvise a different approach when a hint tells you what to do next.

## 5. Template Variables

Variables are passed via `--set Key=Value` flags and available in templates as `{{ .Key }}`.

Available template functions:
- `{{ .AppName | lower }}` — lowercase
- `{{ .AppName | upper }}` — UPPERCASE
- `{{ .AppName | title }}` — Title Case

## 6. Decision Tree

When asked to create new files or project structure:

```
1. Is joist available?
   NO  → Create files manually
   YES → Continue

2. Does the task involve creating multiple related files / architectural layers?
   NO  → Create files manually
   YES → Continue

3. Run: joist list
   Templates exist for this task?
   NO  → Create files manually (or create a new template if the pattern is reusable)
   YES → Continue

4. Run: joist doc <template> <command>
   Command matches the need?
   YES → Lint, execute, and follow hints
   NO  → Create files manually or add a new command to the template
```
```
