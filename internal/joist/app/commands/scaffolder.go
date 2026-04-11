package commands

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
	"text/template/parse"
	"unicode"

	"github.com/JLugagne/joist/internal/joist/domain"
	"github.com/JLugagne/joist/internal/joist/domain/repositories/filesystem"
	outboundfs "github.com/JLugagne/joist/internal/joist/outbound/filesystem"
	"github.com/Masterminds/sprig/v3"
	"gopkg.in/yaml.v3"
)

// ScaffolderHandler handles manifest-driven scaffolding.
type ScaffolderHandler struct {
	fs filesystem.FileSystem
}

// NewScaffolderHandler creates a new ScaffolderHandler.
func NewScaffolderHandler(fs filesystem.FileSystem) *ScaffolderHandler {
	return &ScaffolderHandler{fs: fs}
}

// ListTemplates scans .joist-templates/ and returns all template directories.
// Templates whose manifests fail to parse are still included (with name only)
// so that lint --all can report the errors.
func (h *ScaffolderHandler) ListTemplates(ctx context.Context) ([]domain.Template, error) {
	var templates []domain.Template
	files, err := os.ReadDir(".joist-templates")
	if err != nil {
		if os.IsNotExist(err) {
			return templates, nil
		}
		return nil, fmt.Errorf("failed to read .joist-templates: %w", err)
	}

	for _, entry := range files {
		if !entry.IsDir() {
			continue
		}
		tmpl, err := h.GetTemplate(ctx, entry.Name())
		if err != nil {
			// Include the template with just its name so lint --all can report the error.
			templates = append(templates, domain.Template{Name: entry.Name()})
			continue
		}
		templates = append(templates, tmpl)
	}
	return templates, nil
}

func (h *ScaffolderHandler) GetTemplate(ctx context.Context, templateName string) (domain.Template, error) {
	var tmpl domain.Template
	path := filepath.Join(".joist-templates", templateName, "manifest.yaml")
	data, err := h.fs.ReadFile(ctx, path)
	if err != nil {
		return tmpl, fmt.Errorf("failed to read manifest for template %s: %w", templateName, err)
	}
	if err := yaml.Unmarshal(data, &tmpl); err != nil {
		return tmpl, fmt.Errorf("failed to parse manifest for template %s: %w", templateName, err)
	}
	if tmpl.Name == "" {
		tmpl.Name = templateName
	}
	if err := detectPostCommandCycle(tmpl); err != nil {
		return tmpl, err
	}
	return tmpl, nil
}

func getFuncMap() template.FuncMap {
	return sprig.TxtFuncMap()
}

