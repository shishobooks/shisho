package plugins

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/binder"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const applyTestPluginSource = "plugin:test/enricher"

// applyForAttribution posts payload through applyMetadata using the real
// binder, so payload validation (unknown intents) is exercised too.
func applyForAttribution(t *testing.T, h *handler, payload PluginApplyPayload) error {
	t.Helper()
	payload.PluginScope = "test"
	payload.PluginID = "enricher"
	body, err := json.Marshal(payload)
	require.NoError(t, err)

	e := echo.New()
	b, err := binder.New()
	require.NoError(t, err)
	e.Binder = b
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	c := e.NewContext(req, httptest.NewRecorder())
	c.Set("user", &models.User{ID: 1, LibraryAccess: []*models.UserLibraryAccess{{LibraryID: nil}}})
	return h.applyMetadata(c)
}

// newAttributionFixture builds a book and EPUB file whose scalars all carry
// the given source, plus a handler whose publisher finder resolves
// "Stored Publisher" to the stored publisher ID.
func newAttributionFixture(t *testing.T, source string) (*models.Book, *models.File, *stubBookStoreForApply, *handler) {
	t.Helper()
	book, file := newApplyTestBookWithFile(t, "Stored Title", models.FileTypeEPUB)
	subtitle := "Stored Subtitle"
	description := "Stored description"
	url := "https://example.com/stored"
	language := "en"
	abridged := false
	releaseDate := time.Date(2024, time.January, 2, 0, 0, 0, 0, time.UTC)
	publisherID := 9

	book.TitleSource = source
	book.SortTitle = "Stored Title"
	book.SortTitleSource = models.DataSourceFilepath
	book.Subtitle = &subtitle
	book.SubtitleSource = &source
	book.Description = &description
	book.DescriptionSource = &source
	file.PublisherID = &publisherID
	file.Publisher = &models.Publisher{ID: publisherID, Name: "Stored Publisher"}
	file.PublisherSource = &source
	file.URL = &url
	file.URLSource = &source
	file.Language = &language
	file.LanguageSource = &source
	file.Abridged = &abridged
	file.AbridgedSource = &source
	file.ReleaseDate = &releaseDate
	file.ReleaseDateSource = &source

	store := &stubBookStoreForApply{stubBookStoreForPersist: stubBookStoreForPersist{book: book}}
	h := newApplyTestHandler(store)
	h.enrich.publisherFinder = &stubPublisherFinderByName{ids: map[string]int{"Stored Publisher": publisherID}}
	return book, file, store, h
}

// stubPublisherFinderByName resolves known names to fixed IDs and anything
// else to ID 100, so tests can model "same publisher" versus "new publisher".
type stubPublisherFinderByName struct {
	ids map[string]int
}

func (s *stubPublisherFinderByName) FindOrCreatePublisher(_ context.Context, name string, _ int) (*models.Publisher, error) {
	if id, ok := s.ids[name]; ok {
		return &models.Publisher{ID: id, Name: name}, nil
	}
	return &models.Publisher{ID: 100, Name: name}, nil
}

func allScalarIntents(intent string) map[string]string {
	return map[string]string{
		"title": intent, "subtitle": intent, "description": intent,
		"publisher": intent, "url": intent, "language": intent,
		"release_date": intent, "abridged": intent,
	}
}

// Defect: file-level rows default ON regardless of source, so applying an
// unchanged manual value used to rewrite its source to the plugin.
func TestApplyMetadata_Attribution_UnchangedScalarsPreserveStoredSource(t *testing.T) {
	t.Parallel()

	book, file, store, h := newAttributionFixture(t, models.DataSourceManual)

	err := applyForAttribution(t, h, PluginApplyPayload{
		BookID: book.ID,
		FileID: &file.ID,
		Fields: map[string]any{
			// Each value differs from the stored one only by canonicalization.
			"title":        "  Stored Title ",
			"subtitle":     "Stored Subtitle  ",
			"description":  "<p>Stored description</p>",
			"publisher":    " Stored Publisher ",
			"url":          " https://example.com/stored ",
			"language":     "EN",
			"release_date": "2024-01-02T00:00:00Z",
			"abridged":     false,
		},
		Sources: allScalarIntents(SourceIntentPlugin),
	})
	require.NoError(t, err)

	assert.Equal(t, "Stored Title", book.Title)
	assert.Equal(t, models.DataSourceManual, book.TitleSource)
	assert.Equal(t, models.DataSourceFilepath, book.SortTitleSource)
	assert.Equal(t, models.DataSourceManual, *book.SubtitleSource)
	assert.Equal(t, "Stored description", *book.Description)
	assert.Equal(t, models.DataSourceManual, *book.DescriptionSource)
	assert.Equal(t, 9, *file.PublisherID)
	assert.Equal(t, models.DataSourceManual, *file.PublisherSource)
	assert.Equal(t, models.DataSourceManual, *file.URLSource)
	assert.Equal(t, models.DataSourceManual, *file.LanguageSource)
	assert.Equal(t, models.DataSourceManual, *file.ReleaseDateSource)
	assert.Equal(t, models.DataSourceManual, *file.AbridgedSource)
	assert.Empty(t, store.updatedBookColumns, "a no-op apply must not write book columns")
	assert.Empty(t, store.updatedFileColumns, "a no-op apply must not write file columns")
}

