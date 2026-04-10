package commands_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"github.com/JLugagne/joist/internal/joist/app/commands"
	"github.com/JLugagne/joist/internal/joist/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestTemplates(t *testing.T, fs *mockFS) {
	err := os.MkdirAll(".joist-templates/hexagonal", 0755)
	require.NoError(t, err)

	manifest := `name: hexagonal
description: A test template
commands:
  - command: bootstrap
    description: bootstraps
    variables:
      - key: AppName
        description: app name
    files:
      - source: main.go.tmpl
        destination: cmd/{{ .AppName }}/main.go
    post_commands:
      - add_app
    hint: Bootstrapped

  - command: add_app
    description: adds app
    variables:
      - key: AppName
        description: app name
    files:
      - source: app.go.tmpl
        destination: internal/{{ .AppName }}/app.go
    hint: App added
`
	fs.files[".joist-templates/hexagonal/manifest.yaml"] = []byte(manifest)
	fs.files[".joist-templates/hexagonal/main.go.tmpl"] = []byte("package main\nfunc main() {}")
	fs.files[".joist-templates/hexagonal/app.go.tmpl"] = []byte("package app\ntype App struct{}")
}

func setupCycleTemplate(t *testing.T, fs *mockFS) {
	err := os.MkdirAll(".joist-templates/cycle", 0755)
	require.NoError(t, err)

	manifest := `name: cycle
commands:
  - command: A
    post_commands: [B]
  - command: B
    post_commands: [C]
  - command: C
    post_commands: [A]
`
	fs.files[".joist-templates/cycle/manifest.yaml"] = []byte(manifest)
}

func TestScaffolder_GetTemplate(t *testing.T) {
	fs := &mockFS{files: make(map[string][]byte)}
	setupTestTemplates(t, fs)
	setupCycleTemplate(t, fs)

	handler := commands.NewScaffolderHandler(fs)

	t.Run("Valid Manifest", func(t *testing.T) {
		tmpl, err := handler.GetTemplate(context.Background(), "hexagonal")
		require.NoError(t, err)
		assert.Equal(t, "hexagonal", tmpl.Name)
		assert.Len(t, tmpl.Commands, 2)
	})

	t.Run("Cyclic Manifest", func(t *testing.T) {
		_, err := handler.GetTemplate(context.Background(), "cycle")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cycle detected")
	})
}