// Execute performs the scaffolding with dedup and hint aggregation.
// After files are written, post_commands are resolved and either printed
// (for the LLM/user to run manually) or executed directly (default, unless opts.DryRun is true).
// It returns a slice of FileEvent recording what happened to each target file.
func (h *ScaffolderHandler) Execute(ctx context.Context, templateName, commandName string, params map[string]string, opts domain.ExecuteOptions) ([]domain.FileEvent, error) {
	tmpl, err := h.GetTemplate(ctx, templateName)
	if err != nil {
		return nil, err
	}

	cmdMap := make(map[string]domain.TemplateCommand)
	for _, c := range tmpl.Commands {
		cmdMap[c.Command] = c
	}

	if _, ok := cmdMap[commandName]; !ok {
		return nil, fmt.Errorf("command '%s' not found in template '%s'", commandName, templateName)
	}

	// Pre-flight check (only when neither skip nor force is set)
	if !opts.Skip && !opts.Force {
		if err := h.preFlightCheck(ctx, tmpl.Name, commandName, cmdMap, params); err != nil {
			return nil, err
		}
	}

	// Execution
	visited := make(map[string]bool)
	var executedCommands []string
	var createdFiles []string
	var fileEvents []domain.FileEvent
	var hints []string

	funcMap := getFuncMap()

	// shellCmds accumulates resolved shell commands in order (mode + rendered string + pattern).
	type resolvedCmd struct {
		mode    string // "all" or "per-file"
		command string
		pattern string
		silent  bool
	}
	var shellCmds []resolvedCmd

	// cmdShellCmds accumulates per-command shell commands (rendered with params).
	var cmdShellCmds []resolvedCmd

	var executeNode func(cmdName string) error
	executeNode = func(cmdName string) error {
		if visited[cmdName] {
			return nil
		}
		visited[cmdName] = true

		cmd := cmdMap[cmdName]

		for _, fileTmpl := range cmd.Files {
			// Resolve target path
			pathTmpl, err := template.New("path").Funcs(funcMap).Parse(fileTmpl.Destination)
			if err != nil {
				return err
			}
			var pathBuf bytes.Buffer
			if err := pathTmpl.Execute(&pathBuf, params); err != nil {
				return err
			}
			targetPath := pathBuf.String()

			if err := safeDestination(targetPath); err != nil {
				return err
			}

			// Check if file already exists (for skip/force handling).
			// Per-file on_conflict overrides global --skip/--force flags.
			_, existErr := h.fs.ReadFile(ctx, targetPath)
			fileExists := existErr == nil

			if fileExists {
				conflict := fileTmpl.OnConflict
				if conflict == "" || conflict == "default" {
					// Fall back to global flags.
					if opts.Skip {
						conflict = "skip"
					} else if opts.Force {
						conflict = "force"
					}
				}
				switch conflict {
				case "skip":
					fileEvents = append(fileEvents, domain.FileEvent{Path: targetPath, Action: "skipped"})
					continue
				case "force":
					// Fall through to overwrite.
				default:
					return fmt.Errorf("file %s already exists (use --skip to skip or --force to overwrite)", targetPath)
				}
			}

			// Create directories
			dir := filepath.Dir(targetPath)
			if err := h.fs.MkdirAll(ctx, dir); err != nil {
				return err
			}

			// Write content
			content := []byte("")
			if fileTmpl.Source != "" {
				tmplPath := filepath.Join(".joist-templates", tmpl.Name, fileTmpl.Source)
				tmplData, err := h.fs.ReadFile(ctx, tmplPath)
				if err != nil {
					return fmt.Errorf("failed to read template %s: %w", tmplPath, err)
				}
				contentTmpl, err := template.New("content").Funcs(funcMap).Parse(string(tmplData))
				if err != nil {
					return err
				}
				var contentBuf bytes.Buffer
				if err := contentTmpl.Execute(&contentBuf, params); err != nil {
					return err
				}
				content = contentBuf.Bytes()
			}

			if err := h.fs.WriteFile(ctx, targetPath, content); err != nil {
				return err
			}
			if fileExists {
				fileEvents = append(fileEvents, domain.FileEvent{Path: targetPath, Action: "overwritten"})
			} else {
				fileEvents = append(fileEvents, domain.FileEvent{Path: targetPath, Action: "created"})
			}
			createdFiles = append(createdFiles, targetPath)
		}

		if cmd.Hint != "" {
			hintTmpl, err := template.New("hint").Funcs(funcMap).Parse(cmd.Hint)
			if err == nil {
				var hintBuf bytes.Buffer
				if err := hintTmpl.Execute(&hintBuf, params); err == nil {
					hints = append(hints, fmt.Sprintf("--- %s ---\n%s\n", cmdName, hintBuf.String()))
				}
			}
		}

		// Resolve per-command shell_commands with the same params as files/hints.
		for _, sc := range cmd.ShellCommands {
			t, err := template.New("cmdshell").Funcs(funcMap).Parse(sc.Command)
			if err != nil {
				return fmt.Errorf("failed to parse command shell_command %q: %w", sc.Command, err)
			}
			var buf bytes.Buffer
			if err := t.Execute(&buf, params); err != nil {
				return fmt.Errorf("failed to render command shell_command %q: %w", sc.Command, err)
			}
			cmdShellCmds = append(cmdShellCmds, resolvedCmd{command: buf.String(), silent: sc.Silent})
		}

		executedCommands = append(executedCommands, cmdName)

		// Chain post_commands (other commands in this template, deduplicated)
		for _, postCmd := range cmd.PostCommands {
			if err := executeNode(postCmd); err != nil {
				return err
			}
		}

		return nil
	}

	if err := executeNode(commandName); err != nil {
		return fileEvents, err
	}

	// Output summary
	fmt.Printf("File events:\n")
	for _, ev := range fileEvents {
		fmt.Printf("  [%s] %s\n", ev.Action, ev.Path)
	}
	fmt.Printf("\n")
	for _, hint := range hints {
		fmt.Println(hint)
	}
	// Collect shell_commands from the template root.
	// These are NOT rendered here — they are rendered in the second pass below
	// with a combined context that includes both user params and File/Files.
	for _, sc := range tmpl.ShellCommands {
		mode := sc.Mode
		if mode == "" {
			mode = "all"
		}
		shellCmds = append(shellCmds, resolvedCmd{mode: mode, command: sc.Command, pattern: sc.Pattern, silent: sc.Silent})
	}

	if len(cmdShellCmds) == 0 && len(shellCmds) == 0 {
		return fileEvents, nil
	}

	type renderedCmd struct {
		rendered string
		silent   bool
	}
	var toRun []renderedCmd

	// Per-command shell_commands (already rendered with params).
	for _, sc := range cmdShellCmds {
		toRun = append(toRun, renderedCmd{rendered: sc.command, silent: sc.silent})
	}

	// Resolve per-file and all-files variants of template-level shell_commands.
	// {{ .Files }} → space-joined list of files matching the pattern
	// {{ .File }} → individual file (per-file mode only)

	for _, sc := range shellCmds {
		// Filter files based on pattern (if specified)
		var matchingFiles []string
		for _, f := range createdFiles {
			if matchesPattern(f, sc.pattern) {
				matchingFiles = append(matchingFiles, f)
			}
		}

		// Skip shell command if no files match the pattern
		if len(matchingFiles) == 0 {
			continue
		}

		matchingFilesStr := strings.Join(matchingFiles, " ")

		// Build a combined data context: user params + File/Files.
		baseData := make(map[string]string, len(params)+2)
		for k, v := range params {
			baseData[k] = v
		}
		baseData["Files"] = matchingFilesStr

		switch sc.mode {
		case "per-file":
			for _, f := range matchingFiles {
				baseData["File"] = f
				t, err := template.New("scfile").Funcs(funcMap).Parse(sc.command)
				if err != nil {
					return fileEvents, fmt.Errorf("failed to parse per-file shell_command %q: %w", sc.command, err)
				}
				var buf bytes.Buffer
				if err := t.Execute(&buf, baseData); err != nil {
					return fileEvents, fmt.Errorf("failed to render per-file shell_command %q: %w", sc.command, err)
				}
				toRun = append(toRun, renderedCmd{rendered: buf.String(), silent: sc.silent})
			}
		default: // "all"
			t, err := template.New("scall").Funcs(funcMap).Parse(sc.command)
			if err != nil {
				return fileEvents, fmt.Errorf("failed to parse shell_command %q: %w", sc.command, err)
			}
			var buf bytes.Buffer
			if err := t.Execute(&buf, baseData); err != nil {
				return fileEvents, fmt.Errorf("failed to render shell_command %q: %w", sc.command, err)
			}
			toRun = append(toRun, renderedCmd{rendered: buf.String(), silent: sc.silent})
		}
	}

	if opts.DryRun {
		fmt.Printf("\nShell commands (dry-run):\n")
		for _, rc := range toRun {
			fmt.Printf("  %s\n", rc.rendered)
		}
	} else {
		fmt.Println()
		for _, rc := range toRun {
			fmt.Printf("Execute command: %s\n", rc.rendered)
			cmd := exec.CommandContext(ctx, "sh", "-c", rc.rendered)
			var out bytes.Buffer
			cmd.Stdout = &out
			cmd.Stderr = &out
			err := cmd.Run()
			if rc.silent {
				if err != nil {
					fmt.Printf("Result: Error\n")
					return fileEvents, fmt.Errorf("shell_command %q failed: %w", rc.rendered, err)
				}
				fmt.Printf("Result: Success\n")
			} else {
				result := strings.TrimSpace(out.String())
				if result != "" {
					fmt.Printf("Result: %s\n", result)
				}
				if err != nil {
					return fileEvents, fmt.Errorf("shell_command %q failed: %w", rc.rendered, err)
				}
			}
			fmt.Println()
		}
	}

	skippedCount := len(visited) - len(executedCommands)
	fmt.Printf("SUCCESS: Executed %s/%s (%d commands, %d skipped)\n", templateName, commandName, len(executedCommands), skippedCount)

	return fileEvents, nil
}

