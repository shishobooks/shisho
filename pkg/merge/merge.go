// Package merge holds the input checks that every resource merge (People,
// Series, Genres, Tags, Publishers) runs before it writes anything.
package merge

import (
	"fmt"

	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
)

// Side is one resource in a merge: its id and the Library it belongs to.
type Side struct {
	ID        int
	LibraryID int
}

// CheckPreconditions validates merging source into target on behalf of user.
// Handlers retrieve both sides first, so a missing target or source is a 404
// from the retrieve, and then call this before the merge:
//
//   - 401 when user is nil, so a handler reached without an authenticated
//     user fails closed instead of skipping the access check.
//   - 403 when the user cannot access the target's or the source's Library.
//     The merge deletes the source, so both sides need access.
//   - 422 when source and target are the same resource. A self-merge would
//     delete the target and every link to it.
//   - 422 when source and target belong to different Libraries.
//
// kind is the lowercase singular resource name used in the messages.
func CheckPreconditions(user *models.User, kind string, target, source Side) error {
	if user == nil {
		return errcodes.AuthenticationRequired()
	}
	if !user.HasLibraryAccess(target.LibraryID) || !user.HasLibraryAccess(source.LibraryID) {
		return errcodes.LibraryAccessDenied()
	}
	if target.ID == source.ID {
		return SelfMergeError(kind)
	}
	if target.LibraryID != source.LibraryID {
		return errcodes.ValidationError(fmt.Sprintf("A %s can only be merged into a %s in the same library", kind, kind))
	}
	return nil
}

// SelfMergeError is the validation error for merging a resource into itself.
// Merge services return it as a backstop for callers that skip the handler.
func SelfMergeError(kind string) error {
	return errcodes.ValidationError(fmt.Sprintf("A %s cannot be merged into itself", kind))
}
