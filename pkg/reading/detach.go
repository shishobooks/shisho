package reading

import (
	"context"
	"encoding/json"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// DetachBooks copies the title and author names of each of bookIDs onto
// every user's Read-throughs of it, for a delete about to remove those
// Books. The delete clears the Read-throughs' Book, and the copy is how
// their users still recognise them (ADR 0015). Book deletes call it first,
// in the same transaction, and a library delete calls DetachLibraryBooks. A
// merge needs neither: MergeBooks moves the Read-throughs to the Book kept.
func DetachBooks(ctx context.Context, db bun.IDB, bookIDs []int) error {
	if len(bookIDs) == 0 {
		return nil
	}
	var books []struct {
		ID    int    `bun:"id"`
		Title string `bun:"title"`
	}
	err := db.NewSelect().
		TableExpr("books AS b").
		Column("b.id", "b.title").
		Where("b.id IN (?)", bun.List(bookIDs)).
		Where("EXISTS (SELECT 1 FROM read_throughs AS rt WHERE rt.book_id = b.id)").
		Scan(ctx, &books)
	if err != nil {
		return errors.WithStack(err)
	}
	if len(books) == 0 {
		return nil
	}

	withReadThroughs := make([]int, len(books))
	for i, b := range books {
		withReadThroughs[i] = b.ID
	}
	authors, err := authorNames(ctx, db, withReadThroughs)
	if err != nil {
		return err
	}
	for _, b := range books {
		names := authors[b.ID]
		if names == nil {
			names = []string{}
		}
		encoded, err := json.Marshal(names)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.NewUpdate().
			Model((*models.ReadThrough)(nil)).
			Set("book_title = ?", b.Title).
			Set("book_authors = ?", string(encoded)).
			Where("book_id = ?", b.ID).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

// DetachLibraryBooks is DetachBooks for every Book in libraryID, for a
// library delete about to cascade to them.
func DetachLibraryBooks(ctx context.Context, db bun.IDB, libraryID int) error {
	var bookIDs []int
	err := db.NewSelect().
		Model((*models.ReadThrough)(nil)).
		ColumnExpr("DISTINCT rt.book_id").
		Where("rt.book_id IN (SELECT id FROM books WHERE library_id = ?)", libraryID).
		Scan(ctx, &bookIDs)
	if err != nil {
		return errors.WithStack(err)
	}
	return DetachBooks(ctx, db, bookIDs)
}

// authorNames returns the names of each of bookIDs' authors, by Book id, in
// the Book's order. A person credited in several roles, as on a comic, is
// named once.
func authorNames(ctx context.Context, db bun.IDB, bookIDs []int) (map[int][]string, error) {
	names := make(map[int][]string, len(bookIDs))
	if len(bookIDs) == 0 {
		return names, nil
	}
	var rows []struct {
		BookID   int    `bun:"book_id"`
		PersonID int    `bun:"person_id"`
		Name     string `bun:"name"`
	}
	err := db.NewSelect().
		TableExpr("authors AS a").
		ColumnExpr("a.book_id, a.person_id, p.name").
		Join("JOIN persons AS p ON p.id = a.person_id").
		Where("a.book_id IN (?)", bun.List(bookIDs)).
		OrderExpr("a.book_id, a.sort_order, a.id").
		Scan(ctx, &rows)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	seen := make(map[[2]int]bool, len(rows))
	for _, row := range rows {
		key := [2]int{row.BookID, row.PersonID}
		if seen[key] {
			continue
		}
		seen[key] = true
		names[row.BookID] = append(names[row.BookID], row.Name)
	}
	return names, nil
}
