package mcp

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/JLugagne/scaffor/internal/scaffor/domain"
	"github.com/JLugagne/scaffor/internal/scaffor/domain/service"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

type docInput struct {
	Template string `json:"template" jsonschema:"name of the template directory under .scaffor-templates/ (e.g. \"service\")"`
	Command  string `json:"command,omitempty" jsonschema:"command name within the template; omit to list all commands, provide to get the variables and post_commands for that specific command"`
	All      bool   `json:"all,omitempty" jsonschema:"when true and command is omitted, show full details (variables, post_commands) for every command in the template at once"`
}

type executeInput struct {
	Template string            `json:"template" jsonschema:"name of the template directory under .scaffor-templates/ (e.g. \"service\")"`
	Command  string            `json:"command" jsonschema:"command name to execute within the template (e.g. \"create\"); call doc first to discover available commands"`
	Params   map[string]string `json:"params,omitempty" jsonschema:"variables as key→value pairs; keys are case-sensitive and must start with a capital letter (e.g. {\"Name\": \"catalog\", \"Port\": \"8080\"}); call doc(template, command) to discover required variables"`
	DryRun   bool              `json:"dry_run,omitempty" jsonschema:"when true, shell_commands are printed but not executed; when false (default) they are executed automatically"`
	Skip     bool              `json:"skip,omitempty" jsonschema:"when true, silently skip files that already exist instead of failing"`
	Force    bool              `json:"force,omitempty" jsonschema:"when true, overwrite files that already exist instead of failing"`
	// optional working directory for the execute command; defaults to the MCP client's project root
	Dir string `json:"dir,omitempty" jsonschema:"optional working directory; defaults to the MCP client project root (cwd)"`
}

type lintInput struct {
	Template string `json:"template,omitempty" jsonschema:"name of the template directory under .scaffor-templates/ to validate; required unless all is true"`
	Dir      string `json:"dir,omitempty" jsonschema:"directory containing templates; overrides the default .scaffor-templates/ search path"`
	All      bool   `json:"all,omitempty" jsonschema:"when true, lint every template in the directory; template is ignored"`
}

type batchItem struct {
	Command string            `json:"command" jsonschema:"command name within the template"`
	Params  map[string]string `json:"params,omitempty" jsonschema:"variables for this command; keys are case-sensitive and must start with a capital letter"`
}

type batchExecuteInput struct {
	Template string      `json:"template" jsonschema:"name of the template directory under .scaffor-templates/"`
	Steps    []batchItem `json:"steps" jsonschema:"ordered list of {command, params} pairs to execute sequentially"`
	Skip     bool        `json:"skip,omitempty" jsonschema:"when true, silently skip files that already exist instead of failing"`
	Force    bool        `json:"force,omitempty" jsonschema:"when true, overwrite files that already exist instead of failing"`
	// optional working directory for all steps; defaults to the MCP client's project root
	Dir string `json:"dir,omitempty" jsonschema:"optional working directory for all steps; defaults to the MCP client project root (cwd)"`
}

// NewServer creates an MCP server exposing scaffor's scaffolding tools.
func NewServer(factory ScaffolderFactory) *sdkmcp.Server {
	session, err := NewSession()
	if err != nil {
		// If we can't create a session log, proceed without one.
		session = nil
	}

	server := sdkmcp.NewServer(&sdkmcp.Implementation{
		Name:    "scaffor",
		Title:   "scaffor — template-driven scaffolding",
		Version: "1.0.0",
	}, &sdkmcp.ServerOptions{
		Instructions: `scaffor manages code generation for this project through templates in .scaffor-templates/.

IMPORTANT: before creating any new files, components, services, handlers, or project
structure, always call list first to check whether a template already exists for it.
Use the template if one matches — only write files manually when none does.

Workflow:
  list                          → discover available templates
  doc(template)                 → list its commands and descriptions
  doc(template, all=true)       → get variables for ALL commands at once (preferred)
  doc(template, command)        → get the exact variables for one command
  execute(template, command, …) → scaffold the files (use for a single command)
  batch_execute(template, steps) → run many commands in one call (use when N > 1, e.g. 23 migrations)
  lint(template)                → validate a manifest after editing it
  test(template)                → run the template's test block in a temp directory
  status                        → review the session log of all tool calls and file events

When you need to call the same command repeatedly (e.g. add_migration × N), always use
batch_execute with all N steps in one call instead of N separate execute calls.

Status: every tool call is logged to a .scaffor/<session-id>.jsonl file for the duration of
this MCP session. Call status at any time to review what has been done so far, including
which files were created, overwritten, or skipped during execute calls.`,
	})

	registerListTemplates(server, factory, session)
	registerDocTemplate(server, factory, session)
	registerExecuteTemplate(server, factory, session)
	registerBatchExecute(server, factory, session)
	registerLintTemplate(server, factory, session)
	registerTestTemplate(server, factory, session)
	registerStatus(server, session)

	return server
}

