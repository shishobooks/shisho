package sortspec

import (
	"context"

	"github.com/shishobooks/shisho/pkg/models"
)

// LibrarySettingsReader is the dependency-injection seam ResolveForLibrary
// uses to read a user's stored sort preference for a library. In
// production it is satisfied by *pkg/settings.Service; tests pass a
// fake.
//
// It exists here (not in pkg/settings) to break what would otherwise
// be a circular import: pkg/settings already imports pkg/sortspec to
// validate sort_spec strings at write time, so pkg/sortspec cannot
// turn around and import pkg/settings to read them back. Defining the
// narrow read contract on this side of the boundary lets the resolver
// stay in pkg/sortspec without dragging in the rest of the settings
// service surface.
//
// Implementations MUST return (nil, nil) — not an error — when no row
// exists for (userID, libraryID); ResolveForLibrary treats that as
// "no stored preference" and falls back to the builtin default.
type LibrarySettingsReader interface {
	GetLibrarySettings(ctx context.Context, userID, libraryID int) (*models.UserLibrarySettings, error)
}

// ResolveForLibrary picks the sort levels to apply for a given caller.
//
// Priority:
//  1. explicit — if non-empty (caller passed an explicit URL param),
//     it wins.
//  2. stored — look up user_library_settings for (userID, libraryID);
//     if a row exists with a parseable sort_spec, use it.
//  3. nil — caller should fall back to whatever hard-coded default
//     it was using before this feature shipped.
//
// Errors from the reader are swallowed: sort is a non-critical UX
// affordance and should never fail a request. An invalid stored spec
// is treated the same as no spec (returns nil).
func ResolveForLibrary(
	ctx context.Context,
	reader LibrarySettingsReader,
	userID, libraryID int,
	explicit []SortLevel,
) []SortLevel {
	if len(explicit) > 0 {
		return explicit
	}

	settings, err := reader.GetLibrarySettings(ctx, userID, libraryID)
	if err != nil || settings == nil || settings.SortSpec == nil {
		return nil
	}

	levels, err := Parse(*settings.SortSpec)
	if err != nil {
		return nil
	}
	return levels
}
