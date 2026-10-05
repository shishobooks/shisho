package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

// keyFTSRowsByEntityID rebuilds books_fts, series_fts, persons_fts,
// genres_fts, tags_fts, and publishers_fts so every row's rowid equals the id
// of the entity it indexes. The search service now inserts rows that way and
// deletes one entity's row with WHERE rowid = ?, a direct lookup. Rows
// written before this migration have auto-assigned rowids, so those deletes
// would miss them.
//
// The statements are a snapshot of search.Service.RebuildAllIndexes at the
// time of this migration. They are copied rather than called so later
// changes to the search service cannot alter what this migration does
// against the schema as it stood here. Rebuilding from the source tables
// also drops rows left behind for deleted entities and any duplicate rows.
func keyFTSRowsByEntityID(ctx context.Context, db *bun.DB) error {
	return db.RunInTx(ctx, nil, rebuildFTSTablesInTx)
}

// rebuildFTSTablesInTx empties every FTS table and refills it from the
// source tables. 20261005000000 reuses it after repairing rows, since the
// search tables have not changed since this snapshot. It is frozen like any
// migration: never update it for a later schema, since both migrations must
// keep doing what they did against the schema they ran on.
func rebuildFTSTablesInTx(ctx context.Context, tx bun.Tx) error {
	for _, query := range []string{
		"DELETE FROM books_fts",
		"DELETE FROM series_fts",
		"DELETE FROM persons_fts",
		"DELETE FROM genres_fts",
		"DELETE FROM tags_fts",
		"DELETE FROM publishers_fts",
		`INSERT INTO books_fts (rowid, book_id, library_id, title, filepath, subtitle, authors, filenames, narrators, series_names)
		SELECT
			b.id AS rowid,
			b.id,
			b.library_id,
			b.title,
			b.filepath,
			COALESCE(b.subtitle, ''),
			COALESCE((SELECT GROUP_CONCAT(name, ' ') FROM (
				SELECT DISTINCT p.name FROM authors a JOIN persons p ON a.person_id = p.id WHERE a.book_id = b.id
				UNION
				SELECT DISTINCT pa.name FROM authors a JOIN person_aliases pa ON pa.person_id = a.person_id WHERE a.book_id = b.id
			)), ''),
			COALESCE((SELECT GROUP_CONCAT(f.filepath, ' ') FROM files f WHERE f.book_id = b.id), ''),
			COALESCE((SELECT GROUP_CONCAT(name, ' ') FROM (
				SELECT DISTINCT p.name FROM files f JOIN narrators n ON n.file_id = f.id JOIN persons p ON n.person_id = p.id WHERE f.book_id = b.id
				UNION
				SELECT DISTINCT pa.name FROM files f JOIN narrators n ON n.file_id = f.id JOIN person_aliases pa ON pa.person_id = n.person_id WHERE f.book_id = b.id
			)), ''),
			COALESCE((SELECT GROUP_CONCAT(name, ' ') FROM (
				SELECT s.name FROM book_series bs JOIN series s ON bs.series_id = s.id WHERE bs.book_id = b.id
				UNION
				SELECT sa.name FROM book_series bs JOIN series_aliases sa ON sa.series_id = bs.series_id WHERE bs.book_id = b.id
			)), '')
		FROM books b`,
		`INSERT INTO series_fts (rowid, series_id, library_id, name, description, book_titles, book_authors)
		SELECT
			s.id AS rowid,
			s.id,
			s.library_id,
			s.name || COALESCE(' ' || (SELECT GROUP_CONCAT(sa.name, ' ') FROM series_aliases sa WHERE sa.series_id = s.id), ''),
			COALESCE(s.description, ''),
			COALESCE((SELECT GROUP_CONCAT(b.title, ' ') FROM book_series bs JOIN books b ON bs.book_id = b.id WHERE bs.series_id = s.id), ''),
			COALESCE((SELECT GROUP_CONCAT(name, ' ') FROM (SELECT DISTINCT p.name FROM book_series bs JOIN books b ON bs.book_id = b.id JOIN authors a ON a.book_id = b.id JOIN persons p ON a.person_id = p.id WHERE bs.series_id = s.id)), '')
		FROM series s`,
		`INSERT INTO persons_fts (rowid, person_id, library_id, name, sort_name)
		SELECT id AS rowid, id, library_id,
			name || COALESCE(' ' || (SELECT GROUP_CONCAT(pa.name, ' ') FROM person_aliases pa WHERE pa.person_id = persons.id), ''),
			sort_name
		FROM persons`,
		`INSERT INTO genres_fts (rowid, genre_id, library_id, name)
		SELECT id AS rowid, id, library_id,
			name || COALESCE(' ' || (SELECT GROUP_CONCAT(ga.name, ' ') FROM genre_aliases ga WHERE ga.genre_id = genres.id), '')
		FROM genres`,
		`INSERT INTO tags_fts (rowid, tag_id, library_id, name)
		SELECT id AS rowid, id, library_id,
			name || COALESCE(' ' || (SELECT GROUP_CONCAT(ta.name, ' ') FROM tag_aliases ta WHERE ta.tag_id = tags.id), '')
		FROM tags`,
		`INSERT INTO publishers_fts (rowid, publisher_id, library_id, name)
		SELECT id AS rowid, id, library_id,
			name || COALESCE(' ' || (SELECT GROUP_CONCAT(pa.name, ' ') FROM publisher_aliases pa WHERE pa.publisher_id = publishers.id), '')
		FROM publishers`,
	} {
		if _, err := tx.ExecContext(ctx, query); err != nil {
			return errors.Wrap(err, "failed to rebuild FTS tables keyed by entity id")
		}
	}
	return nil
}

func init() {
	up := keyFTSRowsByEntityID

	// Nothing to undo. The previous search code deletes FTS rows by the
	// stored id column, so rows keyed by entity id work with it unchanged.
	//
	// Running an older binary against this database and then upgrading again
	// does not rerun this migration, and rows the older binary wrote carry
	// auto-assigned rowids. Book 5 re-indexed there could land at rowid 101,
	// and creating book 101 later would replace that row. The next library
	// scan rebuilds every index and repairs it. Rolling back with the
	// migrate command removes this migration's record, so a later migrate
	// reruns the rebuild.
	down := func(context.Context, *bun.DB) error {
		return nil
	}

	Migrations.MustRegister(up, down)
}
