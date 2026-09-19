package worker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/plugins"
	"github.com/shishobooks/shisho/pkg/sidecar"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// Capture database writes at the handler boundary, including FTS churn.
type identifyWrites struct{ queries []string }

func (q *identifyWrites) BeforeQuery(ctx context.Context, _ *bun.QueryEvent) context.Context {
	return ctx
}
func (q *identifyWrites) AfterQuery(_ context.Context, event *bun.QueryEvent) {
	sql := strings.ToUpper(strings.TrimSpace(event.Query))
	if strings.HasPrefix(sql, "INSERT") || strings.HasPrefix(sql, "DELETE") {
		q.queries = append(q.queries, event.Query)
	}
}

func relationshipFields() map[string]any {
	return map[string]any{
		"authors":   []map[string]any{{"name": "First Author"}, {"name": "Second Author", "role": "editor"}},
		"genres":    []string{"Fiction", "Adventure"},
		"tags":      []string{"Favorite", "Read"},
		"narrators": []string{"First Narrator", "Second Narrator"},
	}
}

func relationshipIntents(intent string) map[string]string {
	return map[string]string{"authors": intent, "genres": intent, "tags": intent, "narrators": intent}
}

// The fixture uses the real apply route and stores. The synthetic file is
// marked M4B only to enable narrator editing; these tests do not scan it again.
func newRelationshipApplyFixture(t *testing.T) *identifyScanFixture {
	t.Helper()
	f := newIdentifyScanFixture(t)
	book, file := f.retrieve(t)
	file.FileType = models.FileTypeM4B
	_, err := f.tc.db.NewUpdate().Model(file).Column("file_type").WherePK().Exec(f.tc.ctx)
	require.NoError(t, err)
	postIdentifyApply(t, newIdentifyApplyServer(t, f.tc), plugins.PluginApplyPayload{
		BookID: book.ID, FileID: &file.ID, Fields: relationshipFields(),
		Sources: relationshipIntents(plugins.SourceIntentPlugin), PluginScope: "test", PluginID: "auto-enricher",
	})
	return f
}

func TestIdentifyRelationships_ClearRemovesSource(t *testing.T) {
	t.Parallel()
	f := newRelationshipApplyFixture(t)
	book, file := f.retrieve(t)
	postIdentifyApply(t, newIdentifyApplyServer(t, f.tc), plugins.PluginApplyPayload{
		BookID: book.ID, FileID: &file.ID,
		Fields:  map[string]any{"authors": []any{}, "genres": []any{}, "tags": []any{}, "narrators": []any{}},
		Sources: relationshipIntents(plugins.SourceIntentUser), PluginScope: "test", PluginID: "auto-enricher",
	})
	book, file = f.retrieve(t)
	assert.Empty(t, book.Authors)
	assert.Empty(t, book.BookGenres)
	assert.Empty(t, book.BookTags)
	assert.Empty(t, file.Narrators)
	assert.Empty(t, book.AuthorSource)
	assert.Nil(t, book.GenreSource)
	assert.Nil(t, book.TagSource)
	assert.Nil(t, file.NarratorSource)
	// AuthorSource is represented as a nullzero string by the model.
	var source *string
	require.NoError(t, f.tc.db.NewRaw("SELECT author_source FROM books WHERE id = ?", book.ID).Scan(f.tc.ctx, &source))
	assert.Nil(t, source)
}

func TestIdentifyRelationships_NoOpPreservesSourceWithoutWrites(t *testing.T) {
	t.Parallel()
	for _, source := range []string{models.DataSourceManual, "plugin:test/auto-enricher"} {
		t.Run(source, func(t *testing.T) {
			t.Parallel()
			f := newRelationshipApplyFixture(t)
			book, file := f.retrieve(t)
			_, err := f.tc.db.NewRaw("UPDATE books SET author_source = ?, genre_source = ?, tag_source = ? WHERE id = ?", source, source, source, book.ID).Exec(f.tc.ctx)
			require.NoError(t, err)
			_, err = f.tc.db.NewRaw("UPDATE files SET narrator_source = ? WHERE id = ?", source, file.ID).Exec(f.tc.ctx)
			require.NoError(t, err)
			fields := relationshipFields()
			fields["genres"] = []string{"Adventure", "fiction"}
			fields["tags"] = []string{"Read", "favorite"}
			fields["authors"] = []map[string]any{{"name": "first author", "role": ""}, {"name": "Second Author", "role": "editor"}}
			writes := &identifyWrites{}
			f.tc.db.AddQueryHook(writes)
			postIdentifyApply(t, newIdentifyApplyServer(t, f.tc), plugins.PluginApplyPayload{
				BookID: book.ID, FileID: &file.ID, Fields: fields, Sources: relationshipIntents(plugins.SourceIntentPlugin), PluginScope: "test", PluginID: "auto-enricher",
			})
			updated, updatedFile := f.retrieve(t)
			assert.Equal(t, source, updated.AuthorSource)
			assert.Equal(t, &source, updated.GenreSource)
			assert.Equal(t, &source, updated.TagSource)
			assert.Equal(t, &source, updatedFile.NarratorSource)
			assert.Equal(t, book.Authors, updated.Authors)
			assert.Equal(t, file.Narrators, updatedFile.Narrators)
			assert.Empty(t, writes.queries, "no-op must not delete/insert relationships or reindex FTS")
		})
	}
}