// Serve runs the MCP server over stdio.
func Serve(ctx context.Context, factory ScaffolderFactory) error {
	return NewServer(factory).Run(ctx, &sdkmcp.StdioTransport{})
}

func logCall(session *Session, tool string, params map[string]any, events []FileEvent) {
	if session == nil {
		return
	}
	_ = session.Log(tool, params, events)
}

func registerListTemplates(server *sdkmcp.Server, factory ScaffolderFactory, session *Session) {
	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name: "list",
		Description: `ALWAYS call this tool before creating any new files, components, services, handlers,
repositories, or any repeating project structure. scaffor may already have a template that
does it correctly and consistently — skip this check only if you are certain no template
could apply.

Lists all scaffolding templates available in the project's .scaffor-templates/ directory.
Each template is a named subdirectory containing a manifest.yaml that defines commands,
the variables they require, and the files they create.

When a relevant template exists, use it instead of writing files by hand:
  1. list                          — discover available templates  (this tool)
  2. doc(template)                 — see its commands and descriptions
  3. doc(template, command)        — get required variables for a command
  4. execute(template, command, …) — scaffold the files

Only create files manually if no template covers the use case.`,
	}, func(ctx context.Context, req *sdkmcp.CallToolRequest, _ struct{}) (*sdkmcp.CallToolResult, any, error) {
		logCall(session, "list", nil, nil)
		s, err := resolveScaffolder(ctx, req, factory)
		if err != nil {
			return errResult(err.Error()), nil, nil
		}
		templates, err := s.ListTemplates(ctx)
		if err != nil {
			return errResult(err.Error()), nil, nil
		}
		if len(templates) == 0 {
			return textResult("No templates found."), nil, nil
		}
		var sb strings.Builder
		sb.WriteString("Available Templates:\n")
		for _, tmpl := range templates {
			fmt.Fprintf(&sb, "\n- %s", tmpl.Name)
			if tmpl.Source != "" {
				fmt.Fprintf(&sb, " (from %s)", tmpl.Source)
			}
			sb.WriteString(":\n")
			if len(tmpl.Commands) == 0 && tmpl.Description == "" {
				sb.WriteString("    (manifest has errors — call lint to see details)\n")
				continue
			}
			desc := strings.TrimSpace(tmpl.Description)
			if desc != "" {
				for _, line := range strings.Split(desc, "\n") {
					fmt.Fprintf(&sb, "    %s\n", line)
				}
			}
		}
		return textResult(sb.String()), nil, nil
	})
}

func writeCommandDetail(sb *strings.Builder, cmd domain.TemplateCommand) {
	fmt.Fprintf(sb, "Command: %s\n", cmd.Command)
	fmt.Fprintf(sb, "  %s\n\n", cmd.Description)
	if len(cmd.Variables) > 0 {
		sb.WriteString("  Variables:\n")
		for _, v := range cmd.Variables {
			fmt.Fprintf(sb, "    %s\t%s\n", v.Key, v.Description)
		}
	} else {
		sb.WriteString("  Variables: None\n")
	}
	if len(cmd.PostCommands) > 0 {
		sb.WriteString("\n  Post-commands:\n")
		for _, pc := range cmd.PostCommands {
			fmt.Fprintf(sb, "    → %s\n", pc)
		}
	}
	if len(cmd.ShellCommands) > 0 {
		sb.WriteString("\n  Shell commands:\n")
		for _, sc := range cmd.ShellCommands {
			mode := sc.Mode
			if mode == "" {
				mode = "all"
			}
			fmt.Fprintf(sb, "    [%s] $ %s\n", mode, sc.Command)
		}
	}
}