func TestApplyMetadata_Attribution_ChangedScalarsMapIntentToCanonicalSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		sources map[string]string
		want    string
	}{
		{"plugin intent stores the specific plugin source", allScalarIntents(SourceIntentPlugin), applyTestPluginSource},
		{"user intent stores manual", allScalarIntents(SourceIntentUser), models.DataSourceManual},
		{"missing intent is treated as user", nil, models.DataSourceManual},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			book, file, _, h := newAttributionFixture(t, "epub_metadata")

			err := applyForAttribution(t, h, PluginApplyPayload{
				BookID: book.ID,
				FileID: &file.ID,
				Fields: map[string]any{
					"title":        "New Title",
					"subtitle":     "New Subtitle",
					"description":  "New description",
					"publisher":    "New Publisher",
					"url":          "https://example.com/new",
					"language":     "fr",
					"release_date": "2025-06-07",
					"abridged":     true,
				},
				Sources: tt.sources,
			})
			require.NoError(t, err)

			assert.Equal(t, "New Title", book.Title)
			assert.Equal(t, tt.want, book.TitleSource)
			assert.Equal(t, "New Subtitle", *book.Subtitle)
			assert.Equal(t, tt.want, *book.SubtitleSource)
			assert.Equal(t, "New description", *book.Description)
			assert.Equal(t, tt.want, *book.DescriptionSource)
			assert.Equal(t, 100, *file.PublisherID)
			assert.Equal(t, tt.want, *file.PublisherSource)
			assert.Equal(t, "https://example.com/new", *file.URL)
			assert.Equal(t, tt.want, *file.URLSource)
			assert.Equal(t, "fr", *file.Language)
			assert.Equal(t, tt.want, *file.LanguageSource)
			assert.Equal(t, time.Date(2025, time.June, 7, 0, 0, 0, 0, time.UTC), *file.ReleaseDate)
			assert.Equal(t, tt.want, *file.ReleaseDateSource)
			assert.True(t, *file.Abridged)
			assert.Equal(t, tt.want, *file.AbridgedSource)
		})
	}
}

func TestApplyMetadata_Attribution_RejectsUnknownIntent(t *testing.T) {
	t.Parallel()

	for _, intent := range []string{models.DataSourceManual, applyTestPluginSource, ""} {
		t.Run(intent, func(t *testing.T) {
			t.Parallel()

			book, file, store, h := newAttributionFixture(t, "epub_metadata")

			err := applyForAttribution(t, h, PluginApplyPayload{
				BookID:  book.ID,
				FileID:  &file.ID,
				Fields:  map[string]any{"subtitle": "New Subtitle"},
				Sources: map[string]string{"subtitle": intent},
			})

			var ec *errcodes.Error
			require.ErrorAs(t, err, &ec)
			assert.Equal(t, http.StatusUnprocessableEntity, ec.HTTPCode)
			assert.Equal(t, "Stored Subtitle", *book.Subtitle)
			assert.Equal(t, "epub_metadata", *book.SubtitleSource)
			assert.Empty(t, store.updatedBookColumns)
		})
	}
}

// The per-entry identifier intent is reserved for Identifier attribution. It
// is validated now so an invalid value can never be accepted and later stored.
func TestApplyMetadata_Attribution_RejectsUnknownIdentifierEntryIntent(t *testing.T) {
	t.Parallel()

	book, file, _, h := newAttributionFixture(t, "epub_metadata")
	identStore := &stubIdentStoreForPersist{}
	h.enrich.identStore = identStore

	err := applyForAttribution(t, h, PluginApplyPayload{
		BookID: book.ID,
		FileID: &file.ID,
		Fields: map[string]any{
			"identifiers": []any{map[string]any{"type": "isbn_13", "value": "9780316769488", "source": "manual"}},
		},
	})

	var ec *errcodes.Error
	require.ErrorAs(t, err, &ec)
	assert.Equal(t, http.StatusUnprocessableEntity, ec.HTTPCode)
	assert.Empty(t, identStore.deleteCalls)
}