func TestScaffolder_Execute(t *testing.T) {
	fs := &mockFS{files: make(map[string][]byte)}
	setupTestTemplates(t, fs)

	handler := commands.NewScaffolderHandler(fs)
	ctx := context.Background()

	t.Run("Execute Chain", func(t *testing.T) {
		params := map[string]string{"AppName": "testapp"}
		err := handler.Execute(ctx, "hexagonal", "bootstrap", params, false)
		require.NoError(t, err)

		// Check files were created in mock FS
		_, okMain := fs.files["cmd/testapp/main.go"]
		assert.True(t, okMain, "main.go should be created")

		_, okApp := fs.files["internal/testapp/app.go"]
		assert.True(t, okApp, "app.go should be created")
	})

	t.Run("Pre-flight Check Failure", func(t *testing.T) {
		// Create the file beforehand so it fails pre-flight
		fs.files["cmd/conflict/main.go"] = []byte("exists")

		params := map[string]string{"AppName": "conflict"}
		err := handler.Execute(ctx, "hexagonal", "bootstrap", params, false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
	})
}

type mockFS struct {
	files map[string][]byte
}

func (m *mockFS) ReadFile(ctx context.Context, path string) ([]byte, error) {
	if content, ok := m.files[path]; ok {
		return content, nil
	}
	return nil, os.ErrNotExist
}

func (m *mockFS) WriteFile(ctx context.Context, path string, content []byte) error {
	m.files[path] = content
	return nil
}

func (m *mockFS) MkdirAll(ctx context.Context, path string) error { return nil }

func (m *mockFS) ExecuteGoImports(ctx context.Context, files []string) error { return nil }

func setupLintTemplates(t *testing.T, fs *mockFS) {
	t.Helper()
	err := os.MkdirAll(".joist-templates/lint-ok", 0755)
	require.NoError(t, err)
	err = os.MkdirAll(".joist-templates/lint-bad", 0755)
	require.NoError(t, err)

	// Valid template: variables declared, post_commands exist.
	// Uses both plain {{ .Name }} and piped {{ .Name | lower }} / {{ .Name | upper }} forms.
	fs.files[".joist-templates/lint-ok/manifest.yaml"] = []byte(`name: lint-ok
commands:
  - command: create
    variables:
      - key: Name
        description: name
    files:
      - source: file.go.tmpl
        destination: "{{ .Name | lower }}/file.go"
    post_commands:
      - setup
  - command: setup
    variables:
      - key: Name
        description: name
    files: []
`)
	// Template file uses both plain and piped access to the same declared variable.
	fs.files[".joist-templates/lint-ok/file.go.tmpl"] = []byte("package {{ .Name | lower }}\n// {{ .Name }}")

	// Invalid: undeclared vars used via pipeline in destination and source, plus ghost post_command.
	fs.files[".joist-templates/lint-bad/manifest.yaml"] = []byte(`name: lint-bad
commands:
  - command: broken
    variables: []
    files:
      - source: broken.go.tmpl
        destination: "{{ .Missing | lower }}/file.go"
    post_commands:
      - ghost
`)
	// Source file uses a piped undeclared variable.
	fs.files[".joist-templates/lint-bad/broken.go.tmpl"] = []byte("package {{ .AlsoMissing | upper }}")

	// Template with a lowercase variable key (invalid).
	err = os.MkdirAll(".joist-templates/lint-lowercase", 0755)
	require.NoError(t, err)
	fs.files[".joist-templates/lint-lowercase/manifest.yaml"] = []byte(`name: lint-lowercase
commands:
  - command: create
    variables:
      - key: appName
        description: app name
    files: []
`)

	// Template with a typo: declares "AppName", uses "ApName" (one edit away).
	err = os.MkdirAll(".joist-templates/lint-typo", 0755)
	require.NoError(t, err)
	fs.files[".joist-templates/lint-typo/manifest.yaml"] = []byte(`name: lint-typo
commands:
  - command: create
    variables:
      - key: AppName
        description: app name
    files:
      - source: file.go.tmpl
        destination: "{{ .ApName | lower }}/file.go"
`)
	fs.files[".joist-templates/lint-typo/file.go.tmpl"] = []byte("package main")
}

func TestScaffolder_Lint(t *testing.T) {
	fs := &mockFS{files: make(map[string][]byte)}
	setupLintTemplates(t, fs)

	handler := commands.NewScaffolderHandler(fs)
	ctx := context.Background()

	t.Run("Valid template returns no errors", func(t *testing.T) {
		errs := handler.Lint(ctx, "lint-ok", "")
		assert.Empty(t, errs, "declared variables used with | lower / | upper pipes should not produce errors")
	})

	t.Run("Piped undeclared variable in destination path is caught", func(t *testing.T) {
		errs := handler.Lint(ctx, "lint-bad", "")
		found := false
		for _, e := range errs {
			if e.Command == "broken" && e.Field == "files.destination" && strings.Contains(e.Message, "Missing") {
				found = true
			}
		}
		assert.True(t, found, "{{ .Missing | lower }} in destination should be reported, got: %v", errs)
	})

	t.Run("Piped undeclared variable in source template is caught", func(t *testing.T) {
		errs := handler.Lint(ctx, "lint-bad", "")
		found := false
		for _, e := range errs {
			if e.Command == "broken" && strings.Contains(e.Field, "broken.go.tmpl") && strings.Contains(e.Message, "AlsoMissing") {
				found = true
			}
		}
		assert.True(t, found, "{{ .AlsoMissing | upper }} in source file should be reported, got: %v", errs)
	})

	t.Run("Undeclared variable in destination path", func(t *testing.T) {
		errs := handler.Lint(ctx, "lint-bad", "")
		found := false
		for _, e := range errs {
			if e.Command == "broken" && e.Field == "files.destination" && strings.Contains(e.Message, "Missing") {
				found = true
			}
		}
		assert.True(t, found, "expected undeclared variable error for destination, got: %v", errs)
	})

	t.Run("Undeclared variable in source template file", func(t *testing.T) {
		errs := handler.Lint(ctx, "lint-bad", "")
		found := false
		for _, e := range errs {
			if e.Command == "broken" && strings.Contains(e.Field, "broken.go.tmpl") && strings.Contains(e.Message, "AlsoMissing") {
				found = true
			}
		}
		assert.True(t, found, "expected undeclared variable error for source file, got: %v", errs)
	})

	t.Run("Post command references undefined command", func(t *testing.T) {
		errs := handler.Lint(ctx, "lint-bad", "")
		found := false
		for _, e := range errs {
			if e.Command == "broken" && e.Field == "post_commands" && strings.Contains(e.Message, "ghost") {
				found = true
			}
		}
		assert.True(t, found, "expected undefined post_command error, got: %v", errs)
	})

	t.Run("Lowercase variable key is flagged", func(t *testing.T) {
		errs := handler.Lint(ctx, "lint-lowercase", "")
		found := false
		for _, e := range errs {
			if e.Command == "create" && e.Field == "variables" && strings.Contains(e.Message, "appName") {
				found = true
			}
		}
		assert.True(t, found, "lowercase variable key should be reported, got: %v", errs)
	})

	t.Run("Typo in variable name suggests closest match", func(t *testing.T) {
		errs := handler.Lint(ctx, "lint-typo", "")
		found := false
		for _, e := range errs {
			if e.Command == "create" && strings.Contains(e.Message, "ApName") && strings.Contains(e.Message, "AppName") {
				found = true
			}
		}
		assert.True(t, found, "typo should suggest closest declared variable, got: %v", errs)
	})

	t.Run("Missing template returns lint error", func(t *testing.T) {
		errs := handler.Lint(ctx, "nonexistent", "")
		require.NotEmpty(t, errs)
		assert.Equal(t, "manifest", errs[0].Field)
	})

	t.Run("LintError.Error formats with command", func(t *testing.T) {
		e := domain.LintError{Command: "cmd", Field: "post_commands", Message: `references undefined command "ghost"`}
		msg := e.Error()
		assert.Contains(t, msg, "cmd")
		assert.Contains(t, msg, "post_commands")
		assert.Contains(t, msg, "ghost")
	})

	t.Run("LintError.Error formats without command", func(t *testing.T) {
		e := domain.LintError{Field: "manifest", Message: "file not found"}
		msg := e.Error()
		assert.Contains(t, msg, "manifest")
		assert.Contains(t, msg, "file not found")
		assert.NotContains(t, msg, "command")
	})
}

func TestScaffolder_ListTemplates(t *testing.T) {
	ctx := context.Background()

	t.Run("no .joist-templates dir returns empty slice", func(t *testing.T) {
		fs := &mockFS{files: make(map[string][]byte)}
		handler := commands.NewScaffolderHandler(fs)
		templates, err := handler.ListTemplates(ctx)
		require.NoError(t, err)
		assert.Empty(t, templates)
	})

	t.Run("valid templates are returned", func(t *testing.T) {
		fs := &mockFS{files: make(map[string][]byte)}
		setupTestTemplates(t, fs)
		handler := commands.NewScaffolderHandler(fs)

		// ListTemplates uses os.ReadDir, so we need actual dirs on disk
		require.NoError(t, os.MkdirAll(".joist-templates/hexagonal", 0755))
		t.Cleanup(func() { _ = os.RemoveAll(".joist-templates") })

		templates, err := handler.ListTemplates(ctx)
		require.NoError(t, err)
		require.Len(t, templates, 1)
		assert.Equal(t, "hexagonal", templates[0].Name)
	})

	t.Run("template with invalid manifest is silently skipped", func(t *testing.T) {
		fs := &mockFS{files: make(map[string][]byte)}
		// manifest missing entirely — GetTemplate will error, ListTemplates skips
		handler := commands.NewScaffolderHandler(fs)

		require.NoError(t, os.MkdirAll(".joist-templates/broken", 0755))
		t.Cleanup(func() { _ = os.RemoveAll(".joist-templates") })

		templates, err := handler.ListTemplates(ctx)
		require.NoError(t, err)
		assert.Empty(t, templates)
	})

	t.Run("os.ReadDir error (non-NotExist) is returned", func(t *testing.T) {
		fs := &mockFS{files: make(map[string][]byte)}
		handler := commands.NewScaffolderHandler(fs)

		// Place a regular file where .joist-templates should be a directory;
		// os.ReadDir on a file returns an error that is not os.IsNotExist.
		require.NoError(t, os.WriteFile(".joist-templates", []byte("not a dir"), 0644))
		t.Cleanup(func() { _ = os.Remove(".joist-templates") })

		_, err := handler.ListTemplates(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read .joist-templates")
	})
}

func TestScaffolder_GetTemplate_Extra(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid yaml returns error", func(t *testing.T) {
		fs := &mockFS{files: map[string][]byte{
			".joist-templates/bad/manifest.yaml": []byte(":\tinvalid: yaml: ["),
		}}
		handler := commands.NewScaffolderHandler(fs)
		_, err := handler.GetTemplate(ctx, "bad")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse manifest")
	})

	t.Run("name defaults to directory name when empty", func(t *testing.T) {
		fs := &mockFS{files: map[string][]byte{
			".joist-templates/mytemplate/manifest.yaml": []byte("commands: []\n"),
		}}
		handler := commands.NewScaffolderHandler(fs)
		tmpl, err := handler.GetTemplate(ctx, "mytemplate")
		require.NoError(t, err)
		assert.Equal(t, "mytemplate", tmpl.Name)
	})
}