func registerDocTemplate(server *sdkmcp.Server, factory ScaffolderFactory, session *Session) {
	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name: "doc",
		Description: `Show documentation for a scaffor template or one of its commands.

Three modes depending on which arguments you provide:

  template only — lists every command the template exposes (name + description) and any
    shell_commands that run automatically after execution.

  template + all=true — shows full details (variables, post_commands) for every command
    in the template at once, so you don't need to call doc per command.

  template + command — shows the full detail for a single command: its description, every
    variable you must supply via params when calling execute, and the post_commands.

Call this before execute whenever you need to know which params are required.
Prefer all=true when you need an overview of the entire template.`,
	}, func(ctx context.Context, req *sdkmcp.CallToolRequest, args docInput) (*sdkmcp.CallToolResult, any, error) {
		logCall(session, "doc", map[string]any{"template": args.Template, "command": args.Command, "all": args.All}, nil)
		s, err := resolveScaffolder(ctx, req, factory)
		if err != nil {
			return errResult(err.Error()), nil, nil
		}
		tmpl, err := s.GetTemplate(ctx, args.Template)
		if err != nil {
			return errResult(err.Error()), nil, nil
		}

		var sb strings.Builder
		if args.Command == "" && !args.All {
			fmt.Fprintf(&sb, "Template: %s\n", tmpl.Name)
			fmt.Fprintf(&sb, "%s\n\n", tmpl.Description)
			sb.WriteString("Commands:\n")
			for _, c := range tmpl.Commands {
				fmt.Fprintf(&sb, "  %s - %s\n", c.Command, c.Description)
			}
			if len(tmpl.ShellCommands) > 0 {
				sb.WriteString("\nShell commands (run after any execute):\n")
				for _, sc := range tmpl.ShellCommands {
					mode := sc.Mode
					if mode == "" {
						mode = "all"
					}
					fmt.Fprintf(&sb, "  [%s] %s\n", mode, sc.Command)
				}
			}
			fmt.Fprintf(&sb, "\nRun doc with a command name or all=true for details.")
		} else if args.All {
			fmt.Fprintf(&sb, "Template: %s\n", tmpl.Name)
			fmt.Fprintf(&sb, "%s\n\n", tmpl.Description)
			for i, c := range tmpl.Commands {
				if i > 0 {
					sb.WriteString("\n")
				}
				writeCommandDetail(&sb, c)
			}
			if len(tmpl.ShellCommands) > 0 {
				sb.WriteString("\nShell commands (run after any execute):\n")
				for _, sc := range tmpl.ShellCommands {
					mode := sc.Mode
					if mode == "" {
						mode = "all"
					}
					fmt.Fprintf(&sb, "  [%s] %s\n", mode, sc.Command)
				}
			}
		} else {
			cmdMap := make(map[string]domain.TemplateCommand)
			for _, c := range tmpl.Commands {
				cmdMap[c.Command] = c
			}
			targetCmd, ok := cmdMap[args.Command]
			if !ok {
				return errResult(fmt.Sprintf("command %q not found in template %q", args.Command, args.Template)), nil, nil
			}
			writeCommandDetail(&sb, targetCmd)
		}
		return textResult(sb.String()), nil, nil
	})
}

