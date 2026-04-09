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
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
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

// ListTemplates scans .joist-templates/ and returns all valid templates.
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
			continue
		}
		templates = append(templates, tmpl)
	}
	return templates, nil
}

// GetTemplate reads and validates a template manifest.
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
	return tmpl, nil
}

func getFuncMap() template.FuncMap {
	return template.FuncMap{
		"lower": strings.ToLower,
		"upper": strings.ToUpper,
		"title": cases.Title(language.English).String,
	}
}

// Execute performs the scaffolding with dedup and hint aggregation.
// After files are written, post_commands are resolved and either printed
// (for the LLM/user to run manually) or executed directly when runCommands is true.
func (h *ScaffolderHandler) Execute(ctx context.Context, templateName, commandName string, params map[string]string, runCommands bool) error {
	tmpl, err := h.GetTemplate(ctx, templateName)
	if err != nil {
		return err
	}

	cmdMap := make(map[string]domain.TemplateCommand)
	for _, c := range tmpl.Commands {
		cmdMap[c.Command] = c
	}

	if _, ok := cmdMap[commandName]; !ok {
		return fmt.Errorf("command '%s' not found in template '%s'", commandName, templateName)
	}

	// Pre-flight check
	if err := h.preFlightCheck(ctx, tmpl.Name, commandName, cmdMap, params); err != nil {
		return err
	}

	// Execution
	visited := make(map[string]bool)
	var executedCommands []string
	var createdFiles []string
	var hints []string

	funcMap := getFuncMap()

	// postCmds accumulates resolved shell commands in order (mode + rendered string).
	type resolvedCmd struct {
		mode    string // "all" or "per-file"
		command string
	}
	var postCmds []resolvedCmd

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

		executedCommands = append(executedCommands, cmdName)

		// Collect post_commands (resolved against params, files resolved later)
		for _, pc := range cmd.PostCommands {
			cmdTmpl, err := template.New("postcmd").Funcs(funcMap).Parse(pc.Command)
			if err != nil {
				return fmt.Errorf("failed to parse post_command template %q: %w", pc.Command, err)
			}
			var cmdBuf bytes.Buffer
			if err := cmdTmpl.Execute(&cmdBuf, params); err != nil {
				return fmt.Errorf("failed to render post_command template %q: %w", pc.Command, err)
			}
			mode := pc.Mode
			if mode == "" {
				mode = "all"
			}
			postCmds = append(postCmds, resolvedCmd{mode: mode, command: cmdBuf.String()})
		}

		return nil
	}

	if err := executeNode(commandName); err != nil {
		return err
	}

	// Output summary
	skippedCount := len(visited) - len(executedCommands)
	fmt.Printf("Created files:\n")
	for _, f := range createdFiles {
		fmt.Printf("  %s\n", f)
	}
	fmt.Printf("\n")
	for _, hint := range hints {
		fmt.Println(hint)
	}
	fmt.Printf("SUCCESS: Executed %s/%s (%d commands, %d skipped)\n", templateName, commandName, len(executedCommands), skippedCount)

	if len(postCmds) == 0 {
		return nil
	}

	// Resolve per-file and all-files variants of each post_command.
	// {{ .Files }} → space-joined list of all created files
	// {{ .File }} → individual file (per-file mode only)
	allFilesStr := strings.Join(createdFiles, " ")

	type shellCmd struct {
		rendered string
	}
	var toRun []shellCmd

	for _, pc := range postCmds {
		switch pc.mode {
		case "per-file":
			for _, f := range createdFiles {
				data := map[string]string{"File": f, "Files": allFilesStr}
				t, err := template.New("pcfile").Funcs(funcMap).Parse(pc.command)
				if err != nil {
					return fmt.Errorf("failed to parse per-file post_command %q: %w", pc.command, err)
				}
				var buf bytes.Buffer
				if err := t.Execute(&buf, data); err != nil {
					return fmt.Errorf("failed to render per-file post_command %q: %w", pc.command, err)
				}
				toRun = append(toRun, shellCmd{rendered: buf.String()})
			}
		default: // "all"
			data := map[string]string{"Files": allFilesStr}
			t, err := template.New("pcall").Funcs(funcMap).Parse(pc.command)
			if err != nil {
				return fmt.Errorf("failed to parse post_command %q: %w", pc.command, err)
			}
			var buf bytes.Buffer
			if err := t.Execute(&buf, data); err != nil {
				return fmt.Errorf("failed to render post_command %q: %w", pc.command, err)
			}
			toRun = append(toRun, shellCmd{rendered: buf.String()})
		}
	}

	if runCommands {
		fmt.Printf("\nRunning post-commands:\n")
		for _, sc := range toRun {
			fmt.Printf("  $ %s\n", sc.rendered)
			cmd := exec.CommandContext(ctx, "sh", "-c", sc.rendered)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("post_command %q failed: %w", sc.rendered, err)
			}
		}
	} else {
		fmt.Printf("\nPost-commands to run:\n")
		for _, sc := range toRun {
			fmt.Printf("  %s\n", sc.rendered)
		}
		fmt.Printf("\nRun with --run-commands to execute them automatically.\n")
	}

	return nil
}