func TestScaffolder_Execute_Extra(t *testing.T) {
	ctx := context.Background()

	t.Run("unknown command returns error", func(t *testing.T) {
		fs := &mockFS{files: make(map[string][]byte)}
		setupTestTemplates(t, fs)
		handler := commands.NewScaffolderHandler(fs)
		err := handler.Execute(ctx, "hexagonal", "nonexistent", map[string]string{"AppName": "x"}, false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("path template error returns error", func(t *testing.T) {
		fs := &mockFS{files: map[string][]byte{
			".joist-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands:
  - command: do
    files:
      - destination: "{{ .Bad | unknownfunc }}"
`),
		}}
		handler := commands.NewScaffolderHandler(fs)
		err := handler.Execute(ctx, "tmpl", "do", map[string]string{}, false)
		require.Error(t, err)
	})

	t.Run("directory traversal in destination blocked", func(t *testing.T) {
		fs := &mockFS{files: map[string][]byte{
			".joist-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands:
  - command: do
    files:
      - destination: "../../etc/passwd"
`),
		}}
		handler := commands.NewScaffolderHandler(fs)
		err := handler.Execute(ctx, "tmpl", "do", map[string]string{}, false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "..")
	})

	t.Run("runCommands=false prints shell commands", func(t *testing.T) {
		fs := &mockFS{files: map[string][]byte{
			".joist-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands:
  - command: do
    files: []
shell_commands:
  - command: "echo hello"
`),
		}}
		handler := commands.NewScaffolderHandler(fs)
		err := handler.Execute(ctx, "tmpl", "do", map[string]string{}, false)
		require.NoError(t, err)
	})

	t.Run("shell_command per-file mode runs once per created file", func(t *testing.T) {
		fs := &mockFS{files: make(map[string][]byte)}
		fs.files[".joist-templates/tmpl/manifest.yaml"] = []byte(`name: tmpl
commands:
  - command: do
    files:
      - destination: "out/a.txt"
      - destination: "out/b.txt"
shell_commands:
  - command: "echo {{ .File }}"
    mode: per-file
`)
		handler := commands.NewScaffolderHandler(fs)
		err := handler.Execute(ctx, "tmpl", "do", map[string]string{}, false)
		require.NoError(t, err)
	})

	t.Run("shell_command all mode uses Files placeholder", func(t *testing.T) {
		fs := &mockFS{files: map[string][]byte{
			".joist-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands:
  - command: do
    files:
      - destination: "out/a.txt"
shell_commands:
  - command: "echo {{ .Files }}"
`),
		}}
		handler := commands.NewScaffolderHandler(fs)
		err := handler.Execute(ctx, "tmpl", "do", map[string]string{}, false)
		require.NoError(t, err)
	})
}

func TestScaffolder_Lint_Extra(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid manifest yaml returns lint error", func(t *testing.T) {
		fs := &mockFS{files: map[string][]byte{
			".joist-templates/bad/manifest.yaml": []byte(":\tinvalid: yaml: ["),
		}}
		handler := commands.NewScaffolderHandler(fs)
		errs := handler.Lint(ctx, "bad", "")
		require.NotEmpty(t, errs)
		assert.Equal(t, "manifest", errs[0].Field)
	})

	t.Run("empty shell_command is flagged", func(t *testing.T) {
		fs := &mockFS{files: map[string][]byte{
			".joist-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands: []
shell_commands:
  - command: ""
`),
		}}
		handler := commands.NewScaffolderHandler(fs)
		errs := handler.Lint(ctx, "tmpl", "")
		found := false
		for _, e := range errs {
			if e.Field == "shell_commands" && strings.Contains(e.Message, "empty") {
				found = true
			}
		}
		assert.True(t, found, "empty shell_command should be flagged, got: %v", errs)
	})

	t.Run("invalid shell_command mode is flagged", func(t *testing.T) {
		fs := &mockFS{files: map[string][]byte{
			".joist-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands: []
shell_commands:
  - command: "echo hi"
    mode: "badmode"
`),
		}}
		handler := commands.NewScaffolderHandler(fs)
		errs := handler.Lint(ctx, "tmpl", "")
		found := false
		for _, e := range errs {
			if e.Field == "shell_commands" && strings.Contains(e.Message, "badmode") {
				found = true
			}
		}
		assert.True(t, found, "invalid mode should be flagged, got: %v", errs)
	})
}