func registerExecuteTemplate(server *sdkmcp.Server, factory ScaffolderFactory, session *Session) {
	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name: "execute",
		Description: `Execute a scaffor template command to scaffold (create) files from templates.

template and command identify which command to run. Call doc(template, command)
first to discover the required variables and their meanings.

params must contain every variable declared in the command. Keys are case-sensitive and
must start with a capital letter (e.g. {"Name": "catalog", "Port": "8080"}). Missing
variables cause the tool to return an error listing exactly what is missing.

Pre-flight check: by default the tool refuses to run if any target file already exists,
preventing accidental overwrites. Use skip=true to silently skip existing files or
force=true to overwrite them.

After files are written, shell_commands defined in the manifest are executed automatically.
Set dry_run=true to print them without executing.

dir overrides the working directory for file output and shell_commands. Defaults to the
MCP client's project root (cwd).

The output reports all file events (created, overwritten, skipped), shell command results,
optional per-command hints, and shell commands.`,
	}, func(ctx context.Context, req *sdkmcp.CallToolRequest, args executeInput) (*sdkmcp.CallToolResult, any, error) {
		s, err := resolveScaffolderWithDir(ctx, req, factory, args.Dir)
		if err != nil {
			return errResult(err.Error()), nil, nil
		}
		params := args.Params
		if params == nil {
			params = map[string]string{}
		}
		opts := domain.ExecuteOptions{
			DryRun: args.DryRun,
			Skip:   args.Skip,
			Force:  args.Force,
		}
		var fileEvents []domain.FileEvent
		// Execute writes its output to os.Stdout. Capture it so that it can be
		// returned as MCP tool content instead of leaking into the stdio transport.
		out, err := captureStdout(func() error {
			var execErr error
			fileEvents, execErr = s.Execute(ctx, args.Template, args.Command, params, opts)
			return execErr
		})

		// Convert domain file events to session file events for logging
		var sessionEvents []FileEvent
		for _, ev := range fileEvents {
			sessionEvents = append(sessionEvents, FileEvent{Path: ev.Path, Action: ev.Action})
		}
		logCall(session, "execute", map[string]any{
			"template": args.Template,
			"command":  args.Command,
			"params":   args.Params,
			"skip":     args.Skip,
			"force":    args.Force,
		}, sessionEvents)

		if err != nil {
			text := strings.TrimSpace(out)
			if text != "" {
				text += "\n" + err.Error()
			} else {
				text = err.Error()
			}
			return errResult(text), nil, nil
		}
		return textResult(out), nil, nil
	})
}

func registerBatchExecute(server *sdkmcp.Server, factory ScaffolderFactory, session *Session) {
	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name: "batch_execute",
		Description: `Execute multiple scaffor template commands in a single call, sequentially.

Use this instead of calling execute() N times when you need to run the same command
repeatedly with different params (e.g. 23 add_migration calls). One batch_execute call
replaces N round-trips.

steps is an ordered array of {command, params} objects. Each step runs the same template
(specified at the top level). Steps execute in order; if any step fails the batch stops
and returns the error along with output accumulated so far.

skip and force apply to every step (same semantics as in execute).

dir overrides the working directory for all steps. Defaults to the MCP client's project
root (cwd).

The output reports each step's file events, hints, and shell command results in sequence.`,
	}, func(ctx context.Context, req *sdkmcp.CallToolRequest, args batchExecuteInput) (*sdkmcp.CallToolResult, any, error) {
		s, err := resolveScaffolderWithDir(ctx, req, factory, args.Dir)
		if err != nil {
			return errResult(err.Error()), nil, nil
		}
		opts := domain.ExecuteOptions{
			Skip:  args.Skip,
			Force: args.Force,
		}

		var sb strings.Builder
		var allSessionEvents []FileEvent

		for i, step := range args.Steps {
			params := step.Params
			if params == nil {
				params = map[string]string{}
			}

			var fileEvents []domain.FileEvent
			out, err := captureStdout(func() error {
				var execErr error
				fileEvents, execErr = s.Execute(ctx, args.Template, step.Command, params, opts)
				return execErr
			})

			for _, ev := range fileEvents {
				allSessionEvents = append(allSessionEvents, FileEvent{Path: ev.Path, Action: ev.Action})
			}

			fmt.Fprintf(&sb, "=== step %d: %s/%s ===\n%s\n", i+1, args.Template, step.Command, out)

			if err != nil {
				logCall(session, "batch_execute", map[string]any{
					"template": args.Template,
					"steps":    args.Steps,
					"skip":     args.Skip,
					"force":    args.Force,
				}, allSessionEvents)
				sb.WriteString(err.Error())
				return errResult(sb.String()), nil, nil
			}
		}

		logCall(session, "batch_execute", map[string]any{
			"template": args.Template,
			"steps":    args.Steps,
			"skip":     args.Skip,
			"force":    args.Force,
		}, allSessionEvents)

		return textResult(sb.String()), nil, nil
	})
}