func (h *ScaffolderHandler) preFlightCheck(ctx context.Context, templateName, commandName string, cmdMap map[string]domain.TemplateCommand, params map[string]string) error {
	visited := make(map[string]bool)
	funcMap := getFuncMap()

	var walk func(cmdName string) error
	walk = func(cmdName string) error {
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

// Lint validates a template manifest and returns a list of issues found.
// It checks:
//   - all declared variable keys start with a capital letter (required by text/template)
//   - all variables referenced in destination paths and template file sources are declared
//   - all post_commands have a non-empty command and a valid mode ("all" or "per-file")
//
// When an undeclared variable is found, it suggests the closest declared variable
// by Levenshtein distance if one is within a reasonable edit distance.
func (h *ScaffolderHandler) Lint(ctx context.Context, templateName string) []domain.LintError {
	// Parse the manifest directly so we can report all issues even when the DAG is invalid.
	var tmpl domain.Template
	path := filepath.Join(".joist-templates", templateName, "manifest.yaml")
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

		// Validate post_commands
		for i, pc := range cmd.PostCommands {
			if pc.Command == "" {
				errs = append(errs, domain.LintError{
					Command: cmd.Command,
					Field:   "post_commands",
					Message: fmt.Sprintf("post_command[%d] has an empty command", i),
				})
			}
			if pc.Mode != "" && pc.Mode != "all" && pc.Mode != "per-file" {
				errs = append(errs, domain.LintError{
					Command: cmd.Command,
					Field:   "post_commands",
					Message: fmt.Sprintf("post_command[%d] has invalid mode %q (must be \"all\" or \"per-file\")", i, pc.Mode),
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

		// Check variables used in destination paths
		for _, f := range cmd.Files {
			used := extractTemplateVars(f.Destination)
			for v := range used {
				if !declared[v] {
					errs = append(errs, undeclaredErr(v, "files.destination"))
				}
			}
		}

		// Check variables used in template file source contents
		for _, f := range cmd.Files {
			if f.Source == "" {
				continue
			}
			tmplPath := filepath.Join(".joist-templates", tmpl.Name, f.Source)
			data, err := h.fs.ReadFile(ctx, tmplPath)
			if err != nil {
				errs = append(errs, domain.LintError{
					Command: cmd.Command,
					Field:   "files.source",
					Message: fmt.Sprintf("cannot read template file %q: %v", f.Source, err),
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
	}

	return errs
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
		for _, child := range n.Nodes {
			inspectNode(child, vars)
		}
	case *parse.ActionNode:
		inspectNode(n.Pipe, vars)
	case *parse.PipeNode:
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
		inspectNode(n.List, vars)
		inspectNode(n.ElseList, vars)
		inspectNode(n.Pipe, vars)
	case *parse.RangeNode:
		inspectNode(n.List, vars)
		inspectNode(n.ElseList, vars)
		inspectNode(n.Pipe, vars)
	case *parse.WithNode:
		inspectNode(n.List, vars)
		inspectNode(n.ElseList, vars)
		inspectNode(n.Pipe, vars)
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
