package mcp

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/JLugagne/joist/internal/joist/domain"
	"github.com/JLugagne/joist/internal/joist/domain/service"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

type docInput struct {
	Template string `json:"template" jsonschema:"name of the template directory under .joist-templates/ (e.g. \"service\")"`
	Command  string `json:"command,omitempty" jsonschema:"command name within the template; omit to list all commands, provide to get the variables and post_commands for that specific command"`
}

type executeInput struct {
	Template    string            `json:"template" jsonschema:"name of the template directory under .joist-templates/ (e.g. \"service\")"`
	Command     string            `json:"command" jsonschema:"command name to execute within the template (e.g. \"create\"); call doc first to discover available commands"`
	Params      map[string]string `json:"params,omitempty" jsonschema:"variables as key→value pairs; keys are case-sensitive and must start with a capital letter (e.g. {\"Name\": \"catalog\", \"Port\": \"8080\"}); call doc(template, command) to discover required variables"`
	RunCommands bool              `json:"run_commands,omitempty" jsonschema:"when true, shell_commands in the manifest are executed automatically via sh -c; when false (default) they are printed for manual execution"`
}

type lintInput struct {
	Template string `json:"template,omitempty" jsonschema:"name of the template directory under .joist-templates/ to validate; required unless all is true"`
	Dir      string `json:"dir,omitempty" jsonschema:"directory containing templates; overrides the default .joist-templates/ search path"`
	All      bool   `json:"all,omitempty" jsonschema:"when true, lint every template in the directory; template is ignored"`
}

// NewServer creates an MCP server exposing joist's scaffolding tools.
func NewServer(scaffolder service.ScaffolderCommands) *sdkmcp.Server {
	server := sdkmcp.NewServer(&sdkmcp.Implementation{
		Name:    "joist",
		Title:   "joist — template-driven scaffolding",
		Version: "1.0.0",
	}, &sdkmcp.ServerOptions{
		Instructions: `joist manages code generation for this project through templates in .joist-templates/.

IMPORTANT: before creating any new files, components, services, handlers, or project
structure, always call list first to check whether a template already exists for it.
Use the template if one matches — only write files manually when none does.

Workflow:
  list                          → discover available templates
  doc(template)                 → list its commands and descriptions
  doc(template, command)        → get the exact variables required
  execute(template, command, …) → scaffold the files
  lint(template)                → validate a manifest after editing it`,
	})

	registerListTemplates(server, scaffolder)
	registerDocTemplate(server, scaffolder)
	registerExecuteTemplate(server, scaffolder)
	registerLintTemplate(server, scaffolder)

	return server
}

// Serve runs the MCP server over stdio.
func Serve(ctx context.Context, scaffolder service.ScaffolderCommands) error {
	return NewServer(scaffolder).Run(ctx, &sdkmcp.StdioTransport{})
}

