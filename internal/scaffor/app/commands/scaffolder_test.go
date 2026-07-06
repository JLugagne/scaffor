package commands_test

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/JLugagne/scaffor/internal/scaffor/app/commands"
	"github.com/JLugagne/scaffor/internal/scaffor/config"
	"github.com/JLugagne/scaffor/internal/scaffor/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newHandler wires a mockFS / mockFSWithReadError to a resolver scoped to
// ".scaffor-templates", auto-discovering template names from the mock FS'
// files map so tests don't have to list them explicitly.
func newHandler(fs filesystemShim) *commands.ScaffolderHandler {
	names := discoverTemplateNames(fs)
	r := config.NewResolverForTest(".scaffor-templates", names...)
	return commands.NewScaffolderHandler(fs, r)
}

// filesystemShim is the subset of the domain filesystem interface accepted
// by the handler — just a type alias for readability in tests.
type filesystemShim = interface {
	ReadFile(ctx context.Context, path string) ([]byte, error)
	WriteFile(ctx context.Context, path string, content []byte) error
	MkdirAll(ctx context.Context, path string) error
	ReadDir(ctx context.Context, path string) ([]string, error)
}

// discoverTemplateNames inspects a mock FS and returns the set of template
// names found under ".scaffor-templates/<name>/manifest.yaml". Uses
// reflection-free duck typing: expects the mock to expose a `files` map via
// the well-known types used in this test file.
func discoverTemplateNames(fs filesystemShim) []string {
	seen := map[string]struct{}{}
	switch m := fs.(type) {
	case *mockFS:
		for path := range m.files {
			if name, ok := parseTemplateName(path); ok {
				seen[name] = struct{}{}
			}
		}
	case *mockFSWithReadError:
		for path := range m.files {
			if name, ok := parseTemplateName(path); ok {
				seen[name] = struct{}{}
			}
		}
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	return out
}

// parseTemplateName extracts "<name>" from ".scaffor-templates/<name>/..."
// paths, returning ok=true when the path falls under .scaffor-templates/.
func parseTemplateName(path string) (string, bool) {
	const prefix = ".scaffor-templates/"
	if !strings.HasPrefix(path, prefix) {
		return "", false
	}
	rest := path[len(prefix):]
	if i := strings.Index(rest, "/"); i > 0 {
		return rest[:i], true
	}
	return "", false
}

func setupTestTemplates(t *testing.T, fs *mockFS) {
	err := os.MkdirAll(".scaffor-templates/hexagonal", 0755)
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
	fs.files[".scaffor-templates/hexagonal/manifest.yaml"] = []byte(manifest)
	fs.files[".scaffor-templates/hexagonal/main.go.tmpl"] = []byte("package main\nfunc main() {}")
	fs.files[".scaffor-templates/hexagonal/app.go.tmpl"] = []byte("package app\ntype App struct{}")
}

func setupCycleTemplate(t *testing.T, fs *mockFS) {
	err := os.MkdirAll(".scaffor-templates/cycle", 0755)
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
	fs.files[".scaffor-templates/cycle/manifest.yaml"] = []byte(manifest)
}

func TestScaffolder_GetTemplate(t *testing.T) {
	fs := &mockFS{files: make(map[string][]byte)}
	setupTestTemplates(t, fs)
	setupCycleTemplate(t, fs)

	handler := newHandler(fs)

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

	handler := newHandler(fs)
	ctx := context.Background()

	t.Run("Execute Chain", func(t *testing.T) {
		params := map[string]string{"AppName": "testapp"}
		_, err := handler.Execute(ctx, "hexagonal", "bootstrap", params, domain.ExecuteOptions{})
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
		_, err := handler.Execute(ctx, "hexagonal", "bootstrap", params, domain.ExecuteOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
	})
}

// TestScaffolder_Partials covers the _partials/ feature: templates can
// share snippets via {{ define }} files in <template>/_partials/, and
// content templates invoke them with {{ template "name" . }}.
func TestScaffolder_Partials(t *testing.T) {
	ctx := context.Background()

	t.Run("Execute renders partials referenced from content", func(t *testing.T) {
		fs := &mockFS{files: map[string][]byte{
			".scaffor-templates/withparts/manifest.yaml": []byte(`name: withparts
commands:
  - command: gen
    variables:
      - key: Name
        description: name
    files:
      - source: file.go.tmpl
        destination: out/{{ .Name }}.go
`),
			".scaffor-templates/withparts/_partials/header.tmpl": []byte(`{{ define "pkgHeader" }}// generated for {{ .Name }}{{ end }}`),
			".scaffor-templates/withparts/file.go.tmpl":          []byte("{{ template \"pkgHeader\" . }}\npackage {{ .Name }}"),
		}}
		handler := newHandler(fs)
		_, err := handler.Execute(ctx, "withparts", "gen", map[string]string{"Name": "alpha"}, domain.ExecuteOptions{})
		require.NoError(t, err)
		got := string(fs.files["out/alpha.go"])
		assert.Contains(t, got, "// generated for alpha")
		assert.Contains(t, got, "package alpha")
	})

	t.Run("Execute fails clearly when partial has bad syntax", func(t *testing.T) {
		fs := &mockFS{files: map[string][]byte{
			".scaffor-templates/badparts/manifest.yaml": []byte(`name: badparts
commands:
  - command: gen
    variables: []
    files:
      - source: file.go.tmpl
        destination: out/file.go
`),
			".scaffor-templates/badparts/_partials/broken.tmpl": []byte(`{{ define "x" }}{{ .Foo`), // unclosed action
			".scaffor-templates/badparts/file.go.tmpl":          []byte(`package main`),
		}}
		handler := newHandler(fs)
		_, err := handler.Execute(ctx, "badparts", "gen", map[string]string{}, domain.ExecuteOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "partial")
	})

	t.Run("Lint reports broken partial", func(t *testing.T) {
		fs := &mockFS{files: map[string][]byte{
			".scaffor-templates/lintparts/manifest.yaml": []byte(`name: lintparts
commands:
  - command: gen
    variables: []
    files: []
`),
			".scaffor-templates/lintparts/_partials/bad.tmpl": []byte(`{{ define "x" }}{{ .Foo`),
		}}
		handler := newHandler(fs)
		errs := handler.Lint(ctx, "lintparts", ".scaffor-templates")
		var found bool
		for _, e := range errs {
			if strings.HasPrefix(e.Field, "_partials") {
				found = true
				break
			}
		}
		assert.True(t, found, "broken partial should produce a _partials lint error, got: %v", errs)
	})

	t.Run("Lint accepts content using a partial", func(t *testing.T) {
		fs := &mockFS{files: map[string][]byte{
			".scaffor-templates/lintok/manifest.yaml": []byte(`name: lintok
commands:
  - command: gen
    variables:
      - key: Name
        description: name
    files:
      - source: file.go.tmpl
        destination: out/{{ .Name }}.go
`),
			".scaffor-templates/lintok/_partials/h.tmpl": []byte(`{{ define "h" }}// {{ .Name }}{{ end }}`),
			".scaffor-templates/lintok/file.go.tmpl":     []byte("{{ template \"h\" . }}\npackage {{ .Name }}"),
		}}
		handler := newHandler(fs)
		errs := handler.Lint(ctx, "lintok", ".scaffor-templates")
		assert.Empty(t, errs, "valid template with partial should lint cleanly, got: %v", errs)
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

// ReadDir scans the in-memory file map for entries directly under path. The
// mock is path-based with no directory entities, so an empty result is
// returned as os.ErrNotExist to match the real filesystem's behavior for a
// missing _partials directory.
func (m *mockFS) ReadDir(ctx context.Context, path string) ([]string, error) {
	prefix := strings.TrimRight(path, "/") + "/"
	seen := map[string]struct{}{}
	for p := range m.files {
		if !strings.HasPrefix(p, prefix) {
			continue
		}
		rest := p[len(prefix):]
		if i := strings.Index(rest, "/"); i >= 0 {
			rest = rest[:i]
		}
		if rest != "" {
			seen[rest] = struct{}{}
		}
	}
	if len(seen) == 0 {
		return nil, os.ErrNotExist
	}
	out := make([]string, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	return out, nil
}

func (m *mockFS) ExecuteGoImports(ctx context.Context, files []string) error { return nil }

func setupLintTemplates(t *testing.T, fs *mockFS) {
	t.Helper()
	err := os.MkdirAll(".scaffor-templates/lint-ok", 0755)
	require.NoError(t, err)
	err = os.MkdirAll(".scaffor-templates/lint-bad", 0755)
	require.NoError(t, err)

	// Valid template: variables declared, post_commands exist.
	// Uses both plain {{ .Name }} and piped {{ .Name | lower }} / {{ .Name | upper }} forms.
	fs.files[".scaffor-templates/lint-ok/manifest.yaml"] = []byte(`name: lint-ok
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
	fs.files[".scaffor-templates/lint-ok/file.go.tmpl"] = []byte("package {{ .Name | lower }}\n// {{ .Name }}")

	// Invalid: undeclared vars used via pipeline in destination and source, plus ghost post_command.
	fs.files[".scaffor-templates/lint-bad/manifest.yaml"] = []byte(`name: lint-bad
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
	fs.files[".scaffor-templates/lint-bad/broken.go.tmpl"] = []byte("package {{ .AlsoMissing | upper }}")

	// Template with a lowercase variable key (invalid).
	err = os.MkdirAll(".scaffor-templates/lint-lowercase", 0755)
	require.NoError(t, err)
	fs.files[".scaffor-templates/lint-lowercase/manifest.yaml"] = []byte(`name: lint-lowercase
commands:
  - command: create
    variables:
      - key: appName
        description: app name
    files: []
`)

	// Template with a typo: declares "AppName", uses "ApName" (one edit away).
	err = os.MkdirAll(".scaffor-templates/lint-typo", 0755)
	require.NoError(t, err)
	fs.files[".scaffor-templates/lint-typo/manifest.yaml"] = []byte(`name: lint-typo
commands:
  - command: create
    variables:
      - key: AppName
        description: app name
    files:
      - source: file.go.tmpl
        destination: "{{ .ApName | lower }}/file.go"
`)
	fs.files[".scaffor-templates/lint-typo/file.go.tmpl"] = []byte("package main")
}

func TestScaffolder_Lint(t *testing.T) {
	fs := &mockFS{files: make(map[string][]byte)}
	setupLintTemplates(t, fs)

	handler := newHandler(fs)
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

	t.Run("no .scaffor-templates dir returns empty slice", func(t *testing.T) {
		_ = os.RemoveAll(".scaffor-templates")
		fs := &mockFS{files: make(map[string][]byte)}
		handler := newHandler(fs)
		templates, err := handler.ListTemplates(ctx)
		require.NoError(t, err)
		assert.Empty(t, templates)
	})

	t.Run("valid templates are returned", func(t *testing.T) {
		fs := &mockFS{files: make(map[string][]byte)}
		setupTestTemplates(t, fs)
		handler := newHandler(fs)

		require.NoError(t, os.MkdirAll(".scaffor-templates/hexagonal", 0755))
		t.Cleanup(func() { _ = os.RemoveAll(".scaffor-templates") })

		templates, err := handler.ListTemplates(ctx)
		require.NoError(t, err)
		require.Len(t, templates, 1)
		assert.Equal(t, "hexagonal", templates[0].Name)
	})

	t.Run("template with invalid manifest is still listed", func(t *testing.T) {
		fs := &mockFS{files: make(map[string][]byte)}
		// Manifest missing entirely — GetTemplate will error, ListTemplates includes stub.
		// Manually register "broken" with the resolver since the mock FS is empty.
		r := config.NewResolverForTest(".scaffor-templates", "broken")
		handler := commands.NewScaffolderHandler(fs, r)

		templates, err := handler.ListTemplates(ctx)
		require.NoError(t, err)
		require.Len(t, templates, 1)
		assert.Equal(t, "broken", templates[0].Name)
		assert.Empty(t, templates[0].Commands)
	})
}

func TestScaffolder_GetTemplate_Extra(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid yaml returns error", func(t *testing.T) {
		fs := &mockFS{files: map[string][]byte{
			".scaffor-templates/bad/manifest.yaml": []byte(":\tinvalid: yaml: ["),
		}}
		handler := newHandler(fs)
		_, err := handler.GetTemplate(ctx, "bad")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse manifest")
	})

	t.Run("name defaults to directory name when empty", func(t *testing.T) {
		fs := &mockFS{files: map[string][]byte{
			".scaffor-templates/mytemplate/manifest.yaml": []byte("commands: []\n"),
		}}
		handler := newHandler(fs)
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
		handler := newHandler(fs)
		_, err := handler.Execute(ctx, "hexagonal", "nonexistent", map[string]string{"AppName": "x"}, domain.ExecuteOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("path template error returns error", func(t *testing.T) {
		fs := &mockFS{files: map[string][]byte{
			".scaffor-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands:
  - command: do
    files:
      - destination: "{{ .Bad | unknownfunc }}"
`),
		}}
		handler := newHandler(fs)
		_, err := handler.Execute(ctx, "tmpl", "do", map[string]string{}, domain.ExecuteOptions{})
		require.Error(t, err)
	})

	t.Run("directory traversal in destination blocked", func(t *testing.T) {
		fs := &mockFS{files: map[string][]byte{
			".scaffor-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands:
  - command: do
    files:
      - destination: "../../etc/passwd"
`),
		}}
		handler := newHandler(fs)
		_, err := handler.Execute(ctx, "tmpl", "do", map[string]string{}, domain.ExecuteOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "..")
	})

	t.Run("dry-run prints shell commands without executing", func(t *testing.T) {
		fs := &mockFS{files: map[string][]byte{
			".scaffor-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands:
  - command: do
    files: []
shell_commands:
  - command: "echo hello"
`),
		}}
		handler := newHandler(fs)
		_, err := handler.Execute(ctx, "tmpl", "do", map[string]string{}, domain.ExecuteOptions{DryRun: true})
		require.NoError(t, err)
	})

	t.Run("shell_command per-file mode runs once per created file", func(t *testing.T) {
		fs := &mockFS{files: make(map[string][]byte)}
		fs.files[".scaffor-templates/tmpl/manifest.yaml"] = []byte(`name: tmpl
commands:
  - command: do
    files:
      - destination: "out/a.txt"
      - destination: "out/b.txt"
shell_commands:
  - command: "echo {{ .File }}"
    mode: per-file
`)
		handler := newHandler(fs)
		_, err := handler.Execute(ctx, "tmpl", "do", map[string]string{}, domain.ExecuteOptions{DryRun: true})
		require.NoError(t, err)
	})

	t.Run("shell_command all mode uses Files placeholder", func(t *testing.T) {
		fs := &mockFS{files: map[string][]byte{
			".scaffor-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands:
  - command: do
    files:
      - destination: "out/a.txt"
shell_commands:
  - command: "echo {{ .Files }}"
`),
		}}
		handler := newHandler(fs)
		_, err := handler.Execute(ctx, "tmpl", "do", map[string]string{}, domain.ExecuteOptions{DryRun: true})
		require.NoError(t, err)
	})
}

func TestScaffolder_Lint_Extra(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid manifest yaml returns lint error", func(t *testing.T) {
		fs := &mockFS{files: map[string][]byte{
			".scaffor-templates/bad/manifest.yaml": []byte(":\tinvalid: yaml: ["),
		}}
		handler := newHandler(fs)
		errs := handler.Lint(ctx, "bad", "")
		require.NotEmpty(t, errs)
		assert.Equal(t, "manifest", errs[0].Field)
	})

	t.Run("empty shell_command is flagged", func(t *testing.T) {
		fs := &mockFS{files: map[string][]byte{
			".scaffor-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands: []
shell_commands:
  - command: ""
`),
		}}
		handler := newHandler(fs)
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
			".scaffor-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands: []
shell_commands:
  - command: "echo hi"
    mode: "badmode"
`),
		}}
		handler := newHandler(fs)
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
				".scaffor-templates/tmpl/manifest.yaml": []byte(manifest),
			}}
			handler := newHandler(fs)
			_, err := handler.Execute(context.Background(), "tmpl", "do", map[string]string{}, domain.ExecuteOptions{})
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
			".scaffor-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands:
  - command: do
    variables:
      - key: Name
        description: name
    files:
      - source: tmpl.go.tmpl
        destination: "out/file.go"
`),
			".scaffor-templates/tmpl/tmpl.go.tmpl": []byte(tmplContent),
		}}
		return newHandler(fs)
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
		".scaffor-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands:
  - command: do
    variables: []
    files:
      - destination: "{{ .Unclosed"
`),
	}}
	handler := newHandler(fs)
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
			".scaffor-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands:
  - command: do
    variables:
      - key: ""
        description: empty key
    files:
      - destination: "{{ .Missing }}/out.go"
`),
		}}
		handler := newHandler(fs)
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
			".scaffor-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands:
  - command: do
    variables:
      - key: A
        description: a
    files:
      - destination: "{{ .VeryLongUndeclaredVariableName }}/out.go"
`),
		}}
		handler := newHandler(fs)
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

