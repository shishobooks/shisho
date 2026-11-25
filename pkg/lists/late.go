package lists

import (
	"context"
	"database/sql"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/database"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// The Shisho app's List changes arrive through its journal, each made at an
// instant that can be long before it arrives (ADR 0024). These run in the
// caller's transaction, which claims the event, and the caller checks the
// user may make the change. For the order of a List's Books, the latest
// move of each Book wins.

// CreateListIn creates a List in db as CreateList does.
func CreateListIn(ctx context.Context, db bun.IDB, opts CreateListOptions, now time.Time) (*models.List, error) {
	defaultSort := opts.DefaultSort
	if defaultSort == "" {
		defaultSort = models.ListSortAddedAtDesc
		if opts.IsOrdered {
			defaultSort = models.ListSortManual
		}
	}
	list := &models.List{
		CreatedAt:   now,
		UpdatedAt:   now,
		UserID:      opts.UserID,
		Name:        opts.Name,
		Description: opts.Description,
		IsOrdered:   opts.IsOrdered,
		DefaultSort: defaultSort,
	}
	_, err := db.NewInsert().Model(list).Returning("*").Exec(ctx)
	if database.IsUniqueViolation(err) {
		return nil, ErrListNameTaken
	}
	return list, errors.WithStack(err)
}

// RenameListIn renames listID in db, refusing with ErrListNameTaken a name
// its owner already uses.
func RenameListIn(ctx context.Context, db bun.IDB, listID int, name string, now time.Time) error {
	_, err := db.NewUpdate().Model((*models.List)(nil)).
		Set("name = ?", name).
		Set("updated_at = ?", now).
		Where("id = ? AND name != ?", listID, name).
		Exec(ctx)
	if database.IsUniqueViolation(err) {
		return ErrListNameTaken
	}
	return errors.WithStack(err)
}

// AddBookIn puts bookID at the end of listID, if it isn't in it, as added at
// at.
func AddBookIn(ctx context.Context, db bun.IDB, listID, bookID, userID int, at, now time.Time) error {
	list := &models.List{}
	if err := db.NewSelect().Model(list).Where("id = ?", listID).Scan(ctx); err != nil {
		return errors.WithStack(err)
	}
	lb := &models.ListBook{ListID: listID, BookID: bookID, AddedAt: at, AddedByUserID: &userID}
	if list.IsOrdered {
		var last int
		err := db.NewSelect().Model((*models.ListBook)(nil)).ColumnExpr("COALESCE(MAX(sort_order), 0)").
			Where("list_id = ?", listID).Scan(ctx, &last)
		if err != nil {
			return errors.WithStack(err)
		}
		next := last + 1
		lb.SortOrder = &next
	}
	res, err := db.NewInsert().Model(lb).On("CONFLICT (list_id, book_id) DO NOTHING").Exec(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return errors.WithStack(err)
	}
	return touchList(ctx, db, listID, now)
}

// RemoveBookIn takes bookID out of listID, unless it was added or moved
// there after at.
func RemoveBookIn(ctx context.Context, db bun.IDB, listID, bookID int, at, now time.Time) error {
	res, err := db.NewDelete().Model((*models.ListBook)(nil)).
		Where("list_id = ? AND book_id = ?", listID, bookID).
		Where("COALESCE(moved_at, added_at) <= ?", at).
		Exec(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return errors.WithStack(err)
	}
	return touchList(ctx, db, listID, now)
}

// MoveBookIn moves bookID in the ordered listID to just after afterBookID,
// or to the top with nil, unless it was added or moved after at. A Book no
// longer in the List, one whose neighbor left it, and a List that's no
// longer ordered stay as they are.
func MoveBookIn(ctx context.Context, db bun.IDB, listID, bookID int, afterBookID *int, at, now time.Time) error {
	list := &models.List{}
	err := db.NewSelect().Model(list).Where("id = ?", listID).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return errors.WithStack(err)
	}
	if !list.IsOrdered {
		return nil
	}
	var listBooks []*models.ListBook
	err = db.NewSelect().Model(&listBooks).
		Where("list_id = ?", listID).
		OrderExpr("sort_order IS NULL, sort_order ASC, added_at DESC, id ASC").
		Scan(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	var moving *models.ListBook
	order := make([]*models.ListBook, 0, len(listBooks))
	for _, lb := range listBooks {
		if lb.BookID == bookID {
			moving = lb
			continue
		}
		order = append(order, lb)
	}
	if moving == nil {
		return nil
	}
	if placed := moving.AddedAt; moving.MovedAt != nil && moving.MovedAt.After(placed) {
		if moving.MovedAt.After(at) {
			return nil
		}
	} else if placed.After(at) {
		return nil
	}
	index := 0
	if afterBookID != nil {
		index = -1
		for i, lb := range order {
			if lb.BookID == *afterBookID {
				index = i + 1
				break
			}
		}
		if index < 0 {
			return nil
		}
	}
	order = append(order[:index], append([]*models.ListBook{moving}, order[index:]...)...)
	for i, lb := range order {
		position := i + 1
		q := db.NewUpdate().Model((*models.ListBook)(nil)).Set("sort_order = ?", position).Where("id = ?", lb.ID)
		if lb.ID == moving.ID {
			q = q.Set("moved_at = ?", at)
		} else if lb.SortOrder != nil && *lb.SortOrder == position {
			continue
		}
		if _, err := q.Exec(ctx); err != nil {
			return errors.WithStack(err)
		}
	}
	return touchList(ctx, db, listID, now)
}

// touchList marks listID as changed at now.
func touchList(ctx context.Context, db bun.IDB, listID int, now time.Time) error {
	_, err := db.NewUpdate().Model((*models.List)(nil)).Set("updated_at = ?", now).Where("id = ?", listID).Exec(ctx)
	return errors.WithStack(err)
}
