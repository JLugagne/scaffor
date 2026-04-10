package mcp_test

import (
	"context"
	"testing"

	"github.com/JLugagne/joist/internal/joist/domain"
	joistmlcp "github.com/JLugagne/joist/internal/joist/inbound/mcp"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockScaffolder is a test double for service.ScaffolderCommands.
type mockScaffolder struct {
	templates  []domain.Template
	executeErr error
	lintErrors []domain.LintError
}

func (m *mockScaffolder) ListTemplates(_ context.Context) ([]domain.Template, error) {
	return m.templates, nil
}

func (m *mockScaffolder) GetTemplate(_ context.Context, name string) (domain.Template, error) {
	for _, t := range m.templates {
		if t.Name == name {
			return t, nil
		}
	}
	return domain.Template{}, &templateNotFoundError{name: name}
}

func (m *mockScaffolder) Execute(_ context.Context, _, _ string, _ map[string]string, _ bool) error {
	return m.executeErr
}

func (m *mockScaffolder) Lint(_ context.Context, _ string, _ string) []domain.LintError {
	return m.lintErrors
}

type templateNotFoundError struct{ name string }

func (e *templateNotFoundError) Error() string {
	return "template not found: " + e.name
}

// connectTestServer wires a joist MCP server to an in-memory client session
// and returns the session ready for tool calls.
func connectTestServer(t *testing.T, scaffolder *mockScaffolder) *sdkmcp.ClientSession {
	t.Helper()
	ctx := context.Background()

	server := joistmlcp.NewServer(scaffolder)
	cTransport, sTransport := sdkmcp.NewInMemoryTransports()

	ss, err := server.Connect(ctx, sTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ss.Close() })

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	cs, err := client.Connect(ctx, cTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })

	return cs
}

// callTool is a small helper that calls a named tool and returns the result text and isError flag.
func callTool(t *testing.T, cs *sdkmcp.ClientSession, name string, args any) (text string, isError bool) {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &sdkmcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	require.NoError(t, err)
	require.NotEmpty(t, res.Content, "expected at least one content item")
	tc, ok := res.Content[0].(*sdkmcp.TextContent)
	require.True(t, ok, "expected TextContent")
	return tc.Text, res.IsError
}

// --- list ---

func TestMCP_List_NoTemplates(t *testing.T) {
	cs := connectTestServer(t, &mockScaffolder{})
	text, isError := callTool(t, cs, "list", nil)
	assert.False(t, isError)
	assert.Contains(t, text, "No templates found")
}

func TestMCP_List_WithTemplates(t *testing.T) {
	scaffolder := &mockScaffolder{
		templates: []domain.Template{
			{Name: "service", Description: "Creates a new service"},
			{Name: "handler", Description: "Adds an HTTP handler"},
		},
	}
	cs := connectTestServer(t, scaffolder)
	text, isError := callTool(t, cs, "list", nil)
	assert.False(t, isError)
	assert.Contains(t, text, "service")
	assert.Contains(t, text, "Creates a new service")
	assert.Contains(t, text, "handler")
	assert.Contains(t, text, "Adds an HTTP handler")
}

// --- doc ---

func TestMCP_Doc_TemplateOverview(t *testing.T) {
	scaffolder := &mockScaffolder{
		templates: []domain.Template{
			{
				Name:        "service",
				Description: "Service scaffolding",
				Commands: []domain.TemplateCommand{
					{Command: "create", Description: "Create a new service"},
					{Command: "add-handler", Description: "Add an HTTP handler"},
				},
			},
		},
	}
	cs := connectTestServer(t, scaffolder)
	text, isError := callTool(t, cs, "doc", map[string]any{"template": "service"})
	assert.False(t, isError)
	assert.Contains(t, text, "service")
	assert.Contains(t, text, "create")
	assert.Contains(t, text, "Create a new service")
	assert.Contains(t, text, "add-handler")
}

func TestMCP_Doc_CommandDetail(t *testing.T) {
	scaffolder := &mockScaffolder{
		templates: []domain.Template{
			{
				Name: "service",
				Commands: []domain.TemplateCommand{
					{
						Command:     "create",
						Description: "Create a new service",
						Variables: []domain.TemplateVariable{
							{Key: "Name", Description: "service name"},
							{Key: "Port", Description: "HTTP port"},
						},
						PostCommands: []string{"add-handler"},
					},
				},
			},
		},
	}
	cs := connectTestServer(t, scaffolder)
	text, isError := callTool(t, cs, "doc", map[string]any{
		"template": "service",
		"command":  "create",
	})
	assert.False(t, isError)
	assert.Contains(t, text, "Create a new service")
	assert.Contains(t, text, "Name")
	assert.Contains(t, text, "service name")
	assert.Contains(t, text, "Port")
	assert.Contains(t, text, "add-handler")
}

func TestMCP_Doc_UnknownTemplate(t *testing.T) {
	cs := connectTestServer(t, &mockScaffolder{})
	_, isError := callTool(t, cs, "doc", map[string]any{"template": "nonexistent"})
	assert.True(t, isError)
}