func TestApplyMetadata_Attribution_AcceptsReservedIdentifierEntryIntent(t *testing.T) {
	t.Parallel()

	book, file, _, h := newAttributionFixture(t, "epub_metadata")
	h.enrich.identStore = &stubIdentStoreForPersist{}

	err := applyForAttribution(t, h, PluginApplyPayload{
		BookID: book.ID,
		FileID: &file.ID,
		Fields: map[string]any{
			"identifiers": []any{map[string]any{"type": "isbn_13", "value": "9780316769488", "source": SourceIntentUser}},
		},
	})
	require.NoError(t, err)
}

func TestApplyMetadata_Attribution_OmittedFieldsKeepValueAndSource(t *testing.T) {
	t.Parallel()

	book, file, store, h := newAttributionFixture(t, models.DataSourceManual)

	err := applyForAttribution(t, h, PluginApplyPayload{
		BookID:  book.ID,
		FileID:  &file.ID,
		Fields:  map[string]any{"subtitle": "New Subtitle"},
		Sources: map[string]string{"subtitle": SourceIntentPlugin},
	})
	require.NoError(t, err)

	assert.Equal(t, [][]string{{"subtitle", "subtitle_source"}}, store.updatedBookColumns)
	assert.Empty(t, store.updatedFileColumns)
	assert.Equal(t, "Stored Title", book.Title)
	assert.Equal(t, models.DataSourceManual, book.TitleSource)
	assert.Equal(t, "Stored description", *book.Description)
	assert.Equal(t, models.DataSourceManual, *book.DescriptionSource)
	assert.Equal(t, models.DataSourceManual, *file.PublisherSource)
	assert.Equal(t, models.DataSourceManual, *file.LanguageSource)
}

// A value and its source must travel in the same column set so a partial
// failure cannot leave a value attributed to the wrong source.
func TestApplyMetadata_Attribution_ValueAndSourceShareOneColumnSet(t *testing.T) {
	t.Parallel()

	book, file, store, h := newAttributionFixture(t, "epub_metadata")

	err := applyForAttribution(t, h, PluginApplyPayload{
		BookID: book.ID,
		FileID: &file.ID,
		Fields: map[string]any{
			"title":     "New Title",
			"publisher": "New Publisher",
			"abridged":  true,
		},
		Sources: allScalarIntents(SourceIntentUser),
	})
	require.NoError(t, err)

	require.Len(t, store.updatedBookColumns, 1)
	assert.ElementsMatch(t, []string{"title", "title_source", "sort_title", "sort_title_source"}, store.updatedBookColumns[0])
	require.Len(t, store.updatedFileColumns, 1)
	assert.ElementsMatch(t, []string{"publisher_id", "publisher_source", "abridged", "abridged_source"}, store.updatedFileColumns[0])
}

func TestApplyMetadata_Attribution_SortTitleFollowsEditFormConvention(t *testing.T) {
	t.Parallel()

	t.Run("regenerates and stamps filepath when sort title is not manual", func(t *testing.T) {
		t.Parallel()

		book, file, _, h := newAttributionFixture(t, "epub_metadata")
		book.SortTitleSource = applyTestPluginSource

		err := applyForAttribution(t, h, PluginApplyPayload{
			BookID:  book.ID,
			FileID:  &file.ID,
			Fields:  map[string]any{"title": "The New Title"},
			Sources: map[string]string{"title": SourceIntentUser},
		})
		require.NoError(t, err)

		assert.Equal(t, models.DataSourceManual, book.TitleSource)
		assert.Equal(t, "New Title, The", book.SortTitle)
		assert.Equal(t, models.DataSourceFilepath, book.SortTitleSource)
	})

	t.Run("leaves a manual sort title alone", func(t *testing.T) {
		t.Parallel()

		book, file, store, h := newAttributionFixture(t, "epub_metadata")
		book.SortTitle = "Custom Sort"
		book.SortTitleSource = models.DataSourceManual

		err := applyForAttribution(t, h, PluginApplyPayload{
			BookID:  book.ID,
			FileID:  &file.ID,
			Fields:  map[string]any{"title": "The New Title"},
			Sources: map[string]string{"title": SourceIntentPlugin},
		})
		require.NoError(t, err)

		assert.Equal(t, "The New Title", book.Title)
		assert.Equal(t, applyTestPluginSource, book.TitleSource)
		assert.Equal(t, "Custom Sort", book.SortTitle)
		assert.Equal(t, models.DataSourceManual, book.SortTitleSource)
		assert.Equal(t, [][]string{{"title", "title_source"}}, store.updatedBookColumns)
	})
}

