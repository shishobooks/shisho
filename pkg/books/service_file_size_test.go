package books

import (
	"context"
	"testing"

	"github.com/shishobooks/shisho/pkg/appsettings"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An empty file is a real file: its size is 0, not unknown, so it must be
// stored as 0 rather than rejected by the NOT NULL constraint.
func TestCreateFile_StoresZeroFilesize(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()
	_, book := setupTestLibraryAndBook(t, db)
	svc := NewService(db, appsettings.NewService(db))

	file := &models.File{
		LibraryID:     book.LibraryID,
		BookID:        book.ID,
		FileType:      "txt",
		FileRole:      models.FileRoleSupplement,
		Filepath:      "/test/empty.txt",
		FilesizeBytes: 0,
	}
	require.NoError(t, svc.CreateFile(ctx, file))

	retrieved, err := svc.RetrieveFile(ctx, RetrieveFileOptions{ID: &file.ID})
	require.NoError(t, err)
	assert.Equal(t, int64(0), retrieved.FilesizeBytes)
}

func TestUpdateFile_StoresZeroFilesize(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()
	_, book := setupTestLibraryAndBook(t, db)
	svc := NewService(db, appsettings.NewService(db))

	file := &models.File{
		LibraryID:     book.LibraryID,
		BookID:        book.ID,
		FileType:      "txt",
		FileRole:      models.FileRoleSupplement,
		Filepath:      "/test/truncated.txt",
		FilesizeBytes: 1234,
	}
	require.NoError(t, svc.CreateFile(ctx, file))

	file.FilesizeBytes = 0
	require.NoError(t, svc.UpdateFile(ctx, file, UpdateFileOptions{Columns: []string{"filesize_bytes"}}))

	retrieved, err := svc.RetrieveFile(ctx, RetrieveFileOptions{ID: &file.ID})
	require.NoError(t, err)
	assert.Equal(t, int64(0), retrieved.FilesizeBytes)
}
