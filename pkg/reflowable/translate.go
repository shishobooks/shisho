package reflowable

import (
	"context"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/models"
)

// Form is a reader's form of position.
type Form int

const (
	// FormCFI is the web reader's.
	FormCFI Form = iota
	// FormXPointer is KOReader's.
	FormXPointer
)

// ErrNotTranslatable is returned for a position that has no reflowable form
// to translate, or a File or copy that isn't an EPUB or KePub.
var ErrNotTranslatable = errors.New("the position doesn't translate")

func translatable(fileType string) bool {
	return fileType == models.FileTypeEPUB || fileType == models.FileTypeKePub
}

// Translate returns position, stored for the File at path of fileType, in
// form for the copy of it delivered as toType: exactly when it resolves, and
// otherwise by the estimate, from the position's percentage at worst. The
// position's locator is in the File's own copy unless its LocatorFileType
// names another.
func Translate(ctx context.Context, path, fileType string, position *models.ReadingPosition, toType string, form Form) (models.ReadingLocator, error) {
	fromType := fileType
	if position.LocatorFileType != nil {
		fromType = *position.LocatorFileType
	}
	reflowablePosition := position.CFI != nil || position.XPointer != nil || position.KoboSpan != nil
	if !reflowablePosition || !translatable(fileType) || !translatable(fromType) || !translatable(toType) {
		return models.ReadingLocator{}, ErrNotTranslatable
	}
	from, err := Open(ctx, path, fromType == models.FileTypeKePub)
	if err != nil {
		return models.ReadingLocator{}, err
	}
	to, err := Open(ctx, path, toType == models.FileTypeKePub)
	if err != nil {
		return models.ReadingLocator{}, err
	}
	place := from.Locate(position.ReadingLocator, position.Percentage)
	if form == FormCFI {
		cfi := to.CFI(place)
		return models.ReadingLocator{CFI: &cfi}, nil
	}
	xp := to.XPointer(place)
	return models.ReadingLocator{XPointer: &xp}, nil
}