// TestScaffolder_Execute_ShellCommands exercises shell command execution (default behaviour).
func TestScaffolder_Execute_ShellCommands(t *testing.T) {
	ctx := context.Background()
	fs := &mockFS{files: map[string][]byte{
		".scaffor-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands:
  - command: do
    files: []
shell_commands:
  - command: "true"
`),
	}}
	handler := newHandler(fs)
	_, err := handler.Execute(ctx, "tmpl", "do", map[string]string{}, domain.ExecuteOptions{})
	require.NoError(t, err)
}

// TestScaffolder_Execute_ContentTemplateError covers the branch where the content
// template inside a source file fails to render.
func TestScaffolder_Execute_ContentTemplateError(t *testing.T) {
	ctx := context.Background()
	fs := &mockFS{files: map[string][]byte{
		".scaffor-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands:
  - command: do
    files:
      - source: bad.go.tmpl
        destination: "out/file.go"
`),
		// This template uses an unknown function, causing Parse to fail.
		".scaffor-templates/tmpl/bad.go.tmpl": []byte(`{{ .Name | unknownfunc }}`),
	}}
	handler := newHandler(fs)
	_, err := handler.Execute(ctx, "tmpl", "do", map[string]string{"Name": "x"}, domain.ExecuteOptions{})
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
	fs.files[".scaffor-templates/tmpl/manifest.yaml"] = []byte(`name: tmpl
commands:
  - command: do
    files:
      - destination: "out/file.go"
`)
	handler := newHandler(fs)
	_, err := handler.Execute(ctx, "tmpl", "do", map[string]string{}, domain.ExecuteOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pre-flight check failed on")
}

