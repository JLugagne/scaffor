// Package config loads scaffor's global configuration from
// $XDG_CONFIG_HOME/scaffor/config.yml (or ~/.config/scaffor/config.yml)
// and expands its declared template source paths.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultLocalTemplatesDir is the fallback source used when no config file
// exists and no --templates-dir override is provided.
const DefaultLocalTemplatesDir = ".scaffor-templates"

// Source is one entry in the config's template_sources list.
type Source struct {
	Path        string `yaml:"path"`
	Description string `yaml:"description,omitempty"`
}

// Config is the parsed global configuration.
type Config struct {
	// Path is the filesystem path of the config file that was loaded.
	// Empty when the config was not loaded from a file.
	Path string `yaml:"-"`

	// Loaded is true when a config file was found and parsed.
	Loaded bool `yaml:"-"`

	// TemplateSources is the declared list of template source directories.
	TemplateSources []Source `yaml:"template_sources"`
}

// ResolvePath returns the expected location of the config file, honoring
// XDG_CONFIG_HOME if set, otherwise falling back to $HOME/.config.
func ResolvePath() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "scaffor", "config.yml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home directory: %w", err)
	}
	return filepath.Join(home, ".config", "scaffor", "config.yml"), nil
}

// FindLocalTemplatesDir walks up from cwd until it finds a .scaffor-templates
// directory, returning its absolute path. Returns "" if none is found.
func FindLocalTemplatesDir() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		candidate := filepath.Join(dir, DefaultLocalTemplatesDir)
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// Load reads the config file from its default location. When no file exists,
// it returns a zero-value Config with Loaded=false and no error, letting
// callers fall back to the current directory's .scaffor-templates/.
func Load() (*Config, error) {
	path, err := ResolvePath()
	if err != nil {
		return nil, err
	}
	return LoadFrom(path)
}

// LoadFrom reads a config file from the given path. Same semantics as Load
// but with an explicit path — used in tests and by callers that want to point
// at a non-default location.
func LoadFrom(path string) (*Config, error) {
	cfg := &Config{Path: path}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}
	cfg.Loaded = true
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", path, err)
	}
	return cfg, nil
}

// ResolveSources expands each Source.Path (tilde, $VAR, ${VAR}) and returns
// the list of absolute paths in declaration order. Relative paths are
// rejected with an explicit error; missing environment variables are
// reported as errors.
func (c *Config) ResolveSources() ([]Source, error) {
	var resolved []Source
	for i, src := range c.TemplateSources {
		if strings.TrimSpace(src.Path) == "" {
			return nil, fmt.Errorf("template_sources[%d]: path is empty", i)
		}
		expanded, err := ExpandPath(src.Path)
		if err != nil {
			return nil, fmt.Errorf("template_sources[%d] (%s): %w", i, src.Path, err)
		}
		if !filepath.IsAbs(expanded) {
			return nil, fmt.Errorf("template_sources[%d] (%s): path must be absolute after expansion (got %q)", i, src.Path, expanded)
		}
		resolved = append(resolved, Source{Path: expanded, Description: src.Description})
	}
	return resolved, nil
}

var envVarPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}|\$([A-Za-z_][A-Za-z0-9_]*)`)

// ExpandPath expands ~ and environment variable references in a path.
// Returns an error if a referenced environment variable is not set.
func ExpandPath(path string) (string, error) {
	out := path
	if strings.HasPrefix(out, "~/") || out == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("expanding ~: %w", err)
		}
		if out == "~" {
			out = home
		} else {
			out = filepath.Join(home, out[2:])
		}
	}

	var missing []string
	out = envVarPattern.ReplaceAllStringFunc(out, func(match string) string {
		name := strings.TrimPrefix(match, "$")
		name = strings.TrimPrefix(name, "{")
		name = strings.TrimSuffix(name, "}")
		val, ok := os.LookupEnv(name)
		if !ok {
			missing = append(missing, name)
			return match
		}
		return val
	})
	if len(missing) > 0 {
		return "", fmt.Errorf("environment variable(s) not set: %s", strings.Join(missing, ", "))
	}
	return out, nil
}

// ExampleConfigContents returns the default content written by `scaffor
// init-config`: a commented example with no active sources so scaffor
// behavior is unchanged until the user edits it.
func ExampleConfigContents() string {
	return `# scaffor global configuration
#
# Declare directories that scaffor should scan for templates. When you run
# scaffor list / doc / execute / lint / test, every template found in these
# sources is available by name — no need to copy templates into each project
# or pass --templates-dir.
#
# Path forms supported:
#   - absolute paths: /home/me/templates
#   - tilde-expanded: ~/work/scaffor-templates
#   - environment variables: $SCAFFOR_SHARED/templates or ${SCAFFOR_SHARED}/templates
#
# Relative paths are rejected. The first source that defines a template wins;
# scaffor logs a warning when the same template name appears in more than one
# source.

template_sources: []
# Example:
# template_sources:
#   - path: ~/work/scaffor-templates
#     description: Personal templates
#   - path: ~/work/team-scaffor-templates
#     description: Shared team templates
`
}
