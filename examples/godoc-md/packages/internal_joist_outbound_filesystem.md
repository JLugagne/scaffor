# Package: internal/joist/outbound/filesystem

## Overview
Concrete implementation of the `FileSystem` interface. This adapter implements actual file system operations using Go's `os` and `ioutil` packages, fulfilling the contract defined in the domain layer.

## Types

### FileSystem
```go
type FileSystem struct {}
```

Concrete implementation of `domain.repositories.filesystem.FileSystem`. Performs actual file I/O operations on the system.

#### Constructor

**NewFileSystem() \*FileSystem**
```go
func NewFileSystem() *FileSystem
```

Creates a new file system adapter with no state. Ready to use immediately.

#### Methods

**ReadFile(ctx context.Context, path string) ([]byte, error)**
```go
func (f *FileSystem) ReadFile(ctx context.Context, path string) ([]byte, error)
```

Reads the entire contents of a file into memory. Wraps `os.ReadFile()`.

Returns:
- `[]byte`: File contents
- `error`: File not found, permission denied, or other I/O errors

**WriteFile(ctx context.Context, path string, content []byte) error**
```go
func (f *FileSystem) WriteFile(ctx context.Context, path string, content []byte) error
```

Writes data to a file, creating or truncating as needed. Wraps `os.WriteFile()` with mode 0644 (readable by all, writable by owner).

Returns:
- `error`: Permission denied, disk full, or other I/O errors

**MkdirAll(ctx context.Context, path string) error**
```go
func (f *FileSystem) MkdirAll(ctx context.Context, path string) error
```

Creates all necessary parent directories for the given path. Similar to `mkdir -p`. Wraps `os.MkdirAll()`.

Returns:
- `error`: Permission denied or other I/O errors. Does not error if the directory already exists.

## Usage

```go
fs := NewFileSystem()

// Read template file
data, err := fs.ReadFile(ctx, ".joist-templates/my-template/template.go.tpl")
if err != nil {
    return err
}

// Create output directory
err = fs.MkdirAll(ctx, "generated/models")
if err != nil {
    return err
}

// Write generated file
err = fs.WriteFile(ctx, "generated/models/user.go", []byte("package models\n..."))
if err != nil {
    return err
}
```

## Architecture Notes

This adapter:
- Implements the domain's `FileSystem` interface
- Enables testing by allowing mock implementations to be substituted
- Encapsulates all OS-level I/O operations
- Uses context for potential future timeout/cancellation support
- Permissions default to 0644 for files (owner read/write, others read)