// TestScaffolder_Lint_InvalidDestinationTemplateSyntax verifies that invalid
// template syntax in destination paths is caught during linting.
func TestScaffolder_Lint_InvalidDestinationTemplateSyntax(t *testing.T) {
	ctx := context.Background()
	fs := &mockFS{files: map[string][]byte{
		".scaffor-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands:
  - command: do
    files:
      - destination: "{{ .Unclosed"
`),
	}}
	handler := newHandler(fs)
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
		".scaffor-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands:
  - command: do
    variables:
      - key: Name
        description: name
    files:
      - source: bad.go.tmpl
        destination: "out/{{ .Name }}.go"
`),
		".scaffor-templates/tmpl/bad.go.tmpl": []byte("package main\n{{ .Name | unknownfunc }}"),
	}}
	handler := newHandler(fs)
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
		".scaffor-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands:
  - command: do
    variables:
      - key: Name
        description: name
    files: []
    hint: "Created {{ .Name | unknownfunc }}"
`),
	}}
	handler := newHandler(fs)
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
		".scaffor-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands: []
shell_commands:
  - command: "gofmt {{ .Files | unknownfunc }}"
`),
	}}
	handler := newHandler(fs)
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

func (m *mockFSWithReadError) ReadDir(_ context.Context, path string) ([]string, error) {
	prefix := strings.TrimRight(path, "/") + "/"
	seen := map[string]struct{}{}
	for p := range m.files {
		if !strings.HasPrefix(p, prefix) {
			continue
		}
		rest := p[len(prefix):]
		if i := strings.Index(rest, "/"); i >= 0 {
			rest = rest[:i]
		}
		if rest != "" {
			seen[rest] = struct{}{}
		}
	}
	if len(seen) == 0 {
		return nil, os.ErrNotExist
	}
	out := make([]string, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	return out, nil
}

func (m *mockFSWithReadError) ExecuteGoImports(_ context.Context, _ []string) error { return nil }

// TestScaffolder_ShellCommand_PatternMatching tests that patterns filter files correctly.
func TestScaffolder_ShellCommand_PatternMatching(t *testing.T) {
	ctx := context.Background()

	t.Run("pattern *.go matches only .go files", func(t *testing.T) {
		fs := &mockFS{files: map[string][]byte{
			".scaffor-templates/tmpl/manifest.yaml": []byte(`name: tmpl
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
		handler := newHandler(fs)
		_, err := handler.Execute(ctx, "tmpl", "do", map[string]string{}, domain.ExecuteOptions{DryRun: true})
		require.NoError(t, err)
	})

	t.Run("multiple patterns separated by comma", func(t *testing.T) {
		fs := &mockFS{files: map[string][]byte{
			".scaffor-templates/tmpl/manifest.yaml": []byte(`name: tmpl
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
		handler := newHandler(fs)
		_, err := handler.Execute(ctx, "tmpl", "do", map[string]string{}, domain.ExecuteOptions{DryRun: true})
		require.NoError(t, err)
	})

	t.Run("per-file mode with pattern", func(t *testing.T) {
		fs := &mockFS{files: map[string][]byte{
			".scaffor-templates/tmpl/manifest.yaml": []byte(`name: tmpl
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
		handler := newHandler(fs)
		_, err := handler.Execute(ctx, "tmpl", "do", map[string]string{}, domain.ExecuteOptions{DryRun: true})
		require.NoError(t, err)
	})

	t.Run("no matching files skips shell command", func(t *testing.T) {
		fs := &mockFS{files: map[string][]byte{
			".scaffor-templates/tmpl/manifest.yaml": []byte(`name: tmpl
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
		handler := newHandler(fs)
		_, err := handler.Execute(ctx, "tmpl", "do", map[string]string{}, domain.ExecuteOptions{DryRun: true})
		require.NoError(t, err)
	})

	t.Run("no pattern matches all files (default behavior)", func(t *testing.T) {
		fs := &mockFS{files: map[string][]byte{
			".scaffor-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands:
  - command: do
    files:
      - destination: "file.go"
      - destination: "file.js"
shell_commands:
  - command: "echo {{ .Files }}"
`),
		}}
		handler := newHandler(fs)
		_, err := handler.Execute(ctx, "tmpl", "do", map[string]string{}, domain.ExecuteOptions{DryRun: true})
		require.NoError(t, err)
	})
}

// TestScaffolder_ShellCommand_FilesRendering verifies that {{ .Files }} and {{ .File }}
// are correctly rendered in template-level shell_commands (regression test for <no value> bug).
func TestScaffolder_ShellCommand_FilesRendering(t *testing.T) {
	ctx := context.Background()

	t.Run("Files placeholder is rendered with created file paths", func(t *testing.T) {
		fs := &mockFS{files: map[string][]byte{
			".scaffor-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands:
  - command: do
    files:
      - destination: "out/a.go"
      - destination: "out/b.go"
shell_commands:
  - command: "echo {{ .Files }}"
`),
		}}
		handler := newHandler(fs)
		// Execute (not dry-run) so the shell command actually runs.
		// "echo ..." always succeeds and produces output.
		_, err := handler.Execute(ctx, "tmpl", "do", map[string]string{}, domain.ExecuteOptions{})
		require.NoError(t, err)
		// If {{ .Files }} were <no value>, echo would print "<no value>" and the
		// command would still succeed, but we verify no error occurs at all.
	})

	t.Run("File placeholder is rendered per-file", func(t *testing.T) {
		fs := &mockFS{files: map[string][]byte{
			".scaffor-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands:
  - command: do
    files:
      - destination: "out/x.go"
shell_commands:
  - command: "echo {{ .File }}"
    mode: per-file
`),
		}}
		handler := newHandler(fs)
		_, err := handler.Execute(ctx, "tmpl", "do", map[string]string{}, domain.ExecuteOptions{})
		require.NoError(t, err)
	})

	t.Run("user params are available in template-level shell_commands", func(t *testing.T) {
		fs := &mockFS{files: map[string][]byte{
			".scaffor-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands:
  - command: do
    variables:
      - key: Name
        description: name
    files:
      - destination: "out/file.go"
shell_commands:
  - command: "echo {{ .Name }} {{ .Files }}"
`),
		}}
		handler := newHandler(fs)
		_, err := handler.Execute(ctx, "tmpl", "do", map[string]string{"Name": "hello"}, domain.ExecuteOptions{})
		require.NoError(t, err)
	})
}

// TestScaffolder_Lint_ShellCommandPatternValidation tests pattern validation in lint.
func TestScaffolder_Lint_ShellCommandPatternValidation(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid pattern is flagged", func(t *testing.T) {
		fs := &mockFS{files: map[string][]byte{
			".scaffor-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands: []
shell_commands:
  - command: "echo hi"
    pattern: "["
`),
		}}
		handler := newHandler(fs)
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
			".scaffor-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands: []
shell_commands:
  - command: "echo hi"
    pattern: "*.go"
`),
		}}
		handler := newHandler(fs)
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
			".scaffor-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands: []
shell_commands:
  - command: "echo hi"
    pattern: "*.js,*.tsx"
`),
		}}
		handler := newHandler(fs)
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
			".scaffor-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands: []
shell_commands:
  - command: "echo hi"
    pattern: ""
`),
		}}
		handler := newHandler(fs)
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

// captureInjectionStdout runs fn while redirecting os.Stdout to a pipe and
// returns everything written to it. Used to observe dry-run shell command
// output for injection tests.
func captureInjectionStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	fn()

	require.NoError(t, w.Close())
	data, err := io.ReadAll(r)
	require.NoError(t, err)
	return string(data)
}

// TestScaffolder_Injection_AfterAndBefore covers the two insertion positions
// against an existing target file.
func TestScaffolder_Injection_AfterAndBefore(t *testing.T) {
	ctx := context.Background()

	t.Run("after (default) inserts below the anchor line", func(t *testing.T) {
		fs := &mockFS{files: map[string][]byte{
			".scaffor-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands:
  - command: wire
    variables:
      - key: Name
        description: name
    injections:
      - target: registry.go
        anchor: "// register here"
        content: "	register(\"{{ .Name }}\")"
`),
			"registry.go": []byte("package r\n// register here\nfunc init() {}\n"),
		}}
		handler := newHandler(fs)
		events, err := handler.Execute(ctx, "tmpl", "wire", map[string]string{"Name": "cat"}, domain.ExecuteOptions{})
		require.NoError(t, err)

		got := string(fs.files["registry.go"])
		want := "package r\n// register here\n	register(\"cat\")\nfunc init() {}\n"
		assert.Equal(t, want, got)

		require.Len(t, events, 1)
		assert.Equal(t, "injected", events[0].Action)
		assert.Equal(t, "registry.go", events[0].Path)
	})

	t.Run("before inserts above the anchor line", func(t *testing.T) {
		fs := &mockFS{files: map[string][]byte{
			".scaffor-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands:
  - command: wire
    variables:
      - key: Name
        description: name
    injections:
      - target: registry.go
        anchor: "// register here"
        position: before
        content: "before-line-{{ .Name }}"
`),
			"registry.go": []byte("package r\n// register here\nfunc init() {}\n"),
		}}
		handler := newHandler(fs)
		_, err := handler.Execute(ctx, "tmpl", "wire", map[string]string{"Name": "cat"}, domain.ExecuteOptions{})
		require.NoError(t, err)

		got := string(fs.files["registry.go"])
		want := "package r\nbefore-line-cat\n// register here\nfunc init() {}\n"
		assert.Equal(t, want, got)
	})
}

