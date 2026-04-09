package filesystem

import (
	"context"
)

// FileSystem defines the interface for file system operations.
type FileSystem interface {
	ReadFile(ctx context.Context, path string) ([]byte, error)
	WriteFile(ctx context.Context, path string, data []byte) error
	MkdirAll(ctx context.Context, path string) error
}
