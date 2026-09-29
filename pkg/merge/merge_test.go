package merge

import (
	"net/http"
	"testing"

	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func userWithAccess(libraryIDs ...int) *models.User {
	user := &models.User{}
	for _, id := range libraryIDs {
		user.LibraryAccess = append(user.LibraryAccess, &models.UserLibraryAccess{LibraryID: &id})
	}
	return user
}

func TestCheckPreconditions(t *testing.T) {
	t.Parallel()
	allAccess := &models.User{LibraryAccess: []*models.UserLibraryAccess{{LibraryID: nil}}}

	cases := []struct {
		name   string
		user   *models.User
		target Side
		source Side
		status int // 0 means no error
	}{
		{"valid", userWithAccess(1), Side{ID: 1, LibraryID: 1}, Side{ID: 2, LibraryID: 1}, 0},
		{"no user is unauthorized", nil, Side{ID: 1, LibraryID: 1}, Side{ID: 2, LibraryID: 1}, http.StatusUnauthorized},
		{"no user self-merge is unauthorized", nil, Side{ID: 1, LibraryID: 1}, Side{ID: 1, LibraryID: 1}, http.StatusUnauthorized},
		{"self", userWithAccess(1), Side{ID: 1, LibraryID: 1}, Side{ID: 1, LibraryID: 1}, http.StatusUnprocessableEntity},
		{"target inaccessible", userWithAccess(2), Side{ID: 1, LibraryID: 1}, Side{ID: 2, LibraryID: 2}, http.StatusForbidden},
		{"source inaccessible", userWithAccess(1), Side{ID: 1, LibraryID: 1}, Side{ID: 2, LibraryID: 2}, http.StatusForbidden},
		{"cross-library", allAccess, Side{ID: 1, LibraryID: 1}, Side{ID: 2, LibraryID: 2}, http.StatusUnprocessableEntity},
		{"self without access is forbidden", userWithAccess(2), Side{ID: 1, LibraryID: 1}, Side{ID: 1, LibraryID: 1}, http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := CheckPreconditions(tc.user, "genre", tc.target, tc.source)
			if tc.status == 0 {
				require.NoError(t, err)
				return
			}
			var codeErr *errcodes.Error
			require.ErrorAs(t, err, &codeErr)
			assert.Equal(t, tc.status, codeErr.HTTPCode)
		})
	}
}