func (h *ScaffolderHandler) preFlightCheck(ctx context.Context, templateName, commandName string, cmdMap map[string]domain.TemplateCommand, params map[string]string) error {
	visited := make(map[string]bool)
	funcMap := getFuncMap()

	walk := func(cmdName string) error {
		if visited[cmdName] {
			return nil
		}
		visited[cmdName] = true
		cmd := cmdMap[cmdName]

		for _, fileTmpl := range cmd.Files {
			pathTmpl, err := template.New("path").Funcs(funcMap).Parse(fileTmpl.Destination)
			if err != nil {
				return err
			}
			var pathBuf bytes.Buffer
			if err := pathTmpl.Execute(&pathBuf, params); err != nil {
				return err
			}
			targetPath := pathBuf.String()

			if err := safeDestination(targetPath); err != nil {
				return err
			}

			// Files with on_conflict skip or force don't need a pre-flight check.
			if fileTmpl.OnConflict == "skip" || fileTmpl.OnConflict == "force" {
				continue
			}

			_, err = h.fs.ReadFile(ctx, targetPath)
			if err == nil {
				return fmt.Errorf("pre-flight check failed: file %s already exists", targetPath)
			} else if !os.IsNotExist(err) {
				return fmt.Errorf("pre-flight check failed on %s: %w", targetPath, err)
			}
		}

		return nil
	}

	return walk(commandName)
}

