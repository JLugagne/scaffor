package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"sort"
)

// ResolvedSource is a template source directory with its descriptive label
// and the list of template names found inside it.
type ResolvedSource struct {
	Path        string
	Description string
	Templates   []string
}

// Resolver resolves a template name to the directory that contains it by
// scanning an ordered list of source directories. Callers build a Resolver
// once and reuse it across ListTemplates / GetTemplate / Execute / Lint / Test.
type Resolver struct {
	sources []ResolvedSource

	// override, when non-empty, replaces the source list entirely and is
	// used as the single lookup directory. Set by --templates-dir.
	override string

	// collisions maps template name → list of sources after the first that
	// also contain the same template. Populated by scan().
	collisions map[string][]string

	// missing lists sources that don't exist on disk.
	missing []string
}

// NewResolverFromSources builds a resolver from already-expanded sources.
// Sources are scanned once up-front so subsequent lookups are O(1).
// ignoreMissing=true skips sources that don't exist instead of returning an
// error.
func NewResolverFromSources(sources []Source, ignoreMissing bool) (*Resolver, error) {
	r := &Resolver{collisions: map[string][]string{}}
	for _, src := range sources {
		info, err := os.Stat(src.Path)
		if err != nil {
			if os.IsNotExist(err) {
				r.missing = append(r.missing, src.Path)
				if ignoreMissing {
					continue
				}
				continue
			}
			return nil, fmt.Errorf("stat %s: %w", src.Path, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("template source %s is not a directory", src.Path)
		}
		rs := ResolvedSource{Path: src.Path, Description: src.Description}
		entries, err := os.ReadDir(src.Path)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", src.Path, err)
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			rs.Templates = append(rs.Templates, entry.Name())
		}
		sort.Strings(rs.Templates)
		r.sources = append(r.sources, rs)
	}

	if !ignoreMissing && len(r.missing) > 0 {
		return nil, fmt.Errorf("template sources missing: %v", r.missing)
	}

	// Detect collisions: first occurrence wins, later ones are shadowed.
	seen := map[string]string{}
	for _, src := range r.sources {
		for _, name := range src.Templates {
			if first, ok := seen[name]; ok {
				_ = first
				r.collisions[name] = append(r.collisions[name], src.Path)
			} else {
				seen[name] = src.Path
			}
		}
	}

	return r, nil
}

// NewResolverForTest builds a resolver in memory with a fixed source path
// and known template names. Useful for unit tests that work against an
// in-memory filesystem rather than scanning a real directory.
func NewResolverForTest(sourcePath string, templateNames ...string) *Resolver {
	names := append([]string(nil), templateNames...)
	sort.Strings(names)
	return &Resolver{
		collisions: map[string][]string{},
		sources:    []ResolvedSource{{Path: sourcePath, Templates: names}},
	}
}

// NewResolverForDir builds a resolver that looks in a single directory. Used
// when --templates-dir is passed on the CLI, and as the default fallback
// (dir=".scaffor-templates") when no config is loaded.
func NewResolverForDir(dir string) *Resolver {
	r := &Resolver{
		collisions: map[string][]string{},
		override:   dir,
	}
	// The override source is listed so ListAll() still works. Best-effort
	// stat: if the dir doesn't exist, list will return empty.
	if info, err := os.Stat(dir); err == nil && info.IsDir() {
		rs := ResolvedSource{Path: dir}
		if entries, err := os.ReadDir(dir); err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					rs.Templates = append(rs.Templates, entry.Name())
				}
			}
			sort.Strings(rs.Templates)
		}
		r.sources = []ResolvedSource{rs}
	}
	return r
}

// ErrTemplateNotFound is returned by Resolve when no source contains the
// requested template.
var ErrTemplateNotFound = errors.New("template not found")

// Resolve returns the directory that contains <name> (i.e. the parent of its
// manifest.yaml), scanning sources in declaration order. First match wins.
// Resolution is based on the template name appearing as a subdirectory in the
// source — the caller is responsible for reading the manifest and reporting
// any read errors.
func (r *Resolver) Resolve(name string) (string, error) {
	for _, src := range r.sources {
		if slices.Contains(src.Templates, name) {
			return src.Path, nil
		}
	}
	return "", fmt.Errorf("%w: %s (searched %d source(s))", ErrTemplateNotFound, name, len(r.sources))
}

// ListAll returns every (template-name, source-path) pair across all sources.
// Templates are deduplicated: if the same name appears in multiple sources,
// only the first occurrence is returned, matching Resolve() semantics.
// The caller is responsible for reading and validating the manifest; entries
// without a manifest.yaml will surface as load errors downstream.
func (r *Resolver) ListAll() []TemplateRef {
	seen := map[string]bool{}
	var refs []TemplateRef
	for _, src := range r.sources {
		for _, name := range src.Templates {
			if seen[name] {
				continue
			}
			seen[name] = true
			refs = append(refs, TemplateRef{Name: name, Source: src.Path, Description: src.Description})
		}
	}
	return refs
}

// TemplateRef is a (name, source-path) pair returned by ListAll.
type TemplateRef struct {
	Name        string
	Source      string
	Description string
}

// Sources returns the resolved source list with their discovered templates.
// Useful for `scaffor config` introspection.
func (r *Resolver) Sources() []ResolvedSource {
	out := make([]ResolvedSource, len(r.sources))
	copy(out, r.sources)
	return out
}

// Collisions returns the map of shadowed template names → list of additional
// source paths where the template also appears (i.e. not including the
// winning source). Empty when there are no collisions.
func (r *Resolver) Collisions() map[string][]string {
	return r.collisions
}

// Missing returns the list of configured source paths that don't exist on
// disk. Only populated when the resolver was built with ignoreMissing=true.
func (r *Resolver) Missing() []string {
	return r.missing
}

// WriteCollisionWarnings emits one warning line per shadowed template to w.
// Called from commands that resolve templates to surface collisions to
// stderr.
func (r *Resolver) WriteCollisionWarnings(w io.Writer) {
	if len(r.collisions) == 0 {
		return
	}
	// Deterministic order.
	names := make([]string, 0, len(r.collisions))
	for name := range r.collisions {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		// Re-find the winning source for this name.
		var winner string
		for _, src := range r.sources {
			if slices.Contains(src.Templates, name) {
				winner = src.Path
				break
			}
		}
		_, _ = fmt.Fprintf(w, "warning: template %q found in multiple sources; using %s (shadowed: %v)\n",
			name, winner, r.collisions[name])
	}
}