func TestIdentifyRelationships_ChangedCollectionsUseIntent(t *testing.T) {
	t.Parallel()
	for _, intent := range []string{plugins.SourceIntentUser, plugins.SourceIntentPlugin, ""} {
		name := intent
		if name == "" {
			name = "missing"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newRelationshipApplyFixture(t)
			book, file := f.retrieve(t)
			fields := map[string]any{
				"authors":   []map[string]any{{"name": "Second Author", "role": "editor"}, {"name": "First Author"}},
				"narrators": []string{"Second Narrator", "First Narrator"},
				"genres":    []string{"Fiction"}, "tags": []string{"Read", "Favorite", "New Tag"},
			}
			sources := relationshipIntents(intent)
			if intent == "" {
				sources = nil
			}
			postIdentifyApply(t, newIdentifyApplyServer(t, f.tc), plugins.PluginApplyPayload{
				BookID: book.ID, FileID: &file.ID, Fields: fields, Sources: sources, PluginScope: "test", PluginID: "auto-enricher",
			})
			book, file = f.retrieve(t)
			want := models.DataSourceManual
			if intent == plugins.SourceIntentPlugin {
				want = "plugin:test/auto-enricher"
			}
			assert.Equal(t, want, book.AuthorSource)
			assert.Equal(t, &want, book.GenreSource)
			assert.Equal(t, &want, book.TagSource)
			assert.Equal(t, &want, file.NarratorSource)
			require.Len(t, book.Authors, 2)
			assert.Equal(t, "Second Author", book.Authors[0].Person.Name)
			assert.Equal(t, "editor", *book.Authors[0].Role)
			assert.Equal(t, "First Author", book.Authors[1].Person.Name)
			require.Len(t, file.Narrators, 2)
			assert.Equal(t, "Second Narrator", file.Narrators[0].Person.Name)
			assert.Equal(t, "First Narrator", file.Narrators[1].Person.Name)
			require.Len(t, book.BookGenres, 1)
			assert.Equal(t, "Fiction", book.BookGenres[0].Genre.Name)
			require.Len(t, book.BookTags, 3)
			bookSidecar, err := sidecar.ReadBookSidecarFromModel(book, file)
			require.NoError(t, err)
			require.NotNil(t, bookSidecar)
			require.Len(t, bookSidecar.Authors, 2)
			assert.Equal(t, "Second Author", bookSidecar.Authors[0].Name)
			assert.Equal(t, []string{"Fiction"}, bookSidecar.Genres)
			assert.ElementsMatch(t, []string{"Read", "Favorite", "New Tag"}, bookSidecar.Tags)
			fileSidecar, err := sidecar.ReadFileSidecar(file.Filepath)
			require.NoError(t, err)
			require.NotNil(t, fileSidecar)
			require.Len(t, fileSidecar.Narrators, 2)
			assert.Equal(t, "Second Narrator", fileSidecar.Narrators[0].Name)
		})
	}
}

func TestIdentifyRelationships_RoleChangeAndOmission(t *testing.T) {
	t.Parallel()
	f := newRelationshipApplyFixture(t)
	original, originalFile := f.retrieve(t)
	postIdentifyApply(t, newIdentifyApplyServer(t, f.tc), plugins.PluginApplyPayload{
		BookID: original.ID, FileID: &originalFile.ID,
		Fields:  map[string]any{"authors": []map[string]any{{"name": "First Author", "role": "writer"}, {"name": "Second Author", "role": "editor"}}},
		Sources: relationshipIntents(plugins.SourceIntentUser), PluginScope: "test", PluginID: "auto-enricher",
	})
	book, file := f.retrieve(t)
	assert.Equal(t, models.DataSourceManual, book.AuthorSource)
	require.Len(t, book.Authors, 2)
	assert.Equal(t, "writer", *book.Authors[0].Role)
	assert.Equal(t, original.BookGenres, book.BookGenres)
	assert.Equal(t, original.GenreSource, book.GenreSource)
	assert.Equal(t, original.BookTags, book.BookTags)
	assert.Equal(t, original.TagSource, book.TagSource)
	assert.Equal(t, originalFile.Narrators, file.Narrators)
	assert.Equal(t, originalFile.NarratorSource, file.NarratorSource)
}