func registerLintTemplate(server *sdkmcp.Server, factory ScaffolderFactory, session *Session) {
	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name: "lint",
		Description: `Validate a scaffor template manifest and report any issues.

Checks performed:
  - All variables used in destination paths and template file contents are declared in the command
  - post_commands reference commands that exist within the same template
  - No cycles in post_command chains
  - shell_commands have a valid mode ("all" or "per-file") and valid glob patterns
  - Go text/template syntax is valid in destination paths, source files, and hints
  - Variable keys start with a capital letter (required by Go's template engine)
  - Provides "did you mean X?" suggestions for variables that look like typos

Use this when authoring or editing a template manifest to catch mistakes before running
execute. The dir argument lets you lint templates outside the default
.scaffor-templates/ directory. Pass all=true to lint every template at once.`,
	}, func(ctx context.Context, req *sdkmcp.CallToolRequest, args lintInput) (*sdkmcp.CallToolResult, any, error) {
		logCall(session, "lint", map[string]any{"template": args.Template, "dir": args.Dir, "all": args.All}, nil)
		s, err := resolveScaffolder(ctx, req, factory)
		if err != nil {
			return errResult(err.Error()), nil, nil
		}
		if !args.All {
			if args.Template == "" {
				return errResult("template is required when all is false"), nil, nil
			}
			errs := s.Lint(ctx, args.Template, args.Dir)
			if len(errs) == 0 {
				return textResult(fmt.Sprintf("OK: %s has no issues", args.Template)), nil, nil
			}
			var sb strings.Builder
			fmt.Fprintf(&sb, "LINT ERRORS in %s:\n\n", args.Template)
			for _, e := range errs {
				fmt.Fprintf(&sb, "  %s\n", e.Error())
			}
			fmt.Fprintf(&sb, "\n%d issue(s) found", len(errs))
			return errResult(sb.String()), nil, nil
		}

		templates, err := s.ListTemplates(ctx)
		if err != nil {
			return errResult(err.Error()), nil, nil
		}
		if len(templates) == 0 {
			return textResult("No templates found in .scaffor-templates/"), nil, nil
		}

		var sb strings.Builder
		totalErrors := 0
		for _, tmpl := range templates {
			errs := s.Lint(ctx, tmpl.Name, args.Dir)
			if len(errs) == 0 {
				fmt.Fprintf(&sb, "OK: %s has no issues\n", tmpl.Name)
				continue
			}
			totalErrors += len(errs)
			fmt.Fprintf(&sb, "LINT ERRORS in %s:\n", tmpl.Name)
			for _, e := range errs {
				fmt.Fprintf(&sb, "  %s\n", e.Error())
			}
			sb.WriteString("\n")
		}
		if totalErrors > 0 {
			fmt.Fprintf(&sb, "%d issue(s) found across %d template(s)", totalErrors, len(templates))
			return errResult(sb.String()), nil, nil
		}
		return textResult(sb.String()), nil, nil
	})
}

func registerStatus(server *sdkmcp.Server, session *Session) {
	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name: "status",
		Description: `Return the session log of all tool calls made during this MCP session.

Each line is a JSON object with: timestamp, tool name, parameters, and any file events
(created, overwritten, skipped) produced by execute calls.

Use this to review what has been done so far, verify which files were affected, and
confirm the current state of the scaffolding session.`,
	}, func(ctx context.Context, _ *sdkmcp.CallToolRequest, _ struct{}) (*sdkmcp.CallToolResult, any, error) {
		if session == nil {
			return textResult("Session logging is not available."), nil, nil
		}
		text, err := session.Status()
		if err != nil {
			return errResult(err.Error()), nil, nil
		}
		return textResult(text), nil, nil
	})
}

func textResult(text string) *sdkmcp.CallToolResult {
	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: text}},
	}
}

func errResult(text string) *sdkmcp.CallToolResult {
	return &sdkmcp.CallToolResult{
		IsError: true,
		Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: text}},
	}
}

// captureStdout redirects os.Stdout to a pipe for the duration of fn, then
// restores it and returns the captured output. This prevents Execute's
// fmt.Printf calls from writing directly to the stdio transport channel.
func captureStdout(fn func() error) (string, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return "", err
	}
	orig := os.Stdout
	os.Stdout = w

	fnErr := fn()

	_ = w.Close()
	os.Stdout = orig

	out, _ := io.ReadAll(r)
	_ = r.Close()
	return string(out), fnErr
}

