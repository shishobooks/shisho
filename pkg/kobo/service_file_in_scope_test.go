package kobo

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/shishobooks/shisho/pkg/apikeys"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// loadScopeUser loads the fixture's key owner the way the API key middleware
// does.
func (f *koboScopeFixture) loadScopeUser(t *testing.T) *models.User {
	t.Helper()
	user, err := apikeys.NewService(f.db).AuthenticateOwner(context.Background(), f.key)
	require.NoError(t, err)
	return user
}

func TestFileInScope(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	libraryScope := func(id int) *SyncScope { return &SyncScope{Type: "library", LibraryID: &id} }
	listScope := func(id int) *SyncScope { return &SyncScope{Type: "list", ListID: &id} }

	tests := []struct {
		name string
		// setup narrows the fixture and returns the scope and file to check.
		setup func(t *testing.T, f *koboScopeFixture) (*SyncScope, int)
		want  bool
	}{
		{"all scope, accessible library", func(_ *testing.T, f *koboScopeFixture) (*SyncScope, int) {
			return &SyncScope{Type: "all"}, f.fileB.ID
		}, true},
		{"all scope, inaccessible library", func(t *testing.T, f *koboScopeFixture) (*SyncScope, int) {
			f.restrictToLibrary(t, f.libA.ID)
			return &SyncScope{Type: "all"}, f.fileB.ID
		}, false},
		{"library scope, file in scope library", func(_ *testing.T, f *koboScopeFixture) (*SyncScope, int) {
			return libraryScope(f.libA.ID), f.fileA.ID
		}, true},
		{"library scope, file in another accessible library", func(_ *testing.T, f *koboScopeFixture) (*SyncScope, int) {
			return libraryScope(f.libA.ID), f.fileB.ID
		}, false},
		{"library scope, inaccessible scope library", func(t *testing.T, f *koboScopeFixture) (*SyncScope, int) {
			f.restrictToLibrary(t, f.libA.ID)
			return libraryScope(f.libB.ID), f.fileB.ID
		}, false},
		{"library scope without id", func(_ *testing.T, f *koboScopeFixture) (*SyncScope, int) {
			return &SyncScope{Type: "library"}, f.fileA.ID
		}, false},
		{"list scope, book on list", func(t *testing.T, f *koboScopeFixture) (*SyncScope, int) {
			return listScope(f.insertList(t, f.bookA.ID).ID), f.fileA.ID
		}, true},
		{"list scope, book not on list", func(t *testing.T, f *koboScopeFixture) (*SyncScope, int) {
			return listScope(f.insertList(t, f.bookA.ID).ID), f.fileB.ID
		}, false},
		{"list scope, book on list in inaccessible library", func(t *testing.T, f *koboScopeFixture) (*SyncScope, int) {
			list := f.insertList(t, f.bookA.ID, f.bookB.ID)
			f.restrictToLibrary(t, f.libA.ID)
			return listScope(list.ID), f.fileB.ID
		}, false},
		{"list scope, list the user cannot view", func(t *testing.T, f *koboScopeFixture) (*SyncScope, int) {
			list := f.insertList(t, f.bookA.ID)
			other := &models.User{Username: "other", PasswordHash: "x", RoleID: f.user.RoleID, IsActive: true}
			_, err := f.db.NewInsert().Model(other).Exec(ctx)
			require.NoError(t, err)
			_, err = f.db.NewUpdate().Model((*models.List)(nil)).Set("user_id = ?", other.ID).Where("id = ?", list.ID).Exec(ctx)
			require.NoError(t, err)
			return listScope(list.ID), f.fileA.ID
		}, false},
		{"supplement file", func(t *testing.T, f *koboScopeFixture) (*SyncScope, int) {
			supplement := insertScopeFile(ctx, t, f.db, f.bookA, filepath.Dir(f.fileA.Filepath), "guide", models.FileTypeEPUB, models.FileRoleSupplement)
			return &SyncScope{Type: "all"}, supplement.ID
		}, false},
		{"M4B file", func(t *testing.T, f *koboScopeFixture) (*SyncScope, int) {
			m4b := insertScopeFile(ctx, t, f.db, f.bookA, filepath.Dir(f.fileA.Filepath), "audio", models.FileTypeM4B, models.FileRoleMain)
			return &SyncScope{Type: "all"}, m4b.ID
		}, false},
		{"unknown file", func(_ *testing.T, _ *koboScopeFixture) (*SyncScope, int) {
			return &SyncScope{Type: "all"}, 999999
		}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newKoboScopeFixture(t)
			scope, fileID := tt.setup(t, f)

			got, err := NewService(f.db).FileInScope(ctx, f.loadScopeUser(t), scope, fileID)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// A nil user fails closed with an error rather than matching every file or
// panicking.
func TestFileInScope_NilUserFailsClosed(t *testing.T) {
	t.Parallel()
	f := newKoboScopeFixture(t)

	got, err := NewService(f.db).FileInScope(context.Background(), nil, &SyncScope{Type: "all"}, f.fileA.ID)
	require.Error(t, err)
	assert.False(t, got)
}