func TestMCP_Doc_UnknownCommand(t *testing.T) {
	scaffolder := &mockScaffolder{
		templates: []domain.Template{
			{Name: "service", Commands: []domain.TemplateCommand{{Command: "create"}}},
		},
	}
	cs := connectTestServer(t, scaffolder)
	text, isError := callTool(t, cs, "doc", map[string]any{
		"template": "service",
		"command":  "nonexistent",
	})
	assert.True(t, isError)
	assert.Contains(t, text, "nonexistent")
}

func TestMCP_Doc_ShellCommandsShown(t *testing.T) {
	scaffolder := &mockScaffolder{
		templates: []domain.Template{
			{
				Name: "service",
				Commands: []domain.TemplateCommand{
					{Command: "create", Description: "Create"},
				},
				ShellCommands: []domain.ShellCommand{
					{Command: "go mod tidy", Mode: "all"},
				},
			},
		},
	}
	cs := connectTestServer(t, scaffolder)
	text, isError := callTool(t, cs, "doc", map[string]any{"template": "service"})
	assert.False(t, isError)
	assert.Contains(t, text, "go mod tidy")
}

// --- execute ---

func TestMCP_Execute_Success(t *testing.T) {
	scaffolder := &mockScaffolder{
		templates: []domain.Template{
			{Name: "service", Commands: []domain.TemplateCommand{{Command: "create"}}},
		},
	}
	cs := connectTestServer(t, scaffolder)
	_, isError := callTool(t, cs, "execute", map[string]any{
		"template": "service",
		"command":  "create",
		"params":   map[string]string{"Name": "catalog"},
	})
	assert.False(t, isError)
}

func TestMCP_Execute_Error(t *testing.T) {
	scaffolder := &mockScaffolder{
		templates: []domain.Template{
			{Name: "service", Commands: []domain.TemplateCommand{{Command: "create"}}},
		},
		executeErr: &templateNotFoundError{name: "service"},
	}
	cs := connectTestServer(t, scaffolder)
	_, isError := callTool(t, cs, "execute", map[string]any{
		"template": "service",
		"command":  "create",
	})
	assert.True(t, isError)
}

func TestMCP_Execute_NilParamsBecomesEmptyMap(t *testing.T) {
	// Omitting params must not cause a panic — an empty map is passed to Execute.
	scaffolder := &mockScaffolder{
		templates: []domain.Template{
			{Name: "service", Commands: []domain.TemplateCommand{{Command: "create"}}},
		},
	}
	cs := connectTestServer(t, scaffolder)
	_, isError := callTool(t, cs, "execute", map[string]any{
		"template": "service",
		"command":  "create",
	})
	assert.False(t, isError)
}

// --- lint ---

func TestMCP_Lint_NoIssues(t *testing.T) {
	scaffolder := &mockScaffolder{
		templates: []domain.Template{{Name: "service"}},
	}
	cs := connectTestServer(t, scaffolder)
	text, isError := callTool(t, cs, "lint", map[string]any{"template": "service"})
	assert.False(t, isError)
	assert.Contains(t, text, "OK")
}

func TestMCP_Lint_WithErrors(t *testing.T) {
	scaffolder := &mockScaffolder{
		templates: []domain.Template{{Name: "service"}},
		lintErrors: []domain.LintError{
			{Command: "create", Field: "variables", Message: `variable "name" must start with a capital letter`},
			{Field: "shell_commands", Message: `shell_command[0] has an empty command`},
		},
	}
	cs := connectTestServer(t, scaffolder)
	text, isError := callTool(t, cs, "lint", map[string]any{"template": "service"})
	assert.True(t, isError)
	assert.Contains(t, text, "LINT ERRORS")
	assert.Contains(t, text, "capital letter")
	assert.Contains(t, text, "2 issue(s) found")
}

func TestMCP_Lint_All_NoTemplates(t *testing.T) {
	cs := connectTestServer(t, &mockScaffolder{})
	text, isError := callTool(t, cs, "lint", map[string]any{"all": true})
	assert.False(t, isError)
	assert.Contains(t, text, "No templates found")
}

func TestMCP_Lint_All_AllClean(t *testing.T) {
	scaffolder := &mockScaffolder{
		templates: []domain.Template{{Name: "service"}, {Name: "handler"}},
	}
	cs := connectTestServer(t, scaffolder)
	text, isError := callTool(t, cs, "lint", map[string]any{"all": true})
	assert.False(t, isError)
	assert.Contains(t, text, "OK: service")
	assert.Contains(t, text, "OK: handler")
}

func TestMCP_Lint_All_WithErrors(t *testing.T) {
	scaffolder := &mockScaffolder{
		templates:  []domain.Template{{Name: "service"}, {Name: "handler"}},
		lintErrors: []domain.LintError{{Command: "create", Field: "variables", Message: "bad var"}},
	}
	cs := connectTestServer(t, scaffolder)
	text, isError := callTool(t, cs, "lint", map[string]any{"all": true})
	assert.True(t, isError)
	assert.Contains(t, text, "LINT ERRORS in service")
	assert.Contains(t, text, "LINT ERRORS in handler")
	assert.Contains(t, text, "issue(s) found across 2 template(s)")
}