func registerTestTemplate(server *sdkmcp.Server, factory ScaffolderFactory, session *Session) {
	type testInput struct {
		Template string `json:"template,omitempty" jsonschema:"name of the template to test; required unless all is true"`
		All      bool   `json:"all,omitempty" jsonschema:"when true, test every template that has a test block"`
	}

	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name: "test",
		Description: `Execute a template's test block in a temporary directory and run validation commands.

Each template can define a test block (list of commands with params to execute in order)
and a validate block (list of shell commands that must exit 0). This tool runs them in a
fresh temp directory to verify the template produces valid output.

IMPORTANT — shell_commands run for real:
The test block executes with Force=true. All actions in the manifest run as normal,
including shell_commands (e.g. goimports, go mod tidy). Commands that require network
access or external tooling (go mod tidy, docker, etc.) will fail in environments without
those dependencies.

To skip shell_commands for a test step, set dry_run: true on that step in the manifest:

  test:
    - command: bootstrap
      dry_run: true   # generates files but skips shell_commands
      params:
        AppName: testapp

The validate block always runs regardless of dry_run (it is meant for final checks like
go build ./...).

Pass all=true to test every template that has a test block defined.`,
	}, func(ctx context.Context, req *sdkmcp.CallToolRequest, args testInput) (*sdkmcp.CallToolResult, any, error) {
		logCall(session, "test", map[string]any{"template": args.Template, "all": args.All}, nil)
		s, err := resolveScaffolder(ctx, req, factory)
		if err != nil {
			return errResult(err.Error()), nil, nil
		}
		if !args.All {
			if args.Template == "" {
				return errResult("template is required when all is false"), nil, nil
			}
			out, err := captureStdout(func() error {
				return s.Test(ctx, args.Template)
			})
			if err != nil {
				text := strings.TrimSpace(out)
				if text != "" {
					text += "\n" + err.Error()
				} else {
					text = err.Error()
				}
				return errResult(text), nil, nil
			}
			return textResult(strings.TrimSpace(out) + "\nPASS: " + args.Template), nil, nil
		}

		templates, err := s.ListTemplates(ctx)
		if err != nil {
			return errResult(err.Error()), nil, nil
		}

		var sb strings.Builder
		failures := 0
		tested := 0
		for _, tmpl := range templates {
			if len(tmpl.Test) == 0 {
				continue
			}
			tested++
			out, err := captureStdout(func() error {
				return s.Test(ctx, tmpl.Name)
			})
			if err != nil {
				failures++
				text := strings.TrimSpace(out)
				if text != "" {
					fmt.Fprintf(&sb, "%s\n", text)
				}
				fmt.Fprintf(&sb, "FAIL: %s — %v\n", tmpl.Name, err)
			} else {
				text := strings.TrimSpace(out)
				if text != "" {
					fmt.Fprintf(&sb, "%s\n", text)
				}
				fmt.Fprintf(&sb, "PASS: %s\n", tmpl.Name)
			}
		}

		if tested == 0 {
			return textResult("No templates with test blocks found."), nil, nil
		}
		if failures > 0 {
			fmt.Fprintf(&sb, "\n%d of %d template(s) failed", failures, tested)
			return errResult(sb.String()), nil, nil
		}
		fmt.Fprintf(&sb, "\nAll %d template(s) passed", tested)
		return textResult(sb.String()), nil, nil
	})
}

// ScaffolderFactory builds a ScaffolderCommands for a given project directory.
// An empty projectDir means fall back to os.Getwd().
type ScaffolderFactory func(projectDir string) (service.ScaffolderCommands, error)

// resolveScaffolder asks the MCP client for its roots, takes the first file://
// URI as the project directory, and builds a ScaffolderCommands via factory.
// Falls back to an empty projectDir (which makes the factory use os.Getwd()).
func resolveScaffolder(ctx context.Context, req *sdkmcp.CallToolRequest, factory ScaffolderFactory) (service.ScaffolderCommands, error) {
	projectDir := ""
	if ss, ok := req.GetSession().(*sdkmcp.ServerSession); ok {
		if result, err := ss.ListRoots(ctx, nil); err == nil && len(result.Roots) > 0 {
			uri := result.Roots[0].URI
			if after, ok := strings.CutPrefix(uri, "file://"); ok {
				projectDir = after
			}
		}
	}
	return factory(projectDir)
}

func resolveScaffolderWithDir(ctx context.Context, req *sdkmcp.CallToolRequest, factory ScaffolderFactory, dir string) (service.ScaffolderCommands, error) {
	if dir != "" {
		return factory(dir)
	}
	return resolveScaffolder(ctx, req, factory)
}
