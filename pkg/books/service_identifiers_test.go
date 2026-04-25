package books

import (
	"context"
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestService_BulkCreateFileIdentifiers_DedupesByType(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	_, book := setupTestLibraryAndBook(t, db)
	file := setupTestFile(t, db, book, "epub", createTestEPUBFile(t))

	// Same type appears twice; last-wins.
	identifiers := []*models.FileIdentifier{
		{FileID: file.ID, Type: "asin", Value: "B01ABC1234", Source: models.PluginDataSource("shisho", "audnexus")},
		{FileID: file.ID, Type: "isbn_13", Value: "9780316769488", Source: models.DataSourceEPUBMetadata},
		{FileID: file.ID, Type: "asin", Value: "B02DEF5678", Source: models.DataSourceManual},
	}

	err := svc.BulkCreateFileIdentifiers(ctx, identifiers)
	require.NoError(t, err)

	var stored []*models.FileIdentifier
	err = db.NewSelect().Model(&stored).Where("file_id = ?", file.ID).Order("type ASC").Scan(ctx)
	require.NoError(t, err)

	require.Len(t, stored, 2)
	asin := stored[0]
	isbn := stored[1]
	assert.Equal(t, "asin", asin.Type)
	assert.Equal(t, "B02DEF5678", asin.Value, "expected last-wins for the asin type")
	assert.Equal(t, models.DataSourceManual, asin.Source)
	assert.Equal(t, "isbn_13", isbn.Type)
	assert.Equal(t, "9780316769488", isbn.Value)
}

func TestService_BulkCreateFileIdentifiers_NoDuplicates(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	_, book := setupTestLibraryAndBook(t, db)
	file := setupTestFile(t, db, book, "epub", createTestEPUBFile(t))

	identifiers := []*models.FileIdentifier{
		{FileID: file.ID, Type: "asin", Value: "B01ABC1234", Source: models.DataSourceManual},
		{FileID: file.ID, Type: "isbn_13", Value: "9780316769488", Source: models.DataSourceEPUBMetadata},
	}

	err := svc.BulkCreateFileIdentifiers(ctx, identifiers)
	require.NoError(t, err)

	var stored []*models.FileIdentifier
	err = db.NewSelect().Model(&stored).Where("file_id = ?", file.ID).Order("type ASC").Scan(ctx)
	require.NoError(t, err)
	require.Len(t, stored, 2)
}

func TestService_BulkCreateFileIdentifiers_EmptySliceIsNoop(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	err := svc.BulkCreateFileIdentifiers(ctx, nil)
	require.NoError(t, err)

	err = svc.BulkCreateFileIdentifiers(ctx, []*models.FileIdentifier{})
	require.NoError(t, err)
}
