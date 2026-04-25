package commands

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/JLugagne/scaffor/internal/scaffor/config"
	"github.com/spf13/cobra"
)

// ResolverFactory returns the active template resolver. Mirrors the lazy
// pattern used by ScaffolderFactory so the install command sees the resolver
// after persistent flags are parsed.
type ResolverFactory func() (*config.Resolver, error)

// NewInstallCommand creates the `scaffor install` command, which copies one or
// more templates from their resolved source (typically a global directory
// declared in template_sources) into the current project's
// .scaffor-templates/ directory.
func NewInstallCommand(factory ResolverFactory) *cobra.Command {
	var dest string
	var force bool

	cmd := &cobra.Command{
		Use:   "install <template> [<template>...]",
		Short: "Copy a global template into the local .scaffor-templates/",
		Long: `Copy one or more templates from their resolved source directory into
the local .scaffor-templates/ directory (creating it if needed). Useful for
vendoring a global template into a project so it can be customized locally
without affecting the original.

By default the destination is ./.scaffor-templates/<template>. The command
refuses to overwrite an existing template directory unless --force is passed.

When --force is used to refresh an already-installed template, the command
skips the local copy when resolving the source so it picks up the next
matching source (typically the original global one).`,
		Example: `  # Vendor the go-hexagonal global template into ./.scaffor-templates/
  scaffor install go-hexagonal

  # Install several templates at once
  scaffor install go-hexagonal service

  # Refresh an already-installed local copy from its global source
  scaffor install go-hexagonal --force

  # Install into a custom destination directory
  scaffor install go-hexagonal --dest ./vendor/templates`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resolver, err := factory()
			if err != nil {
				return err
			}

			destDir, err := resolveInstallDest(dest)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(destDir, 0o755); err != nil {
				return fmt.Errorf("creating %s: %w", destDir, err)
			}

			for _, name := range args {
				srcRoot, err := findInstallSource(resolver, name, destDir)
				if err != nil {
					return err
				}
				srcDir := filepath.Join(srcRoot, name)
				dstDir := filepath.Join(destDir, name)

				if sameDir(srcDir, dstDir) {
					return fmt.Errorf("template %q is already at %s; nothing to install", name, dstDir)
				}

				if _, err := os.Stat(dstDir); err == nil {
					if !force {
						return fmt.Errorf("%s already exists (use --force to overwrite)", dstDir)
					}
					if err := os.RemoveAll(dstDir); err != nil {
						return fmt.Errorf("removing existing %s: %w", dstDir, err)
					}
				}

				if err := copyDir(srcDir, dstDir); err != nil {
					return fmt.Errorf("copying %s → %s: %w", srcDir, dstDir, err)
				}
				fmt.Printf("Installed %s\n  from %s\n  to   %s\n", name, srcDir, dstDir)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&dest, "dest", "", "Destination templates directory (default: ./.scaffor-templates)")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite an existing template directory at the destination")
	return cmd
}

// resolveInstallDest returns the absolute destination templates directory.
// When dest is empty it defaults to <cwd>/.scaffor-templates so the install
// always lands in the current project rather than a parent .scaffor-templates
// found by walking up.
func resolveInstallDest(dest string) (string, error) {
	if dest == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("getting working directory: %w", err)
		}
		return filepath.Join(cwd, config.DefaultLocalTemplatesDir), nil
	}
	abs, err := filepath.Abs(dest)
	if err != nil {
		return "", fmt.Errorf("resolving %s: %w", dest, err)
	}
	return abs, nil
}

func sameDir(a, b string) bool {
	aa, err := filepath.Abs(a)
	if err != nil {
		return false
	}
	bb, err := filepath.Abs(b)
	if err != nil {
		return false
	}
	return aa == bb
}

// copyDir recursively copies src into dst, preserving file mode bits. dst is
// created if it does not exist. Symlinks are copied as-is (not followed).
func copyDir(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", src)
	}

	return filepath.Walk(src, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		switch {
		case fi.Mode()&os.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			return os.Symlink(link, target)
		case fi.IsDir():
			return os.MkdirAll(target, fi.Mode().Perm())
		default:
			return copyFile(path, target, fi.Mode().Perm())
		}
	})
}

func copyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// findInstallSource returns the path of the source directory that should be
// used as the install origin for name. When the resolver's first match is the
// destination dir itself (i.e. the template is already vendored locally), the
// scan continues to the next source so --force can refresh from the original
// global source instead of copying the local copy onto itself.
func findInstallSource(resolver *config.Resolver, name, destDir string) (string, error) {
	destAbs, _ := filepath.Abs(destDir)
	for _, src := range resolver.Sources() {
		srcAbs, _ := filepath.Abs(src.Path)
		if srcAbs == destAbs {
			continue
		}
		for _, t := range src.Templates {
			if t == name {
				return src.Path, nil
			}
		}
	}
	// Fall back to Resolve so users get the same error formatting as the rest
	// of the CLI when no other source matches.
	return resolver.Resolve(name)
}