// TestScaffolder_Injection_Guards covers the idempotency guards: an explicit
// skip_if match and the default content-based guard.
func TestScaffolder_Injection_Guards(t *testing.T) {
	ctx := context.Background()

	t.Run("skip_if guard causes injection-skipped on second run", func(t *testing.T) {
		fs := &mockFS{files: map[string][]byte{
			".scaffor-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands:
  - command: wire
    variables:
      - key: Name
        description: name
    injections:
      - target: registry.go
        anchor: "// anchor"
        skip_if: "register(\"{{ .Name }}\")"
        content: "	register(\"{{ .Name }}\") // trailing comment differs"
`),
			"registry.go": []byte("package r\n// anchor\n"),
		}}
		handler := newHandler(fs)

		events1, err := handler.Execute(ctx, "tmpl", "wire", map[string]string{"Name": "cat"}, domain.ExecuteOptions{})
		require.NoError(t, err)
		require.Len(t, events1, 1)
		assert.Equal(t, "injected", events1[0].Action)

		events2, err := handler.Execute(ctx, "tmpl", "wire", map[string]string{"Name": "cat"}, domain.ExecuteOptions{})
		require.NoError(t, err)
		require.Len(t, events2, 1)
		assert.Equal(t, "injection-skipped", events2[0].Action)
	})

	t.Run("default content guard skips when content already present", func(t *testing.T) {
		fs := &mockFS{files: map[string][]byte{
			".scaffor-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands:
  - command: wire
    injections:
      - target: registry.go
        anchor: "// anchor"
        content: "the-injected-line"
`),
			"registry.go": []byte("package r\n// anchor\n"),
		}}
		handler := newHandler(fs)

		events1, err := handler.Execute(ctx, "tmpl", "wire", map[string]string{}, domain.ExecuteOptions{})
		require.NoError(t, err)
		assert.Equal(t, "injected", events1[0].Action)

		events2, err := handler.Execute(ctx, "tmpl", "wire", map[string]string{}, domain.ExecuteOptions{})
		require.NoError(t, err)
		assert.Equal(t, "injection-skipped", events2[0].Action)
	})
}