// Abridged is a nullable boolean: absence, false, and true are three distinct
// stored states, and an Explicit Clear is a fourth request shape.
func TestApplyMetadata_Attribution_AbridgedDistinguishesAbsenceFromFalse(t *testing.T) {
	t.Parallel()

	boolPtr := func(b bool) *bool { return &b }
	manual := models.DataSourceManual
	tests := []struct {
		name       string
		stored     *bool
		selected   any
		wantValue  *bool
		wantSource *string
	}{
		{"absent to false is a change", nil, false, boolPtr(false), &manual},
		{"false to false is a no-op", boolPtr(false), false, boolPtr(false), nil},
		{"false to true is a change", boolPtr(false), true, boolPtr(true), &manual},
		{"true to cleared removes value and source", boolPtr(true), nil, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			book, file, _, h := newAttributionFixture(t, "epub_metadata")
			file.Abridged = tt.stored
			stored := "epub_metadata"
			if tt.stored == nil {
				file.AbridgedSource = nil
			} else {
				file.AbridgedSource = &stored
			}
			if tt.name == "false to false is a no-op" {
				tt.wantSource = &stored
			}

			err := applyForAttribution(t, h, PluginApplyPayload{
				BookID:  book.ID,
				FileID:  &file.ID,
				Fields:  map[string]any{"abridged": tt.selected},
				Sources: map[string]string{"abridged": SourceIntentUser},
			})
			require.NoError(t, err)

			assert.Equal(t, tt.wantValue, file.Abridged)
			assert.Equal(t, tt.wantSource, file.AbridgedSource)
		})
	}
}

// Clearing an already-empty value is a no-op too: it must not write columns.
func TestApplyMetadata_Attribution_ClearingAbsentValueIsNoOp(t *testing.T) {
	t.Parallel()

	book, file := newApplyTestBookWithFile(t, "Stored Title", models.FileTypeEPUB)
	store := &stubBookStoreForApply{stubBookStoreForPersist: stubBookStoreForPersist{book: book}}
	h := newApplyTestHandler(store)

	err := applyForAttribution(t, h, PluginApplyPayload{
		BookID: book.ID,
		FileID: &file.ID,
		Fields: map[string]any{
			"subtitle": "", "description": "", "publisher": "", "url": "",
			"language": "", "release_date": "", "abridged": nil,
		},
	})
	require.NoError(t, err)

	assert.Empty(t, store.updatedBookColumns)
	assert.Empty(t, store.updatedFileColumns)
}

func TestApplyMetadata_Attribution_UnchangedNamePreservesStoredSource(t *testing.T) {
	t.Parallel()

	book, file, store, h := newAttributionFixture(t, "epub_metadata")
	name := "Edition Name"
	manual := models.DataSourceManual
	file.Name = &name
	file.NameSource = &manual
	sameName := " Edition Name "

	err := applyForAttribution(t, h, PluginApplyPayload{
		BookID:   book.ID,
		FileID:   &file.ID,
		Fields:   map[string]any{},
		FileName: &sameName,
		Sources:  map[string]string{SourcesKeyFileName: SourceIntentPlugin},
	})
	require.NoError(t, err)

	assert.Equal(t, "Edition Name", *file.Name)
	assert.Equal(t, models.DataSourceManual, *file.NameSource)
	assert.Empty(t, store.updatedFileColumns)
}

// Before ADR 0006 an Identify clear left the plugin source on the now-absent
// value, which outranks embedded metadata and blocks Scan repopulation.
// Clearing such a field again must heal it, even though the value is already
// absent.
func TestApplyMetadata_Attribution_ClearingAbsentValueRemovesStaleSource(t *testing.T) {
	t.Parallel()

	book, file := newApplyTestBookWithFile(t, "Stored Title", models.FileTypeEPUB)
	stale := applyTestPluginSource
	book.SubtitleSource = &stale
	book.DescriptionSource = &stale
	file.NameSource = &stale
	file.PublisherSource = &stale
	file.URLSource = &stale
	file.ReleaseDateSource = &stale
	file.LanguageSource = &stale
	file.AbridgedSource = &stale
	store := &stubBookStoreForApply{stubBookStoreForPersist: stubBookStoreForPersist{book: book}}
	h := newApplyTestHandler(store)
	emptyName := ""

	err := applyForAttribution(t, h, PluginApplyPayload{
		BookID:   book.ID,
		FileID:   &file.ID,
		FileName: &emptyName,
		Fields: map[string]any{
			"subtitle": "", "description": "", "publisher": "", "url": "",
			"language": "", "release_date": "", "abridged": nil,
		},
	})
	require.NoError(t, err)

	assert.Nil(t, book.SubtitleSource)
	assert.Nil(t, book.DescriptionSource)
	assert.Nil(t, file.NameSource)
	assert.Nil(t, file.PublisherSource)
	assert.Nil(t, file.URLSource)
	assert.Nil(t, file.ReleaseDateSource)
	assert.Nil(t, file.LanguageSource)
	assert.Nil(t, file.AbridgedSource)
	require.Len(t, store.updatedBookColumns, 1)
	assert.ElementsMatch(t, []string{"subtitle", "subtitle_source", "description", "description_source"}, store.updatedBookColumns[0])
	require.Len(t, store.updatedFileColumns, 1)
}
