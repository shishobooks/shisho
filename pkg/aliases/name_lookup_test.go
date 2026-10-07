package aliases_test

import (
	"context"
	"database/sql"
	"strings"
	"sync"
	"testing"

	"github.com/shishobooks/shisho/pkg/aliases"
	"github.com/shishobooks/shisho/pkg/appsettings"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/genres"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/people"
	"github.com/shishobooks/shisho/pkg/publishers"
	"github.com/shishobooks/shisho/pkg/series"
	"github.com/shishobooks/shisho/pkg/tags"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// queryRecorder captures every query except the EXPLAIN queries the test
// runs to inspect them.
type queryRecorder struct {
	mu      sync.Mutex
	queries []string
}

func (r *queryRecorder) BeforeQuery(ctx context.Context, _ *bun.QueryEvent) context.Context {
	return ctx
}

func (r *queryRecorder) AfterQuery(_ context.Context, event *bun.QueryEvent) {
	if strings.HasPrefix(event.Query, "EXPLAIN") {
		return
	}
	r.mu.Lock()
	r.queries = append(r.queries, event.Query)
	r.mu.Unlock()
}

func (r *queryRecorder) take() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	queries := r.queries
	r.queries = nil
	return queries
}

// Case-insensitive name lookups (find-or-create by name, alias resolution,
// alias conflict checks) must compare with `name = ? COLLATE NOCASE` so they
// search the (name COLLATE NOCASE, library_id) unique index on both columns.
// `LOWER(name) = LOWER(?)` cannot use that index: alias tables were scanned
// in full and resource tables were filtered row by row within the library.
// Matching stays case-insensitive and scoped to the library.
func TestNameLookups_CaseInsensitiveUsingNameIndex(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := testdb.New(t)

	insert := func(model any) {
		t.Helper()
		_, err := db.NewInsert().Model(model).Exec(ctx)
		require.NoError(t, err)
	}
	addAlias := func(table, fk string, parentID int, name string, libraryID int) {
		t.Helper()
		_, err := db.NewRaw(
			"INSERT INTO "+table+" ("+fk+", name, library_id) VALUES (?, ?, ?)", parentID, name, libraryID,
		).Exec(ctx)
		require.NoError(t, err)
	}

	lib := &models.Library{Name: "Library", CoverAspectRatio: "book", DownloadFormatPreference: models.DownloadFormatOriginal}
	otherLib := &models.Library{Name: "Other Library", CoverAspectRatio: "book", DownloadFormatPreference: models.DownloadFormatOriginal}
	insert(lib)
	insert(otherLib)

	sanderson := &models.Person{LibraryID: lib.ID, Name: "Brandon Sanderson", SortName: "Sanderson, Brandon", SortNameSource: models.DataSourceFilepath}
	jordan := &models.Person{LibraryID: lib.ID, Name: "Robert Jordan", SortName: "Jordan, Robert", SortNameSource: models.DataSourceFilepath}
	zola := &models.Person{LibraryID: lib.ID, Name: "Émile Zola", SortName: "Zola, Émile", SortNameSource: models.DataSourceFilepath}
	stormlight := &models.Series{LibraryID: lib.ID, Name: "The Stormlight Archive", NameSource: models.DataSourceFilepath, SortName: "Stormlight Archive, The", SortNameSource: models.DataSourceFilepath}
	scifi := &models.Genre{LibraryID: lib.ID, Name: "Science Fiction"}
	fantasy := &models.Genre{LibraryID: lib.ID, Name: "Fantasy"}
	epic := &models.Tag{LibraryID: lib.ID, Name: "Epic"}
	tor := &models.Publisher{LibraryID: lib.ID, Name: "Tor Books"}
	for _, model := range []any{sanderson, jordan, zola, stormlight, scifi, fantasy, epic, tor} {
		insert(model)
	}
	addAlias("person_aliases", "person_id", jordan.ID, "James Oliver Rigney Jr.", lib.ID)
	addAlias("series_aliases", "series_id", stormlight.ID, "Stormlight", lib.ID)
	addAlias("genre_aliases", "genre_id", scifi.ID, "Sci-Fi", lib.ID)
	addAlias("tag_aliases", "tag_id", epic.ID, "Doorstopper", lib.ID)
	addAlias("publisher_aliases", "publisher_id", tor.ID, "Tor", lib.ID)

	peopleSvc := people.NewService(db)
	seriesSvc := series.NewService(db)
	booksSvc := books.NewService(db, appsettings.NewService(db))
	genresSvc := genres.NewService(db)
	tagsSvc := tags.NewService(db)
	publishersSvc := publishers.NewService(db)
	aliasesSvc := aliases.NewService(db)

	recorder := &queryRecorder{}
	db.AddQueryHook(recorder)

	cases := []struct {
		name string
		// lookup returns the id of the matched resource.
		lookup func() (int, error)
		wantID int
		// indexed lists the tables whose name lookup must search the
		// (name COLLATE NOCASE, library_id) unique index.
		indexed []string
	}{
		{
			name: "person by name",
			lookup: func() (int, error) {
				p, err := peopleSvc.FindOrCreatePerson(ctx, "brandon SANDERSON", lib.ID)
				if err != nil {
					return 0, err
				}
				return p.ID, nil
			},
			wantID:  sanderson.ID,
			indexed: []string{"persons"},
		},
		{
			name: "person by alias",
			lookup: func() (int, error) {
				p, err := peopleSvc.FindOrCreatePerson(ctx, "JAMES oliver rigney jr.", lib.ID)
				if err != nil {
					return 0, err
				}
				return p.ID, nil
			},
			wantID:  jordan.ID,
			indexed: []string{"persons", "person_aliases"},
		},
		{
			name: "series by name",
			lookup: func() (int, error) {
				s, err := seriesSvc.FindOrCreateSeries(ctx, "the STORMLIGHT archive", lib.ID, models.DataSourceFilepath)
				if err != nil {
					return 0, err
				}
				return s.ID, nil
			},
			wantID:  stormlight.ID,
			indexed: []string{"series"},
		},
		{
			name: "series by alias from the books service",
			lookup: func() (int, error) {
				s, err := booksSvc.FindOrCreateSeries(ctx, "STORMLIGHT", lib.ID, models.DataSourceFilepath)
				if err != nil {
					return 0, err
				}
				return s.ID, nil
			},
			wantID:  stormlight.ID,
			indexed: []string{"series", "series_aliases"},
		},
		{
			name: "genre by alias",
			lookup: func() (int, error) {
				g, err := genresSvc.FindOrCreateGenre(ctx, "sci-fi", lib.ID)
				if err != nil {
					return 0, err
				}
				return g.ID, nil
			},
			wantID:  scifi.ID,
			indexed: []string{"genres", "genre_aliases"},
		},
		{
			name: "tag by alias",
			lookup: func() (int, error) {
				tag, err := tagsSvc.FindOrCreateTag(ctx, "DOORSTOPPER", lib.ID)
				if err != nil {
					return 0, err
				}
				return tag.ID, nil
			},
			wantID:  epic.ID,
			indexed: []string{"tags", "tag_aliases"},
		},
		{
			name: "publisher by alias",
			lookup: func() (int, error) {
				p, err := publishersSvc.FindOrCreatePublisher(ctx, "tor", lib.ID)
				if err != nil {
					return 0, err
				}
				return p.ID, nil
			},
			wantID:  tor.ID,
			indexed: []string{"publishers", "publisher_aliases"},
		},
		{
			name: "alias resolution",
			lookup: func() (int, error) {
				return aliases.FindResourceIDByAlias(ctx, db, aliases.GenreConfig, "SCI-FI", lib.ID)
			},
			wantID:  scifi.ID,
			indexed: []string{"genre_aliases"},
		},
	}

	for _, tc := range cases {
		recorder.take()
		id, err := tc.lookup()
		require.NoError(t, err, tc.name)
		assert.Equal(t, tc.wantID, id, tc.name)
		plans := explainAll(ctx, t, db, recorder.take())
		for _, table := range tc.indexed {
			assert.Contains(t, plans, "INDEX ux_"+table+"_name_library_id (name=? AND library_id=?)",
				"%s: %s name lookup should search its NOCASE index", tc.name, table)
		}
	}

	// Alias conflict checks match other resources' names and aliases
	// case-insensitively, through the same indexes.
	recorder.take()
	err := aliasesSvc.AddAlias(ctx, aliases.GenreConfig, fantasy.ID, "science FICTION", lib.ID)
	require.Error(t, err)
	assert.Equal(t, errcodes.ValidationError("Alias conflicts with an existing name"), err)
	assert.Contains(t, explainAll(ctx, t, db, recorder.take()),
		"INDEX ux_genres_name_library_id (name=? AND library_id=?)")
	err = aliasesSvc.AddAlias(ctx, aliases.GenreConfig, fantasy.ID, "sci-FI", lib.ID)
	require.Error(t, err)
	assert.Equal(t, errcodes.ValidationError("Alias conflicts with an existing alias"), err)
	assert.Contains(t, explainAll(ctx, t, db, recorder.take()),
		"INDEX ux_genre_aliases_name_library_id (name=? AND library_id=?)")

	// The lookups stay scoped to the library.
	_, err = aliases.FindResourceIDByAlias(ctx, db, aliases.GenreConfig, "sci-fi", otherLib.ID)
	require.ErrorIs(t, err, sql.ErrNoRows)
	otherLibID := otherLib.ID
	name := "brandon sanderson"
	_, err = peopleSvc.RetrievePerson(ctx, people.RetrievePersonOptions{Name: &name, LibraryID: &otherLibID})
	require.ErrorIs(t, err, errcodes.NotFound("Person"))

	// Case folding is ASCII only, as it was with LOWER() (SQLite's built-in
	// LOWER and NOCASE both fold only A-Z), so a non-ASCII letter must match
	// its case exactly.
	libID := lib.ID
	for _, tc := range []struct {
		name  string
		found bool
	}{
		{"Émile ZOLA", true},
		{"émile zola", false},
	} {
		name := tc.name
		p, err := peopleSvc.RetrievePerson(ctx, people.RetrievePersonOptions{Name: &name, LibraryID: &libID})
		if tc.found {
			require.NoError(t, err, name)
			assert.Equal(t, zola.ID, p.ID, name)
		} else {
			require.ErrorIs(t, err, errcodes.NotFound("Person"), name)
		}
	}
}

// explainAll returns the query plan details of every query, one per line.
func explainAll(ctx context.Context, t *testing.T, db *bun.DB, queries []string) string {
	t.Helper()
	var details []string
	for _, q := range queries {
		var plan []struct {
			ID      int    `bun:"id"`
			Parent  int    `bun:"parent"`
			NotUsed int    `bun:"notused"`
			Detail  string `bun:"detail"`
		}
		require.NoError(t, db.NewRaw("EXPLAIN QUERY PLAN "+q).Scan(ctx, &plan), q)
		for _, row := range plan {
			details = append(details, row.Detail)
		}
	}
	return strings.Join(details, "\n")
}