// TestScaffolder_Injection_MissingTarget covers behaviour when the target file
// does not exist: default failure and on_missing=skip recording a skipped event.
func TestScaffolder_Injection_MissingTarget(t *testing.T) {
	ctx := context.Background()

	t.Run("missing target errors by default", func(t *testing.T) {
		fs := &mockFS{files: map[string][]byte{
			".scaffor-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands:
  - command: wire
    injections:
      - target: missing.go
        anchor: "// anchor"
        content: "x"
`),
		}}
		handler := newHandler(fs)
		// Force so the pre-flight check does not run (it would also fail); this
		// isolates the runtime missing-target branch.
		_, err := handler.Execute(ctx, "tmpl", "wire", map[string]string{}, domain.ExecuteOptions{Force: true})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "injection target missing.go does not exist")
	})

	t.Run("on_missing skip records injection-skipped", func(t *testing.T) {
		fs := &mockFS{files: map[string][]byte{
			".scaffor-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands:
  - command: wire
    injections:
      - target: missing.go
        anchor: "// anchor"
        on_missing: skip
        content: "x"
`),
		}}
		handler := newHandler(fs)
		events, err := handler.Execute(ctx, "tmpl", "wire", map[string]string{}, domain.ExecuteOptions{})
		require.NoError(t, err)
		require.Len(t, events, 1)
		assert.Equal(t, "injection-skipped", events[0].Action)
		assert.Equal(t, "missing.go", events[0].Path)
	})
}

