package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/opensdd/osdd-api/clients/go/osdd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPersistMaterializedResult(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		res := osdd.MaterializedResult_builder{
			Entries: []*osdd.MaterializedResult_Entry{
				osdd.MaterializedResult_Entry_builder{
					File: osdd.FullFileContent_builder{Path: "hello.txt", Content: "Hi"}.Build(),
				}.Build(),
			},
		}.Build()

		err := PersistMaterializedResult(context.Background(), root, res)
		require.NoError(t, err)

		b, err := os.ReadFile(filepath.Join(root, "hello.txt"))
		require.NoError(t, err)
		assert.Equal(t, "Hi", string(b))
	})

	t.Run("nested_dirs", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		path := filepath.Join("a", "b", "c.txt")
		res := osdd.MaterializedResult_builder{
			Entries: []*osdd.MaterializedResult_Entry{
				osdd.MaterializedResult_Entry_builder{
					File: osdd.FullFileContent_builder{Path: path, Content: "nested"}.Build(),
				}.Build(),
			},
		}.Build()

		require.NoError(t, PersistMaterializedResult(context.Background(), root, res))

		b, err := os.ReadFile(filepath.Join(root, path))
		require.NoError(t, err)
		assert.Equal(t, "nested", string(b))
	})

	t.Run("overwrite", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		p := "file.txt"
		res1 := osdd.MaterializedResult_builder{Entries: []*osdd.MaterializedResult_Entry{
			osdd.MaterializedResult_Entry_builder{File: osdd.FullFileContent_builder{Path: p, Content: "v1"}.Build()}.Build(),
		}}.Build()
		res2 := osdd.MaterializedResult_builder{Entries: []*osdd.MaterializedResult_Entry{
			osdd.MaterializedResult_Entry_builder{File: osdd.FullFileContent_builder{Path: p, Content: "v2"}.Build()}.Build(),
		}}.Build()

		require.NoError(t, PersistMaterializedResult(context.Background(), root, res1))
		require.NoError(t, PersistMaterializedResult(context.Background(), root, res2))

		b, err := os.ReadFile(filepath.Join(root, p))
		require.NoError(t, err)
		assert.Equal(t, "v2", string(b))
	})

	t.Run("path_traversal_blocked", func(t *testing.T) {
		root := t.TempDir()
		res := osdd.MaterializedResult_builder{Entries: []*osdd.MaterializedResult_Entry{
			osdd.MaterializedResult_Entry_builder{File: osdd.FullFileContent_builder{Path: filepath.Join("..", "x.txt"), Content: "oops"}.Build()}.Build(),
		}}.Build()

		err := PersistMaterializedResult(context.Background(), root, res)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "escapes root")

		parentFile := filepath.Join(filepath.Dir(root), "x.txt")
		_, statErr := os.Stat(parentFile)
		assert.True(t, os.IsNotExist(statErr), "unexpectedly found parent file outside root")
	})

	t.Run("nil_result", func(t *testing.T) {
		root := t.TempDir()
		err := PersistMaterializedResult(context.Background(), root, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "materialized result cannot be nil")
	})

	t.Run("empty_entries", func(t *testing.T) {
		root := t.TempDir()
		res := osdd.MaterializedResult_builder{}.Build()
		// should not error
		require.NoError(t, PersistMaterializedResult(context.Background(), root, res))
	})
}
