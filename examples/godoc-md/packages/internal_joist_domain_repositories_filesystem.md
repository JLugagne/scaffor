# Package: internal/joist/domain/repositories/filesystem

## Overview
Defines the abstraction for file system operations. This interface allows the domain layer to remain independent of concrete file system implementations.

## Types

### FileSystem
```go
type FileSystem interface {
	ReadFile(ctx context.Context, path string) ([]byte, error)
	WriteFile(ctx context.Context, path string, data []byte) error
	MkdirAll(ctx context.Context, path string) error
}
```

Defines the contract for all file system operations used by joist.

#### Methods

**ReadFile(ctx context.Context, path string) ([]byte, error)**

Reads the entire contents of a file at the given path. Returns an error if the file does not exist or cannot be read.

**WriteFile(ctx context.Context, path string, data []byte) error**

Writes data to a file at the given path. Creates the file if it doesn't exist, or truncates and overwrites if it does. Returns an error if the write fails.

**MkdirAll(ctx context.Context, path string) error**

Creates all necessary parent directories for the given path, similar to `mkdir -p`. Returns an error if directory creation fails.

## Implementations

- `internal/joist/outbound/filesystem.FileSystem` — Real file system implementation using Go's `os` package

## Usage

```go
var fs FileSystem = filesystem.NewFileSystem()

// Read a file
data, err := fs.ReadFile(ctx, "path/to/file.txt")

// Write a file
err := fs.WriteFile(ctx, "path/to/output.txt", []byte("content"))

// Create directories
err := fs.MkdirAll(ctx, "path/to/deeply/nested/dir")
```