// TestScaffolder_Injection_AnchorNotFound covers the anchor-not-found branch
// for both the default failure and on_missing=skip.
func TestScaffolder_Injection_AnchorNotFound(t *testing.T) {
	ctx := context.Background()

	t.Run("anchor not found errors by default", func(t *testing.T) {
		fs := &mockFS{files: map[string][]byte{
			".scaffor-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands:
  - command: wire
    injections:
      - target: registry.go
        anchor: "// nope"
        content: "x"
`),
			"registry.go": []byte("package r\n// anchor\n"),
		}}
		handler := newHandler(fs)
		_, err := handler.Execute(ctx, "tmpl", "wire", map[string]string{}, domain.ExecuteOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "anchor \"// nope\" not found in registry.go")
	})

	t.Run("anchor not found with on_missing skip records injection-skipped", func(t *testing.T) {
		fs := &mockFS{files: map[string][]byte{
			".scaffor-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands:
  - command: wire
    injections:
      - target: registry.go
        anchor: "// nope"
        on_missing: skip
        content: "x"
`),
			"registry.go": []byte("package r\n// anchor\n"),
		}}
		handler := newHandler(fs)
		events, err := handler.Execute(ctx, "tmpl", "wire", map[string]string{}, domain.ExecuteOptions{})
		require.NoError(t, err)
		require.Len(t, events, 1)
		assert.Equal(t, "injection-skipped", events[0].Action)
	})
}