func TestIdentifyRelationships_AliasesAreNoOps(t *testing.T) {
	t.Parallel()
	f := newRelationshipApplyFixture(t)
	book, file := f.retrieve(t)
	for _, alias := range []struct {
		table, column, name string
		id                  int
	}{
		{"person_aliases", "person_id", "Author Alias", book.Authors[0].PersonID},
		{"person_aliases", "person_id", "Narrator Alias", file.Narrators[0].PersonID},
		{"genre_aliases", "genre_id", "Genre Alias", book.BookGenres[0].GenreID},
		{"tag_aliases", "tag_id", "Tag Alias", book.BookTags[0].TagID},
	} {
		_, err := f.tc.db.NewRaw("INSERT INTO "+alias.table+" (created_at, "+alias.column+", name, library_id) VALUES (CURRENT_TIMESTAMP, ?, ?, ?)", alias.id, alias.name, book.LibraryID).Exec(f.tc.ctx)
		require.NoError(t, err)
	}
	fields := relationshipFields()
	fields["authors"] = []map[string]any{{"name": "Author Alias"}, {"name": "Second Author", "role": "editor"}}
	fields["narrators"] = []string{"Narrator Alias", "Second Narrator"}
	// Duplicate aliases still describe one member of an unordered set.
	fields["genres"] = []string{"Genre Alias", book.BookGenres[1].Genre.Name, book.BookGenres[0].Genre.Name}
	fields["tags"] = []string{"Tag Alias", book.BookTags[1].Tag.Name, book.BookTags[0].Tag.Name}
	writes := &identifyWrites{}
	f.tc.db.AddQueryHook(writes)
	postIdentifyApply(t, newIdentifyApplyServer(t, f.tc), plugins.PluginApplyPayload{
		BookID: book.ID, FileID: &file.ID, Fields: fields, Sources: relationshipIntents(plugins.SourceIntentUser), PluginScope: "test", PluginID: "auto-enricher",
	})
	updated, updatedFile := f.retrieve(t)
	assert.Equal(t, book.AuthorSource, updated.AuthorSource)
	assert.Equal(t, book.GenreSource, updated.GenreSource)
	assert.Equal(t, book.TagSource, updated.TagSource)
	assert.Equal(t, file.NarratorSource, updatedFile.NarratorSource)
	assert.Empty(t, writes.queries)
}

func TestIdentifyRelationships_InsertFailureReturnsError(t *testing.T) {
	t.Parallel()
	for _, target := range []struct {
		field, table string
		value        any
	}{
		{"authors", "authors", []map[string]any{{"name": "New Author"}}},
		{"genres", "book_genres", []string{"New Genre"}},
		{"tags", "book_tags", []string{"New Tag"}},
		{"narrators", "narrators", []string{"New Narrator"}},
	} {
		t.Run(target.field, func(t *testing.T) {
			t.Parallel()
			f := newRelationshipApplyFixture(t)
			book, file := f.retrieve(t)
			_, err := f.tc.db.ExecContext(f.tc.ctx, "CREATE TRIGGER fail_insert BEFORE INSERT ON "+target.table+" BEGIN SELECT RAISE(ABORT, 'injected insert failure'); END")
			require.NoError(t, err)
			payload := plugins.PluginApplyPayload{BookID: book.ID, FileID: &file.ID, Fields: map[string]any{target.field: target.value}, Sources: relationshipIntents(plugins.SourceIntentUser), PluginScope: "test", PluginID: "auto-enricher"}
			body, err := json.Marshal(payload)
			require.NoError(t, err)
			req := httptest.NewRequest(http.MethodPost, "/plugins/apply", strings.NewReader(string(body)))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			newIdentifyApplyServer(t, f.tc).ServeHTTP(rec, req)
			assert.Equal(t, http.StatusInternalServerError, rec.Code, rec.Body.String())
			// A failed collection must not be stamped as a completed manual edit.
			updated, updatedFile := f.retrieve(t)
			assert.Equal(t, book.AuthorSource, updated.AuthorSource)
			assert.Equal(t, book.GenreSource, updated.GenreSource)
			assert.Equal(t, book.TagSource, updated.TagSource)
			assert.Equal(t, file.NarratorSource, updatedFile.NarratorSource)
		})
	}
}
