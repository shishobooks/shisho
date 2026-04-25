package review

import (
	"context"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/appsettings"
)

const SettingsKey = "review_criteria"

// Field constants — every name must be in either UniversalCandidates or AudioCandidates.
const (
	FieldAuthors     = "authors"
	FieldDescription = "description"
	FieldCover       = "cover"
	FieldGenres      = "genres"
	FieldTags        = "tags"
	FieldSeries      = "series"
	FieldSubtitle    = "subtitle"
	FieldPublisher   = "publisher"
	FieldImprint     = "imprint"
	FieldIdentifiers = "identifiers"
	FieldReleaseDate = "release_date"
	FieldLanguage    = "language"
	FieldURL         = "url"
	FieldNarrators   = "narrators"
	FieldChapters    = "chapters"
	FieldAbridged    = "abridged"
)

// UniversalCandidates is the set of fields that can be required for all books.
var UniversalCandidates = []string{
	FieldAuthors, FieldDescription, FieldCover, FieldGenres, FieldTags,
	FieldSeries, FieldSubtitle, FieldPublisher, FieldImprint, FieldIdentifiers,
	FieldReleaseDate, FieldLanguage, FieldURL,
}

// AudioCandidates is the set of fields that can be required only when the book has at least one audio file.
var AudioCandidates = []string{FieldNarrators, FieldChapters, FieldAbridged}

type Criteria struct {
	BookFields  []string `json:"book_fields"`
	AudioFields []string `json:"audio_fields"`
}

// Default returns the seeded review criteria.
func Default() Criteria {
	return Criteria{
		BookFields:  []string{FieldAuthors, FieldDescription, FieldCover, FieldGenres},
		AudioFields: []string{FieldNarrators},
	}
}

// Load reads the criteria from app settings, falling back to Default if unset.
func Load(ctx context.Context, settings *appsettings.Service) (Criteria, error) {
	var c Criteria
	ok, err := settings.GetJSON(ctx, SettingsKey, &c)
	if err != nil {
		return Criteria{}, errors.WithStack(err)
	}
	if !ok {
		return Default(), nil
	}
	// Defensive: if either slice is nil from older serializations, use defaults for that side.
	if c.BookFields == nil {
		c.BookFields = Default().BookFields
	}
	if c.AudioFields == nil {
		c.AudioFields = Default().AudioFields
	}
	return c, nil
}

// Save persists the criteria.
func Save(ctx context.Context, settings *appsettings.Service, c Criteria) error {
	return errors.WithStack(settings.SetJSON(ctx, SettingsKey, c))
}

// Validate returns an error if any field name in the criteria isn't a known candidate.
func Validate(c Criteria) error {
	if err := validateAgainst(c.BookFields, UniversalCandidates); err != nil {
		return errors.Wrap(err, "book_fields")
	}
	if err := validateAgainst(c.AudioFields, AudioCandidates); err != nil {
		return errors.Wrap(err, "audio_fields")
	}
	return nil
}

func validateAgainst(fields []string, allowed []string) error {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, a := range allowed {
		allowedSet[a] = struct{}{}
	}
	for _, f := range fields {
		if _, ok := allowedSet[f]; !ok {
			return errors.Errorf("unknown field %q", f)
		}
	}
	return nil
}