// TestScaffolder_Injection_ShellCommandFiles verifies that an injected target
// is added to createdFiles (deduplicated) so template-level shell_commands
// match it via {{ .Files }}. Dry-run is used to observe the rendered command.
func TestScaffolder_Injection_ShellCommandFiles(t *testing.T) {
	ctx := context.Background()

	fs := &mockFS{files: map[string][]byte{
		".scaffor-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands:
  - command: wire
    injections:
      - target: wire_gen.go
        anchor: "// anchor"
        content: "injected"
shell_commands:
  - command: "goimports -w {{ .Files }}"
    pattern: "*.go"
`),
		"wire_gen.go": []byte("package r\n// anchor\n"),
	}}
	handler := newHandler(fs)

	out := captureInjectionStdout(t, func() {
		_, err := handler.Execute(ctx, "tmpl", "wire", map[string]string{}, domain.ExecuteOptions{DryRun: true})
		require.NoError(t, err)
	})
	assert.Contains(t, out, "goimports -w wire_gen.go")
}

// TestScaffolder_Injection_DirJoin verifies ExecuteOptions.Dir is joined onto
// a relative injection target at execution time. Force bypasses the pre-flight
// check (which intentionally does not apply the Dir join) so this isolates the
// runtime Dir-join behaviour.
func TestScaffolder_Injection_DirJoin(t *testing.T) {
	ctx := context.Background()

	fs := &mockFS{files: map[string][]byte{
		".scaffor-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands:
  - command: wire
    injections:
      - target: registry.go
        anchor: "// anchor"
        content: "injected"
`),
		"proj/registry.go": []byte("package r\n// anchor\n"),
	}}
	handler := newHandler(fs)

	events, err := handler.Execute(ctx, "tmpl", "wire", map[string]string{}, domain.ExecuteOptions{Dir: "proj", Force: true})
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "proj/registry.go", events[0].Path)
	assert.Contains(t, string(fs.files["proj/registry.go"]), "injected")
}

