package sortspec

import (
	"context"
	"database/sql"
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
)

// fakeReader is a test double for LibrarySettingsReader.
type fakeReader struct {
	settings *models.UserLibrarySettings
	err      error
}

func (f *fakeReader) GetLibrarySettings(_ context.Context, _ int, _ int) (*models.UserLibrarySettings, error) {
	return f.settings, f.err
}

func TestResolveForLibrary_ExplicitWins(t *testing.T) {
	t.Parallel()

	storedSpec := "title:asc"
	reader := &fakeReader{
		settings: &models.UserLibrarySettings{SortSpec: &storedSpec},
	}

	explicit := []SortLevel{{Field: FieldDateAdded, Direction: DirDesc}}
	got := ResolveForLibrary(context.Background(), reader, 1, 2, explicit)

	assert.Equal(t, explicit, got)
}

func TestResolveForLibrary_StoredUsedWhenNoExplicit(t *testing.T) {
	t.Parallel()

	storedSpec := "author:asc,series:asc"
	reader := &fakeReader{
		settings: &models.UserLibrarySettings{SortSpec: &storedSpec},
	}

	got := ResolveForLibrary(context.Background(), reader, 1, 2, nil)

	assert.Equal(t, []SortLevel{
		{Field: FieldAuthor, Direction: DirAsc},
		{Field: FieldSeries, Direction: DirAsc},
	}, got)
}

func TestResolveForLibrary_ReturnsNilWhenNoRow(t *testing.T) {
	t.Parallel()

	// sql.ErrNoRows or nil settings both mean "no preference saved".
	reader := &fakeReader{settings: nil, err: sql.ErrNoRows}
	got := ResolveForLibrary(context.Background(), reader, 1, 2, nil)

	assert.Nil(t, got)
}

func TestResolveForLibrary_ReturnsNilWhenSortSpecNull(t *testing.T) {
	t.Parallel()

	reader := &fakeReader{
		settings: &models.UserLibrarySettings{SortSpec: nil},
	}
	got := ResolveForLibrary(context.Background(), reader, 1, 2, nil)

	assert.Nil(t, got)
}

func TestResolveForLibrary_InvalidStoredSpecFallsThrough(t *testing.T) {
	t.Parallel()

	// A stored spec that fails to parse (e.g. whitelist drift between
	// releases) should not crash — return nil and let the caller use
	// its hard-coded default.
	bad := "garbage_field:asc"
	reader := &fakeReader{
		settings: &models.UserLibrarySettings{SortSpec: &bad},
	}
	got := ResolveForLibrary(context.Background(), reader, 1, 2, nil)

	assert.Nil(t, got)
}

func TestResolveForLibrary_ReaderErrorFallsThrough(t *testing.T) {
	t.Parallel()

	reader := &fakeReader{err: assert.AnError}
	got := ResolveForLibrary(context.Background(), reader, 1, 2, nil)

	// Unexpected DB errors are swallowed; sort is best-effort.
	assert.Nil(t, got)
}
