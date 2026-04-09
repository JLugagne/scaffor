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
		errs := handler.Lint(ctx, "lint-ok")
		assert.Empty(t, errs, "declared variables used with | lower / | upper pipes should not produce errors")
	})

	t.Run("Piped undeclared variable in destination path is caught", func(t *testing.T) {
		errs := handler.Lint(ctx, "lint-bad")
		found := false
		for _, e := range errs {
			if e.Command == "broken" && e.Field == "files.destination" && strings.Contains(e.Message, "Missing") {
				found = true
			}
		}
		assert.True(t, found, "{{ .Missing | lower }} in destination should be reported, got: %v", errs)
	})

	t.Run("Piped undeclared variable in source template is caught", func(t *testing.T) {
		errs := handler.Lint(ctx, "lint-bad")
		found := false
		for _, e := range errs {
			if e.Command == "broken" && strings.Contains(e.Field, "broken.go.tmpl") && strings.Contains(e.Message, "AlsoMissing") {
				found = true
			}
		}
		assert.True(t, found, "{{ .AlsoMissing | upper }} in source file should be reported, got: %v", errs)
	})

	t.Run("Undeclared variable in destination path", func(t *testing.T) {
		errs := handler.Lint(ctx, "lint-bad")
		found := false
		for _, e := range errs {
			if e.Command == "broken" && e.Field == "files.destination" && strings.Contains(e.Message, "Missing") {
				found = true
			}
		}
		assert.True(t, found, "expected undeclared variable error for destination, got: %v", errs)
	})

	t.Run("Undeclared variable in source template file", func(t *testing.T) {
		errs := handler.Lint(ctx, "lint-bad")
		found := false
		for _, e := range errs {
			if e.Command == "broken" && strings.Contains(e.Field, "broken.go.tmpl") && strings.Contains(e.Message, "AlsoMissing") {
				found = true
			}
		}
		assert.True(t, found, "expected undeclared variable error for source file, got: %v", errs)
	})

	t.Run("Post command references undefined command", func(t *testing.T) {
		errs := handler.Lint(ctx, "lint-bad")
		found := false
		for _, e := range errs {
			if e.Command == "broken" && e.Field == "post_commands" && strings.Contains(e.Message, "ghost") {
				found = true
			}
		}
		assert.True(t, found, "expected undefined post_command error, got: %v", errs)
	})

	t.Run("Lowercase variable key is flagged", func(t *testing.T) {
		errs := handler.Lint(ctx, "lint-lowercase")
		found := false
		for _, e := range errs {
			if e.Command == "create" && e.Field == "variables" && strings.Contains(e.Message, "appName") {
				found = true
			}
		}
		assert.True(t, found, "lowercase variable key should be reported, got: %v", errs)
	})

	t.Run("Typo in variable name suggests closest match", func(t *testing.T) {
		errs := handler.Lint(ctx, "lint-typo")
		found := false
		for _, e := range errs {
			if e.Command == "create" && strings.Contains(e.Message, "ApName") && strings.Contains(e.Message, "AppName") {
				found = true
			}
		}
		assert.True(t, found, "typo should suggest closest declared variable, got: %v", errs)
	})

	t.Run("Missing template returns lint error", func(t *testing.T) {
		errs := handler.Lint(ctx, "nonexistent")
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