func (h *ScaffolderHandler) Lint(ctx context.Context, templateName string, templateDir string) []domain.LintError {
	// Parse the manifest directly so we can report all issues even when the DAG is invalid.
	var tmpl domain.Template
	if templateDir == "" {
		templateDir = ".joist-templates"
	}
	path := filepath.Join(templateDir, templateName, "manifest.yaml")
	data, err := h.fs.ReadFile(ctx, path)
	if err != nil {
		return []domain.LintError{{Field: "manifest", Message: fmt.Sprintf("failed to read manifest for template %s: %v", templateName, err)}}
	}
	if err := yaml.Unmarshal(data, &tmpl); err != nil {
		return []domain.LintError{{Field: "manifest", Message: fmt.Sprintf("failed to parse manifest for template %s: %v", templateName, err)}}
	}
	if tmpl.Name == "" {
		tmpl.Name = templateName
	}

	cmdMap := make(map[string]domain.TemplateCommand)
	for _, c := range tmpl.Commands {
		cmdMap[c.Command] = c
	}

	var errs []domain.LintError

	for _, cmd := range tmpl.Commands {
		// Build declared variable set for this command
		declared := make(map[string]bool)
		for _, v := range cmd.Variables {
			declared[v.Key] = true
			// Variable keys must start with a capital letter (text/template requirement)
			if len(v.Key) > 0 && !unicode.IsUpper(rune(v.Key[0])) {
				errs = append(errs, domain.LintError{
					Command: cmd.Command,
					Field:   "variables",
					Message: fmt.Sprintf("variable key %q must start with a capital letter", v.Key),
				})
			}
		}

		// undeclaredErr builds an error for an undeclared variable, appending
		// a "did you mean X?" suggestion when a close match exists.
		undeclaredErr := func(varName, field string) domain.LintError {
			msg := fmt.Sprintf("variable %q used but not declared", varName)
			if len(declared) > 0 {
				if closest, dist := closestVar(varName, declared); dist > 0 && dist <= 3 {
					msg += fmt.Sprintf(" (did you mean %q?)", closest)
				}
			}
			return domain.LintError{Command: cmd.Command, Field: field, Message: msg}
		}

		// Validate post_commands (must reference existing commands in this template)
		for _, pc := range cmd.PostCommands {
			if _, ok := cmdMap[pc]; !ok {
				errs = append(errs, domain.LintError{
					Command: cmd.Command,
					Field:   "post_commands",
					Message: fmt.Sprintf("references undefined command %q", pc),
				})
			}
		}

		// Validate hint template syntax if present
		if cmd.Hint != "" {
			if _, err := template.New("").Funcs(getFuncMap()).Parse(cmd.Hint); err != nil {
				errs = append(errs, domain.LintError{
					Command: cmd.Command,
					Field:   "hint",
					Message: fmt.Sprintf("hint has invalid template syntax: %v", err),
				})
			} else {
				// Check variables used in hint
				used := extractTemplateVars(cmd.Hint)
				for v := range used {
					if !declared[v] {
						errs = append(errs, undeclaredErr(v, "hint"))
					}
				}
			}
		}

		// Check variables used in destination paths, validate parsing, and on_conflict values
		for _, f := range cmd.Files {
			// Validate on_conflict value
			if f.OnConflict != "" && f.OnConflict != "default" && f.OnConflict != "skip" && f.OnConflict != "force" {
				errs = append(errs, domain.LintError{
					Command: cmd.Command,
					Field:   "files.on_conflict",
					Message: fmt.Sprintf("on_conflict %q is invalid for destination %q (must be \"default\", \"skip\", or \"force\")", f.OnConflict, f.Destination),
				})
			}

			// Validate that the destination path parses as a template
			if _, err := template.New("").Funcs(getFuncMap()).Parse(f.Destination); err != nil {
				errs = append(errs, domain.LintError{
					Command: cmd.Command,
					Field:   "files.destination",
					Message: fmt.Sprintf("destination path %q has invalid template syntax: %v", f.Destination, err),
				})
				continue
			}

			used := extractTemplateVars(f.Destination)
			for v := range used {
				if !declared[v] {
					errs = append(errs, undeclaredErr(v, "files.destination"))
				}
			}
		}

		// Check variables used in template file source contents and validate parsing
		for _, f := range cmd.Files {
			if f.Source == "" {
				continue
			}
			tmplPath := filepath.Join(templateDir, tmpl.Name, f.Source)
			data, err := h.fs.ReadFile(ctx, tmplPath)
			if err != nil {
				errs = append(errs, domain.LintError{
					Command: cmd.Command,
					Field:   "files.source",
					Message: fmt.Sprintf("cannot read template file %q: %v", f.Source, err),
				})
				continue
			}

			// Validate that the template file parses correctly
			if _, err := template.New("").Funcs(getFuncMap()).Parse(string(data)); err != nil {
				errs = append(errs, domain.LintError{
					Command: cmd.Command,
					Field:   "files.source",
					Message: fmt.Sprintf("template file %q has invalid syntax: %v", f.Source, err),
				})
				continue
			}

			used := extractTemplateVars(string(data))
			for v := range used {
				if !declared[v] {
					errs = append(errs, undeclaredErr(v, "files.source:"+f.Source))
				}
			}
		}

		// Validate per-command shell_commands
		for i, sc := range cmd.ShellCommands {
			if sc.Command == "" {
				errs = append(errs, domain.LintError{
					Command: cmd.Command,
					Field:   "shell_commands",
					Message: fmt.Sprintf("shell_command[%d] has an empty command", i),
				})
				continue
			}
			if _, err := template.New("").Funcs(getFuncMap()).Parse(sc.Command); err != nil {
				errs = append(errs, domain.LintError{
					Command: cmd.Command,
					Field:   "shell_commands",
					Message: fmt.Sprintf("shell_command[%d] has invalid template syntax: %v", i, err),
				})
				continue
			}
			scUsed := extractTemplateVars(sc.Command)
			for v := range scUsed {
				if !declared[v] {
					errs = append(errs, undeclaredErr(v, "shell_commands"))
				}
			}
			if sc.Mode != "" && sc.Mode != "all" && sc.Mode != "per-file" {
				errs = append(errs, domain.LintError{
					Command: cmd.Command,
					Field:   "shell_commands",
					Message: fmt.Sprintf("shell_command[%d] has invalid mode %q (must be \"all\" or \"per-file\")", i, sc.Mode),
				})
			}
			if sc.Pattern != "" {
				for _, pattern := range strings.Split(sc.Pattern, ",") {
					pattern = strings.TrimSpace(pattern)
					if pattern == "" {
						continue
					}
					if _, err := filepath.Match(pattern, ""); err != nil {
						errs = append(errs, domain.LintError{
							Command: cmd.Command,
							Field:   "shell_commands",
							Message: fmt.Sprintf("shell_command[%d] has invalid pattern %q: %v", i, pattern, err),
						})
					}
				}
			}
		}
	}

	// Validate template-level shell_commands
	for i, sc := range tmpl.ShellCommands {
		if sc.Command == "" {
			errs = append(errs, domain.LintError{
				Field:   "shell_commands",
				Message: fmt.Sprintf("shell_command[%d] has an empty command", i),
			})
			continue
		}

		// Validate shell command template syntax
		if _, err := template.New("").Funcs(getFuncMap()).Parse(sc.Command); err != nil {
			errs = append(errs, domain.LintError{
				Field:   "shell_commands",
				Message: fmt.Sprintf("shell_command[%d] has invalid template syntax: %v", i, err),
			})
		}

		if sc.Mode != "" && sc.Mode != "all" && sc.Mode != "per-file" {
			errs = append(errs, domain.LintError{
				Field:   "shell_commands",
				Message: fmt.Sprintf("shell_command[%d] has invalid mode %q (must be \"all\" or \"per-file\")", i, sc.Mode),
			})
		}

		// Validate pattern syntax if specified
		if sc.Pattern != "" {
			for _, pattern := range strings.Split(sc.Pattern, ",") {
				pattern = strings.TrimSpace(pattern)
				if pattern == "" {
					continue
				}
				// Validate the pattern by trying to match against a dummy filename
				if _, err := filepath.Match(pattern, ""); err != nil {
					errs = append(errs, domain.LintError{
						Field:   "shell_commands",
						Message: fmt.Sprintf("shell_command[%d] has invalid pattern %q: %v", i, pattern, err),
					})
				}
			}
		}
	}

	return errs
}