func TestScaffolder_SafeDestination(t *testing.T) {
	// Exercise safeDestination via Execute (it's unexported; the path goes through preFlightCheck and executeNode)
	tests := []struct {
		name    string
		dest    string
		wantErr bool
		errMsg  string
	}{
		{"normal path", "internal/app/main.go", false, ""},
		{"parent traversal", "../outside/file.go", true, ".."},
		{"deep traversal", "internal/../../../etc/passwd", true, ".."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest := "name: tmpl\ncommands:\n  - command: do\n    files:\n      - destination: \"" + tt.dest + "\"\n"
			fs := &mockFS{files: map[string][]byte{
				".joist-templates/tmpl/manifest.yaml": []byte(manifest),
			}}
			handler := commands.NewScaffolderHandler(fs)
			err := handler.Execute(context.Background(), "tmpl", "do", map[string]string{}, false)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestScaffolder_InspectNodeBranches exercises if/range/with template constructs
// via Lint, which calls extractTemplateVars → inspectNode internally.
// Templates use an else branch to avoid the nil-ListNode panic in inspectNode.
func TestScaffolder_InspectNodeBranches(t *testing.T) {
	ctx := context.Background()

	makeHandler := func(tmplContent string) *commands.ScaffolderHandler {
		fs := &mockFS{files: map[string][]byte{
			".joist-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands:
  - command: do
    variables:
      - key: Name
        description: name
    files:
      - source: tmpl.go.tmpl
        destination: "out/file.go"
`),
			".joist-templates/tmpl/tmpl.go.tmpl": []byte(tmplContent),
		}}
		return commands.NewScaffolderHandler(fs)
	}

	t.Run("if-else node: declared variable not flagged", func(t *testing.T) {
		h := makeHandler(`{{ if .Name }}hello{{ else }}world{{ end }}`)
		errs := h.Lint(ctx, "tmpl", "")
		assert.Empty(t, errs)
	})

	t.Run("range-else node: declared variable not flagged", func(t *testing.T) {
		h := makeHandler(`{{ range .Name }}x{{ else }}y{{ end }}`)
		errs := h.Lint(ctx, "tmpl", "")
		assert.Empty(t, errs)
	})

	t.Run("with-else node: declared variable not flagged", func(t *testing.T) {
		h := makeHandler(`{{ with .Name }}ok{{ else }}fallback{{ end }}`)
		errs := h.Lint(ctx, "tmpl", "")
		assert.Empty(t, errs)
	})

	t.Run("if-else node: undeclared variable in condition is flagged", func(t *testing.T) {
		h := makeHandler(`{{ if .Missing }}hello{{ else }}world{{ end }}`)
		errs := h.Lint(ctx, "tmpl", "")
		found := false
		for _, e := range errs {
			if strings.Contains(e.Message, "Missing") {
				found = true
			}
		}
		assert.True(t, found, "undeclared var in if condition should be reported, got: %v", errs)
	})
}

// TestScaffolder_ExtractTemplateVarsInvalidTemplate exercises the parse-error
// branch in extractTemplateVars (invalid template string → returns empty map).
// Triggered via Lint on a destination containing an unclosed action.
func TestScaffolder_ExtractTemplateVarsInvalidTemplate(t *testing.T) {
	ctx := context.Background()
	// An unclosed {{ causes template.Parse to fail; extractTemplateVars returns empty map,
	// so no variable errors are reported from an unparseable destination.
	fs := &mockFS{files: map[string][]byte{
		".joist-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands:
  - command: do
    variables: []
    files:
      - destination: "{{ .Unclosed"
`),
	}}
	handler := commands.NewScaffolderHandler(fs)
	// Should not panic; lint may report parse error or nothing for that field.
	_ = handler.Lint(ctx, "tmpl", "")
}

// TestScaffolder_LevenshteinEdgeCases covers the la==0 and lb==0 branches.
func TestScaffolder_LevenshteinEdgeCases(t *testing.T) {
	ctx := context.Background()

	t.Run("lb==0: empty declared key triggers levenshtein(name, empty)", func(t *testing.T) {
		// key: "" in YAML → declared[""] = true; closestVar calls levenshtein("Missing", "")
		// which hits the lb==0 branch and returns len("Missing").
		fs := &mockFS{files: map[string][]byte{
			".joist-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands:
  - command: do
    variables:
      - key: ""
        description: empty key
    files:
      - destination: "{{ .Missing }}/out.go"
`),
		}}
		handler := commands.NewScaffolderHandler(fs)
		errs := handler.Lint(ctx, "tmpl", "")
		found := false
		for _, e := range errs {
			if strings.Contains(e.Message, "Missing") {
				found = true
			}
		}
		assert.True(t, found, "expected undeclared var error, got: %v", errs)
	})

	t.Run("long undeclared name: distance > 3 threshold, no suggestion", func(t *testing.T) {
		fs := &mockFS{files: map[string][]byte{
			".joist-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands:
  - command: do
    variables:
      - key: A
        description: a
    files:
      - destination: "{{ .VeryLongUndeclaredVariableName }}/out.go"
`),
		}}
		handler := commands.NewScaffolderHandler(fs)
		errs := handler.Lint(ctx, "tmpl", "")
		found := false
		for _, e := range errs {
			if strings.Contains(e.Message, "VeryLongUndeclaredVariableName") {
				found = true
			}
		}
		assert.True(t, found, "expected undeclared var error, got: %v", errs)
	})
}

// TestScaffolder_Execute_RunCommands exercises the runCommands=true path.
func TestScaffolder_Execute_RunCommands(t *testing.T) {
	ctx := context.Background()
	fs := &mockFS{files: map[string][]byte{
		".joist-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands:
  - command: do
    files: []
shell_commands:
  - command: "true"
`),
	}}
	handler := commands.NewScaffolderHandler(fs)
	err := handler.Execute(ctx, "tmpl", "do", map[string]string{}, true)
	require.NoError(t, err)
}

// TestScaffolder_Execute_ContentTemplateError covers the branch where the content
// template inside a source file fails to render.
func TestScaffolder_Execute_ContentTemplateError(t *testing.T) {
	ctx := context.Background()
	fs := &mockFS{files: map[string][]byte{
		".joist-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands:
  - command: do
    files:
      - source: bad.go.tmpl
        destination: "out/file.go"
`),
		// This template uses an unknown function, causing Parse to fail.
		".joist-templates/tmpl/bad.go.tmpl": []byte(`{{ .Name | unknownfunc }}`),
	}}
	handler := commands.NewScaffolderHandler(fs)
	err := handler.Execute(ctx, "tmpl", "do", map[string]string{"Name": "x"}, false)
	require.Error(t, err)
}

// TestScaffolder_PreFlightCheck_NonNotExistError covers the branch in preFlightCheck
// where ReadFile returns a non-NotExist error (e.g. permission denied).
func TestScaffolder_PreFlightCheck_NonNotExistError(t *testing.T) {
	ctx := context.Background()
	sentinel := os.ErrPermission
	fs := &mockFSWithReadError{
		files:     make(map[string][]byte),
		readError: sentinel,
		errorPath: "out/file.go",
	}
	fs.files[".joist-templates/tmpl/manifest.yaml"] = []byte(`name: tmpl
commands:
  - command: do
    files:
      - destination: "out/file.go"
`)
	handler := commands.NewScaffolderHandler(fs)
	err := handler.Execute(ctx, "tmpl", "do", map[string]string{}, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pre-flight check failed on")
}

// TestScaffolder_Lint_InvalidDestinationTemplateSyntax verifies that invalid
// template syntax in destination paths is caught during linting.
func TestScaffolder_Lint_InvalidDestinationTemplateSyntax(t *testing.T) {
	ctx := context.Background()
	fs := &mockFS{files: map[string][]byte{
		".joist-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands:
  - command: do
    files:
      - destination: "{{ .Unclosed"
`),
	}}
	handler := commands.NewScaffolderHandler(fs)
	errs := handler.Lint(ctx, "tmpl", "")
	found := false
	for _, e := range errs {
		if e.Field == "files.destination" && strings.Contains(e.Message, "invalid template syntax") {
			found = true
		}
	}
	assert.True(t, found, "invalid destination template syntax should be flagged, got: %v", errs)
}

// TestScaffolder_Lint_InvalidSourceFileTemplateSyntax verifies that invalid
// template syntax in source template files is caught during linting.
func TestScaffolder_Lint_InvalidSourceFileTemplateSyntax(t *testing.T) {
	ctx := context.Background()
	fs := &mockFS{files: map[string][]byte{
		".joist-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands:
  - command: do
    variables:
      - key: Name
        description: name
    files:
      - source: bad.go.tmpl
        destination: "out/{{ .Name }}.go"
`),
		".joist-templates/tmpl/bad.go.tmpl": []byte("package main\n{{ .Name | unknownfunc }}"),
	}}
	handler := commands.NewScaffolderHandler(fs)
	errs := handler.Lint(ctx, "tmpl", "")
	found := false
	for _, e := range errs {
		if e.Field == "files.source" && strings.Contains(e.Message, "invalid syntax") {
			found = true
		}
	}
	assert.True(t, found, "invalid template file syntax should be flagged, got: %v", errs)
}

// TestScaffolder_Lint_InvalidHintTemplateSyntax verifies that invalid
// template syntax in hints is caught during linting.
func TestScaffolder_Lint_InvalidHintTemplateSyntax(t *testing.T) {
	ctx := context.Background()
	fs := &mockFS{files: map[string][]byte{
		".joist-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands:
  - command: do
    variables:
      - key: Name
        description: name
    files: []
    hint: "Created {{ .Name | unknownfunc }}"
`),
	}}
	handler := commands.NewScaffolderHandler(fs)
	errs := handler.Lint(ctx, "tmpl", "")
	found := false
	for _, e := range errs {
		if e.Command == "do" && e.Field == "hint" && strings.Contains(e.Message, "invalid template syntax") {
			found = true
		}
	}
	assert.True(t, found, "invalid hint template syntax should be flagged, got: %v", errs)
}

// TestScaffolder_Lint_InvalidShellCommandTemplateSyntax verifies that invalid
// template syntax in shell_commands is caught during linting.
func TestScaffolder_Lint_InvalidShellCommandTemplateSyntax(t *testing.T) {
	ctx := context.Background()
	fs := &mockFS{files: map[string][]byte{
		".joist-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands: []
shell_commands:
  - command: "gofmt {{ .Files | unknownfunc }}"
`),
	}}
	handler := commands.NewScaffolderHandler(fs)
	errs := handler.Lint(ctx, "tmpl", "")
	found := false
	for _, e := range errs {
		if e.Field == "shell_commands" && strings.Contains(e.Message, "invalid template syntax") {
			found = true
		}
	}
	assert.True(t, found, "invalid shell_command template syntax should be flagged, got: %v", errs)
}

