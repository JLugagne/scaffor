package filesystem_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/JLugagne/scaffor/internal/scaffor/outbound/filesystem"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileSystem_ReadFile(t *testing.T) {
	ctx := context.Background()
	fs := filesystem.NewFileSystem()

	t.Run("reads existing file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "hello.txt")
		require.NoError(t, os.WriteFile(path, []byte("hello world"), 0644))

		got, err := fs.ReadFile(ctx, path)
		require.NoError(t, err)
		assert.Equal(t, []byte("hello world"), got)
	})

	t.Run("returns error for missing file", func(t *testing.T) {
		_, err := fs.ReadFile(ctx, "/nonexistent/path/file.txt")
		require.Error(t, err)
		assert.True(t, os.IsNotExist(err))
	})
}

func TestFileSystem_WriteFile(t *testing.T) {
	ctx := context.Background()
	fs := filesystem.NewFileSystem()

	t.Run("writes plain file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "out.txt")

		require.NoError(t, fs.WriteFile(ctx, path, []byte("content")))

		got, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, []byte("content"), got)
	})

	t.Run("writes and formats .go file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "main.go")
		src := []byte("package main\nfunc main(){}")

		require.NoError(t, fs.WriteFile(ctx, path, src))

		got, err := os.ReadFile(path)
		require.NoError(t, err)
		// goimports adds a newline at end; just verify it's valid Go
		assert.Contains(t, string(got), "package main")
		assert.Contains(t, string(got), "func main()")
	})

	t.Run("returns error for unwritable path", func(t *testing.T) {
		err := fs.WriteFile(ctx, "/nonexistent/dir/file.txt", []byte("x"))
		require.Error(t, err)
	})
}

func TestFileSystem_MkdirAll(t *testing.T) {
	ctx := context.Background()
	fs := filesystem.NewFileSystem()

	t.Run("creates nested directories", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "a", "b", "c")

		require.NoError(t, fs.MkdirAll(ctx, target))

		info, err := os.Stat(target)
		require.NoError(t, err)
		assert.True(t, info.IsDir())
	})

	t.Run("idempotent on existing directory", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, fs.MkdirAll(ctx, dir))
	})
}