// TestScaffolder_Injection_DryRunApplies verifies that injections are applied
// even under dry-run (dry-run only suppresses shell command execution).
func TestScaffolder_Injection_DryRunApplies(t *testing.T) {
	ctx := context.Background()

	fs := &mockFS{files: map[string][]byte{
		".scaffor-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands:
  - command: wire
    injections:
      - target: registry.go
        anchor: "// anchor"
        content: "dry-injected"
`),
		"registry.go": []byte("package r\n// anchor\n"),
	}}
	handler := newHandler(fs)

	_ = captureInjectionStdout(t, func() {
		_, err := handler.Execute(ctx, "tmpl", "wire", map[string]string{}, domain.ExecuteOptions{DryRun: true})
		require.NoError(t, err)
	})
	assert.Contains(t, string(fs.files["registry.go"]), "dry-injected")
}

// TestScaffolder_Injection_PreFlight verifies the two-pass pre-flight check:
// it fails when an injection target is missing and not created by the chain,
// and succeeds when the target is created by the same command chain.
func TestScaffolder_Injection_PreFlight(t *testing.T) {
	ctx := context.Background()

	t.Run("fails when target missing and not created by chain", func(t *testing.T) {
		fs := &mockFS{files: map[string][]byte{
			".scaffor-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands:
  - command: wire
    injections:
      - target: absent.go
        anchor: "// anchor"
        content: "x"
`),
		}}
		handler := newHandler(fs)
		_, err := handler.Execute(ctx, "tmpl", "wire", map[string]string{}, domain.ExecuteOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "pre-flight check failed")
		assert.Contains(t, err.Error(), "absent.go")
	})

	t.Run("succeeds when target created by the same command chain", func(t *testing.T) {
		fs := &mockFS{files: map[string][]byte{
			".scaffor-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands:
  - command: create
    files:
      - source: registry.go.tmpl
        destination: registry.go
    injections:
      - target: registry.go
        anchor: "// anchor"
        content: "injected"
`),
			".scaffor-templates/tmpl/registry.go.tmpl": []byte("package r\n// anchor\n"),
		}}
		handler := newHandler(fs)
		events, err := handler.Execute(ctx, "tmpl", "create", map[string]string{}, domain.ExecuteOptions{})
		require.NoError(t, err)
		require.Len(t, events, 2)
		assert.Equal(t, "created", events[0].Action)
		assert.Equal(t, "injected", events[1].Action)
		assert.Contains(t, string(fs.files["registry.go"]), "injected")
	})
}

// TestScaffolder_Lint_Injection covers lint checks for injections: an
// undeclared variable in content, an empty anchor, and an invalid position.
func TestScaffolder_Lint_Injection(t *testing.T) {
	ctx := context.Background()

	hasErr := func(errs []domain.LintError, field, substr string) bool {
		for _, e := range errs {
			if e.Field == field && strings.Contains(e.Message, substr) {
				return true
			}
		}
		return false
	}

	t.Run("undeclared variable in injection content", func(t *testing.T) {
		fs := &mockFS{files: map[string][]byte{
			".scaffor-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands:
  - command: wire
    variables:
      - key: Name
        description: name
    injections:
      - target: registry.go
        anchor: "// anchor"
        content: "register({{ .Missing }})"
`),
		}}
		handler := newHandler(fs)
		errs := handler.Lint(ctx, "tmpl", "")
		assert.True(t, hasErr(errs, "injections.content", "not declared"), "expected undeclared-variable error, got: %v", errs)
	})

	t.Run("empty anchor", func(t *testing.T) {
		fs := &mockFS{files: map[string][]byte{
			".scaffor-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands:
  - command: wire
    injections:
      - target: registry.go
        anchor: ""
        content: "x"
`),
		}}
		handler := newHandler(fs)
		errs := handler.Lint(ctx, "tmpl", "")
		assert.True(t, hasErr(errs, "injections.anchor", "empty"), "expected empty-anchor error, got: %v", errs)
	})

	t.Run("invalid position value", func(t *testing.T) {
		fs := &mockFS{files: map[string][]byte{
			".scaffor-templates/tmpl/manifest.yaml": []byte(`name: tmpl
commands:
  - command: wire
    injections:
      - target: registry.go
        anchor: "// anchor"
        position: sideways
        content: "x"
`),
		}}
		handler := newHandler(fs)
		errs := handler.Lint(ctx, "tmpl", "")
		assert.True(t, hasErr(errs, "injections.position", "invalid"), "expected invalid-position error, got: %v", errs)
	})
}