// safeDestination rejects paths that contain ".." components to prevent
// directory traversal outside the project directory.
func safeDestination(path string) error {
	clean := filepath.Clean(path)
	for _, part := range strings.Split(clean, string(filepath.Separator)) {
		if part == ".." {
			return fmt.Errorf("destination path %q is not allowed: contains \"..\"", path)
		}
	}
	return nil
}

// extractTemplateVars parses a Go text/template string and returns all
// top-level field names accessed via {{ .FieldName }} syntax, including
// piped forms like {{ .FieldName | lower }}.
func extractTemplateVars(tmplStr string) map[string]bool {
	vars := make(map[string]bool)
	t, err := template.New("").Funcs(getFuncMap()).Parse(tmplStr)
	if err != nil {
		return vars
	}
	inspectNode(t.Root, vars)
	return vars
}

func inspectNode(node parse.Node, vars map[string]bool) {
	if node == nil {
		return
	}
	switch n := node.(type) {
	case *parse.ListNode:
		if n == nil {
			return
		}
		for _, child := range n.Nodes {
			inspectNode(child, vars)
		}
	case *parse.ActionNode:
		if n == nil {
			return
		}
		inspectNode(n.Pipe, vars)
	case *parse.PipeNode:
		if n == nil {
			return
		}
		for _, cmd := range n.Cmds {
			for _, arg := range cmd.Args {
				inspectNode(arg, vars)
			}
		}
	case *parse.FieldNode:
		if len(n.Ident) == 1 {
			vars[n.Ident[0]] = true
		}
	case *parse.IfNode:
		if n.List != nil {
			inspectNode(n.List, vars)
		}
		if n.ElseList != nil {
			inspectNode(n.ElseList, vars)
		}
		if n.Pipe != nil {
			inspectNode(n.Pipe, vars)
		}
	case *parse.RangeNode:
		if n.List != nil {
			inspectNode(n.List, vars)
		}
		if n.ElseList != nil {
			inspectNode(n.ElseList, vars)
		}
		if n.Pipe != nil {
			inspectNode(n.Pipe, vars)
		}
	case *parse.WithNode:
		if n.List != nil {
			inspectNode(n.List, vars)
		}
		if n.ElseList != nil {
			inspectNode(n.ElseList, vars)
		}
		if n.Pipe != nil {
			inspectNode(n.Pipe, vars)
		}
	}
}