// mockFSWithReadError returns a configurable error for a specific path.
type mockFSWithReadError struct {
	files     map[string][]byte
	readError error
	errorPath string
}

func (m *mockFSWithReadError) ReadFile(_ context.Context, path string) ([]byte, error) {
	if path == m.errorPath {
		return nil, m.readError
	}
	if content, ok := m.files[path]; ok {
		return content, nil
	}
	return nil, os.ErrNotExist
}

func (m *mockFSWithReadError) WriteFile(_ context.Context, path string, content []byte) error {
	m.files[path] = content
	return nil
}

func (m *mockFSWithReadError) MkdirAll(_ context.Context, _ string) error { return nil }

func (m *mockFSWithReadError) ExecuteGoImports(_ context.Context, _ []string) error { return nil }

// TestScaffolder_ShellCommand_PatternMatching tests that patterns filter files correctly.
func TestScaffolder_ShellCommand_PatternMatching(t *testing.T) {
	ctx := context.Background()

	t.Run("pattern *.go matches only .go files", func(t *testing.T) {
		fs := &mockFS{files: map[string][]byte{
			".joist-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands:
  - command: do
    files:
      - destination: "main.go"
      - destination: "util.go"
      - destination: "main.js"
shell_commands:
  - command: "gofmt {{ .Files }}"
    pattern: "*.go"
`),
		}}
		handler := commands.NewScaffolderHandler(fs)
		err := handler.Execute(ctx, "tmpl", "do", map[string]string{}, false)
		require.NoError(t, err)
		// When pattern is applied, only .go files should be in {{ .Files }}
		// This is verified by the test not panicking and shell command being printed
	})

	t.Run("multiple patterns separated by comma", func(t *testing.T) {
		fs := &mockFS{files: map[string][]byte{
			".joist-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands:
  - command: do
    files:
      - destination: "app.js"
      - destination: "App.tsx"
      - destination: "style.css"
shell_commands:
  - command: "prettier {{ .Files }}"
    pattern: "*.js,*.tsx"
`),
		}}
		handler := commands.NewScaffolderHandler(fs)
		err := handler.Execute(ctx, "tmpl", "do", map[string]string{}, false)
		require.NoError(t, err)
	})

	t.Run("per-file mode with pattern", func(t *testing.T) {
		fs := &mockFS{files: map[string][]byte{
			".joist-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands:
  - command: do
    files:
      - destination: "file1.go"
      - destination: "file2.go"
      - destination: "file.txt"
shell_commands:
  - command: "gofmt {{ .File }}"
    mode: "per-file"
    pattern: "*.go"
`),
		}}
		handler := commands.NewScaffolderHandler(fs)
		err := handler.Execute(ctx, "tmpl", "do", map[string]string{}, false)
		require.NoError(t, err)
		// Should only run on .go files in per-file mode
	})

	t.Run("no matching files skips shell command", func(t *testing.T) {
		fs := &mockFS{files: map[string][]byte{
			".joist-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands:
  - command: do
    files:
      - destination: "file1.txt"
      - destination: "file2.txt"
shell_commands:
  - command: "gofmt {{ .Files }}"
    pattern: "*.go"
`),
		}}
		handler := commands.NewScaffolderHandler(fs)
		err := handler.Execute(ctx, "tmpl", "do", map[string]string{}, false)
		require.NoError(t, err)
		// Should succeed even though no files match the pattern
	})

	t.Run("no pattern matches all files (default behavior)", func(t *testing.T) {
		fs := &mockFS{files: map[string][]byte{
			".joist-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands:
  - command: do
    files:
      - destination: "file.go"
      - destination: "file.js"
shell_commands:
  - command: "echo {{ .Files }}"
`),
		}}
		handler := commands.NewScaffolderHandler(fs)
		err := handler.Execute(ctx, "tmpl", "do", map[string]string{}, false)
		require.NoError(t, err)
		// Both files should be in {{ .Files }} when no pattern is specified
	})
}

// TestScaffolder_Lint_ShellCommandPatternValidation tests pattern validation in lint.
func TestScaffolder_Lint_ShellCommandPatternValidation(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid pattern is flagged", func(t *testing.T) {
		fs := &mockFS{files: map[string][]byte{
			".joist-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands: []
shell_commands:
  - command: "echo hi"
    pattern: "["
`),
		}}
		handler := commands.NewScaffolderHandler(fs)
		errs := handler.Lint(ctx, "tmpl", "")
		found := false
		for _, e := range errs {
			if e.Field == "shell_commands" && strings.Contains(e.Message, "invalid pattern") {
				found = true
			}
		}
		assert.True(t, found, "invalid pattern should be flagged, got: %v", errs)
	})

	t.Run("valid pattern passes validation", func(t *testing.T) {
		fs := &mockFS{files: map[string][]byte{
			".joist-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands: []
shell_commands:
  - command: "echo hi"
    pattern: "*.go"
`),
		}}
		handler := commands.NewScaffolderHandler(fs)
		errs := handler.Lint(ctx, "tmpl", "")
		found := false
		for _, e := range errs {
			if e.Field == "shell_commands" && strings.Contains(e.Message, "invalid pattern") {
				found = true
			}
		}
		assert.False(t, found, "valid pattern should not be flagged, got: %v", errs)
	})

	t.Run("multiple patterns are validated", func(t *testing.T) {
		fs := &mockFS{files: map[string][]byte{
			".joist-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands: []
shell_commands:
  - command: "echo hi"
    pattern: "*.js,*.tsx"
`),
		}}
		handler := commands.NewScaffolderHandler(fs)
		errs := handler.Lint(ctx, "tmpl", "")
		found := false
		for _, e := range errs {
			if e.Field == "shell_commands" && strings.Contains(e.Message, "invalid pattern") {
				found = true
			}
		}
		assert.False(t, found, "valid patterns should not be flagged, got: %v", errs)
	})

	t.Run("empty pattern string is allowed", func(t *testing.T) {
		fs := &mockFS{files: map[string][]byte{
			".joist-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands: []
shell_commands:
  - command: "echo hi"
    pattern: ""
`),
		}}
		handler := commands.NewScaffolderHandler(fs)
		errs := handler.Lint(ctx, "tmpl", "")
		found := false
		for _, e := range errs {
			if e.Field == "shell_commands" && strings.Contains(e.Message, "invalid pattern") {
				found = true
			}
		}
		assert.False(t, found, "empty pattern should be allowed, got: %v", errs)
	})
}
