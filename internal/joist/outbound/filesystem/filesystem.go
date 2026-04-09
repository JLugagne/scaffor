package filesystem

import (
	"context"
	"os"
	"strings"

	"golang.org/x/tools/imports"
)

// FileSystem is an adapter that interacts with the real file system.
type FileSystem struct{}

// NewFileSystem creates a new FileSystem adapter.
func NewFileSystem() *FileSystem {
	return &FileSystem{}
}

// ReadFile reads the content of the file at path.
func (f *FileSystem) ReadFile(ctx context.Context, path string) ([]byte, error) {
	return os.ReadFile(path)
}

// WriteFile writes content to the file at path.
func (f *FileSystem) WriteFile(ctx context.Context, path string, content []byte) error {
	if strings.HasSuffix(path, ".go") {
		formatted, err := imports.Process(path, content, nil)
		if err == nil {
			content = formatted
		}
	}
	return os.WriteFile(path, content, 0644)
}

// MkdirAll creates a directory and all necessary parents.
func (f *FileSystem) MkdirAll(ctx context.Context, path string) error {
	return os.MkdirAll(path, 0755)
}