// levenshtein returns the edit distance between two strings.
func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	la, lb := len(ra), len(rb)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	row := make([]int, lb+1)
	for j := range row {
		row[j] = j
	}
	for i := 1; i <= la; i++ {
		prev := i
		for j := 1; j <= lb; j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur := min(row[j]+1, min(prev+1, row[j-1]+cost))
			row[j-1] = prev
			prev = cur
		}
		row[lb] = prev
	}
	return row[lb]
}

// closestVar returns the closest declared variable name to the given name
// using Levenshtein distance, along with the distance. Returns ("", -1) if
// the declared set is empty.
func closestVar(name string, declared map[string]bool) (string, int) {
	best, bestDist := "", -1
	for k := range declared {
		d := levenshtein(name, k)
		if bestDist < 0 || d < bestDist {
			best, bestDist = k, d
		}
	}
	return best, bestDist
}

// matchesPattern checks if a file path matches any of the comma-separated glob patterns.
// If patterns is empty, all files match (default behavior).
func matchesPattern(filePath, patterns string) bool {
	if patterns == "" {
		return true
	}

	for _, pattern := range strings.Split(patterns, ",") {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if matched, err := filepath.Match(pattern, filepath.Base(filePath)); err == nil && matched {
			return true
		}
	}
	return false
}

