package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/JLugagne/scaffor/internal/scaffor/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolvePath_UsesXDGWhenSet(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/custom/xdg")
	path, err := config.ResolvePath()
	require.NoError(t, err)
	assert.Equal(t, "/custom/xdg/scaffor/config.yml", path)
}

func TestResolvePath_FallsBackToHome(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "/fake/home")
	path, err := config.ResolvePath()
	require.NoError(t, err)
	assert.Equal(t, "/fake/home/.config/scaffor/config.yml", path)
}

func TestLoadFrom_MissingFileReturnsUnloadedConfig(t *testing.T) {
	cfg, err := config.LoadFrom("/definitely/does/not/exist.yml")
	require.NoError(t, err)
	assert.False(t, cfg.Loaded)
	assert.Empty(t, cfg.TemplateSources)
}

func TestLoadFrom_ParsesValidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	content := `template_sources:
  - path: /abs/one
    description: first
  - path: /abs/two
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	cfg, err := config.LoadFrom(path)
	require.NoError(t, err)
	assert.True(t, cfg.Loaded)
	require.Len(t, cfg.TemplateSources, 2)
	assert.Equal(t, "/abs/one", cfg.TemplateSources[0].Path)
	assert.Equal(t, "first", cfg.TemplateSources[0].Description)
	assert.Equal(t, "/abs/two", cfg.TemplateSources[1].Path)
}

func TestLoadFrom_MalformedYAMLReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	require.NoError(t, os.WriteFile(path, []byte("\t not: [valid"), 0o644))

	_, err := config.LoadFrom(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing config")
}

func TestExpandPath_Tilde(t *testing.T) {
	t.Setenv("HOME", "/h")
	got, err := config.ExpandPath("~/foo/bar")
	require.NoError(t, err)
	assert.Equal(t, "/h/foo/bar", got)
}

func TestExpandPath_TildeOnly(t *testing.T) {
	t.Setenv("HOME", "/h")
	got, err := config.ExpandPath("~")
	require.NoError(t, err)
	assert.Equal(t, "/h", got)
}

func TestExpandPath_EnvVars(t *testing.T) {
	t.Setenv("SCAFFOR_BASE", "/base")
	t.Setenv("SUB", "sub")

	cases := map[string]string{
		"$SCAFFOR_BASE/templates":           "/base/templates",
		"${SCAFFOR_BASE}/templates":         "/base/templates",
		"$SCAFFOR_BASE/$SUB/t":              "/base/sub/t",
		"/no/expansion":                     "/no/expansion",
		"${SCAFFOR_BASE}/x/${SUB}":          "/base/x/sub",
	}
	for in, want := range cases {
		got, err := config.ExpandPath(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, got, "input: %s", in)
	}
}

func TestExpandPath_MissingEnvVarErrors(t *testing.T) {
	// Ensure the variable is unset.
	_ = os.Unsetenv("SCAFFOR_DOES_NOT_EXIST")
	_, err := config.ExpandPath("$SCAFFOR_DOES_NOT_EXIST/templates")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SCAFFOR_DOES_NOT_EXIST")
}

func TestResolveSources_RejectsRelativePaths(t *testing.T) {
	cfg := &config.Config{
		TemplateSources: []config.Source{
			{Path: "relative/path"},
		},
	}
	_, err := cfg.ResolveSources()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "absolute")
}

func TestResolveSources_RejectsEmptyPaths(t *testing.T) {
	cfg := &config.Config{
		TemplateSources: []config.Source{
			{Path: ""},
		},
	}
	_, err := cfg.ResolveSources()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

func TestResolveSources_ExpandsTildeAndEnv(t *testing.T) {
	t.Setenv("HOME", "/h")
	t.Setenv("SCAFFOR_BASE", "/base")
	cfg := &config.Config{
		TemplateSources: []config.Source{
			{Path: "~/one", Description: "first"},
			{Path: "$SCAFFOR_BASE/two"},
		},
	}
	resolved, err := cfg.ResolveSources()
	require.NoError(t, err)
	require.Len(t, resolved, 2)
	assert.Equal(t, "/h/one", resolved[0].Path)
	assert.Equal(t, "first", resolved[0].Description)
	assert.Equal(t, "/base/two", resolved[1].Path)
}

func TestNewResolverFromSources_BuildsFromExistingDirs(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "alpha"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "beta"), 0o755))
	// Stray file at the top level should be ignored.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README"), []byte("x"), 0o644))

	r, err := config.NewResolverFromSources([]config.Source{{Path: dir}}, false)
	require.NoError(t, err)

	gotAlpha, err := r.Resolve("alpha")
	require.NoError(t, err)
	assert.Equal(t, dir, gotAlpha)

	gotBeta, err := r.Resolve("beta")
	require.NoError(t, err)
	assert.Equal(t, dir, gotBeta)

	_, err = r.Resolve("gamma")
	require.ErrorIs(t, err, config.ErrTemplateNotFound)
}

func TestNewResolverFromSources_MissingSourceReturnsError(t *testing.T) {
	_, err := config.NewResolverFromSources([]config.Source{{Path: "/does/not/exist"}}, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing")
}

func TestNewResolverFromSources_IgnoreMissingSkips(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "t1"), 0o755))

	r, err := config.NewResolverFromSources([]config.Source{
		{Path: "/does/not/exist"},
		{Path: dir},
	}, true)
	require.NoError(t, err)

	got, err := r.Resolve("t1")
	require.NoError(t, err)
	assert.Equal(t, dir, got)

	missing := r.Missing()
	assert.Contains(t, missing, "/does/not/exist")
}

func TestNewResolverFromSources_CollisionFirstWins(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dirA, "shared"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dirB, "shared"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dirB, "unique"), 0o755))

	r, err := config.NewResolverFromSources([]config.Source{
		{Path: dirA}, {Path: dirB},
	}, false)
	require.NoError(t, err)

	// Collision: first source wins.
	gotShared, err := r.Resolve("shared")
	require.NoError(t, err)
	assert.Equal(t, dirA, gotShared)

	// Non-colliding template in the second source still resolves.
	gotUnique, err := r.Resolve("unique")
	require.NoError(t, err)
	assert.Equal(t, dirB, gotUnique)

	// Collisions are recorded for reporting.
	col := r.Collisions()
	require.Contains(t, col, "shared")
	assert.Equal(t, []string{dirB}, col["shared"])
}

func TestResolver_ListAll_DedupesAndPreservesOrder(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dirA, "one"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dirA, "two"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dirB, "one"), 0o755)) // shadowed
	require.NoError(t, os.MkdirAll(filepath.Join(dirB, "three"), 0o755))

	r, err := config.NewResolverFromSources([]config.Source{
		{Path: dirA, Description: "a"},
		{Path: dirB, Description: "b"},
	}, false)
	require.NoError(t, err)

	refs := r.ListAll()
	require.Len(t, refs, 3)
	// dirA templates come first (sorted by scan): one, two.
	assert.Equal(t, "one", refs[0].Name)
	assert.Equal(t, dirA, refs[0].Source)
	assert.Equal(t, "two", refs[1].Name)
	// Then dirB's non-colliding entry.
	assert.Equal(t, "three", refs[2].Name)
	assert.Equal(t, dirB, refs[2].Source)
}

func TestNewResolverForDir_EmptyWhenDirMissing(t *testing.T) {
	r := config.NewResolverForDir("/does/not/exist")
	assert.Empty(t, r.ListAll())
}

func TestNewResolverForTest_ResolvesKnownNames(t *testing.T) {
	r := config.NewResolverForTest(".scaffor-templates", "foo", "bar")
	got, err := r.Resolve("foo")
	require.NoError(t, err)
	assert.Equal(t, ".scaffor-templates", got)

	_, err = r.Resolve("missing")
	require.ErrorIs(t, err, config.ErrTemplateNotFound)
}