func registerListTemplates(server *sdkmcp.Server, scaffolder service.ScaffolderCommands) {
	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name: "list",
		Description: `ALWAYS call this tool before creating any new files, components, services, handlers,
repositories, or any repeating project structure. joist may already have a template that
does it correctly and consistently — skip this check only if you are certain no template
could apply.

Lists all scaffolding templates available in the project's .joist-templates/ directory.
Each template is a named subdirectory containing a manifest.yaml that defines commands,
the variables they require, and the files they create.

When a relevant template exists, use it instead of writing files by hand:
  1. list                          — discover available templates  (this tool)
  2. doc(template)                 — see its commands and descriptions
  3. doc(template, command)        — get required variables for a command
  4. execute(template, command, …) — scaffold the files

Only create files manually if no template covers the use case.`,
	}, func(ctx context.Context, _ *sdkmcp.CallToolRequest, _ struct{}) (*sdkmcp.CallToolResult, any, error) {
		templates, err := scaffolder.ListTemplates(ctx)
		if err != nil {
			return errResult(err.Error()), nil, nil
		}
		if len(templates) == 0 {
			return textResult("No templates found in .joist-templates/"), nil, nil
		}
		var sb strings.Builder
		sb.WriteString("Available Templates:\n")
		for _, tmpl := range templates {
			fmt.Fprintf(&sb, "\n- %s:\n", tmpl.Name)
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

func registerDocTemplate(server *sdkmcp.Server, scaffolder service.ScaffolderCommands) {
	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name: "doc",
		Description: `Show documentation for a joist template or one of its commands.

Two modes depending on which arguments you provide:

  template only — lists every command the template exposes (name + description) and any
    shell_commands that run automatically after execution. Use this to understand what a
    template can do before deciding which command to run.

  template + command — shows the full detail for that command: its description, every
    variable you must supply via params when calling execute (name + description),
    and the post_commands that will chain after it (other commands in the same template
    that execute automatically).

Call this before execute whenever you need to know which params are required.`,
	}, func(ctx context.Context, _ *sdkmcp.CallToolRequest, args docInput) (*sdkmcp.CallToolResult, any, error) {
		tmpl, err := scaffolder.GetTemplate(ctx, args.Template)
		if err != nil {
			return errResult(err.Error()), nil, nil
		}

		var sb strings.Builder
		if args.Command == "" {
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
			fmt.Fprintf(&sb, "\nRun doc with a command name for details.")
		} else {
			cmdMap := make(map[string]domain.TemplateCommand)
			for _, c := range tmpl.Commands {
				cmdMap[c.Command] = c
			}
			targetCmd, ok := cmdMap[args.Command]
			if !ok {
				return errResult(fmt.Sprintf("command %q not found in template %q", args.Command, args.Template)), nil, nil
			}
			fmt.Fprintf(&sb, "Command: %s\n", targetCmd.Command)
			fmt.Fprintf(&sb, "  %s\n\n", targetCmd.Description)
			if len(targetCmd.Variables) > 0 {
				sb.WriteString("  Variables:\n")
				for _, v := range targetCmd.Variables {
					fmt.Fprintf(&sb, "    %s\t%s\n", v.Key, v.Description)
				}
			} else {
				sb.WriteString("  Variables: None\n")
			}
			if len(targetCmd.PostCommands) > 0 {
				sb.WriteString("\n  Post-commands:\n")
				for _, pc := range targetCmd.PostCommands {
					fmt.Fprintf(&sb, "    → %s\n", pc)
				}
			}
		}
		return textResult(sb.String()), nil, nil
	})
}

func registerExecuteTemplate(server *sdkmcp.Server, scaffolder service.ScaffolderCommands) {
	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name: "execute",
		Description: `Execute a joist template command to scaffold (create) files from templates.

template and command identify which command to run. Call doc(template, command)
first to discover the required variables and their meanings.

params must contain every variable declared in the command. Keys are case-sensitive and
must start with a capital letter (e.g. {"Name": "catalog", "Port": "8080"}). Missing
variables cause the tool to return an error listing exactly what is missing.

Pre-flight check: the tool refuses to run if any target file already exists, preventing
accidental overwrites.

After files are written, any shell_commands defined in the manifest are handled based on
run_commands:
  false (default) — commands are printed in the output for you or the user to run manually
  true            — commands are executed automatically via sh -c in the working directory

The output reports all created files, optional per-command hints, and shell commands.`,
	}, func(ctx context.Context, _ *sdkmcp.CallToolRequest, args executeInput) (*sdkmcp.CallToolResult, any, error) {
		params := args.Params
		if params == nil {
			params = map[string]string{}
		}
		// Execute writes its output to os.Stdout. Capture it so that it can be
		// returned as MCP tool content instead of leaking into the stdio transport.
		out, err := captureStdout(func() error {
			return scaffolder.Execute(ctx, args.Template, args.Command, params, args.RunCommands)
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
		return textResult(out), nil, nil
	})
}

func registerLintTemplate(server *sdkmcp.Server, scaffolder service.ScaffolderCommands) {
	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name: "lint",
		Description: `Validate a joist template manifest and report any issues.

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
.joist-templates/ directory. Pass all=true to lint every template at once.`,
	}, func(ctx context.Context, _ *sdkmcp.CallToolRequest, args lintInput) (*sdkmcp.CallToolResult, any, error) {
		if !args.All {
			if args.Template == "" {
				return errResult("template is required when all is false"), nil, nil
			}
			errs := scaffolder.Lint(ctx, args.Template, args.Dir)
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

		templates, err := scaffolder.ListTemplates(ctx)
		if err != nil {
			return errResult(err.Error()), nil, nil
		}
		if len(templates) == 0 {
			return textResult("No templates found in .joist-templates/"), nil, nil
		}

		var sb strings.Builder
		totalErrors := 0
		for _, tmpl := range templates {
			errs := scaffolder.Lint(ctx, tmpl.Name, args.Dir)
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