func detectPostCommandCycle(tmpl domain.Template) error {
	adj := make(map[string][]string, len(tmpl.Commands))
	for _, c := range tmpl.Commands {
		adj[c.Command] = c.PostCommands
	}
	visited := make(map[string]bool)
	inStack := make(map[string]bool)
	var dfs func(node string) bool
	dfs = func(node string) bool {
		visited[node] = true
		inStack[node] = true
		for _, next := range adj[node] {
			if !visited[next] {
				if dfs(next) {
					return true
				}
			} else if inStack[next] {
				return true
			}
		}
		inStack[node] = false
		return false
	}
	for _, c := range tmpl.Commands {
		if !visited[c.Command] {
			if dfs(c.Command) {
				return fmt.Errorf("cycle detected in post_commands for template %q", tmpl.Name)
			}
		}
	}
	return nil
}

func (h *ScaffolderHandler) Test(ctx context.Context, templateName string) error {
	tmpl, err := h.GetTemplate(ctx, templateName)
	if err != nil {
		return err
	}

	if len(tmpl.Test) == 0 {
		return fmt.Errorf("template %q has no test block defined", templateName)
	}

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "joist-test-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Copy .joist-templates/<template>/ into the temp dir
	srcDir := filepath.Join(".joist-templates", templateName)
	dstDir := filepath.Join(tmpDir, ".joist-templates", templateName)
	if err := copyDir(srcDir, dstDir); err != nil {
		return fmt.Errorf("failed to copy template to temp dir: %w", err)
	}

	// Save original working directory and change to temp dir
	origDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		return fmt.Errorf("failed to change to temp directory: %w", err)
	}
	defer os.Chdir(origDir)

	// Create a new handler with a real filesystem for the temp dir
	tmpFS := outboundfs.NewFileSystem()
	tmpHandler := NewScaffolderHandler(tmpFS)

	// Build set of all command names in the manifest
	allCommands := make(map[string]bool)
	for _, c := range tmpl.Commands {
		allCommands[c.Command] = true
	}

	// Execute each test step
	exercised := make(map[string]bool)
	for _, step := range tmpl.Test {
		fmt.Printf("[test] Executing %s/%s...\n", templateName, step.Command)
		params := step.Params
		if params == nil {
			params = map[string]string{}
		}
		_, err := tmpHandler.Execute(ctx, templateName, step.Command, params, domain.ExecuteOptions{Force: true})
		if err != nil {
			return fmt.Errorf("test step %q failed: %w", step.Command, err)
		}
		exercised[step.Command] = true
	}

	// Run validation commands
	for _, cmd := range tmpl.Validate {
		fmt.Printf("[validate] Running: %s\n", cmd)
		c := exec.CommandContext(ctx, "sh", "-c", cmd)
		c.Dir = tmpDir
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			return fmt.Errorf("validation command %q failed: %w", cmd, err)
		}
	}

	// Print coverage
	n := len(exercised)
	m := len(allCommands)
	pct := 0
	if m > 0 {
		pct = n * 100 / m
	}
	fmt.Printf("%d of %d commands exercised (%d%%)\n", n, m, pct)

	return nil
}

func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			data, err := os.ReadFile(srcPath)
			if err != nil {
				return err
			}
			if err := os.WriteFile(dstPath, data, 0644); err != nil {
				return err
			}
		}
	}
	return nil
}
