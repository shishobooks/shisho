package reading

import (
	"context"
	"database/sql"
	"strconv"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// BookLibraryIDs returns the library of each of bookIDs that exists, by
// Book id. A missing Book has no entry.
func (svc *Service) BookLibraryIDs(ctx context.Context, bookIDs []int) (map[int]int, error) {
	var rows []struct {
		ID        int `bun:"id"`
		LibraryID int `bun:"library_id"`
	}
	err := svc.db.NewSelect().
		Model((*models.Book)(nil)).
		Column("b.id", "b.library_id").
		Where("b.id IN (?)", bun.List(bookIDs)).
		Scan(ctx, &rows)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	result := make(map[int]int, len(rows))
	for _, row := range rows {
		result[row.ID] = row.LibraryID
	}
	return result, nil
}

// BulkFinish marks each of bookIDs read for userID in one transaction, as
// End does for one: it finishes the Read-through underway, or with nothing
// underway records a finished one ending today. A Book whose status is
// already Read is skipped, so it doesn't gain a second finished read. The
// caller checks the user can reach each Book.
func (svc *Service) BulkFinish(ctx context.Context, userID int, bookIDs []int, today string) (BulkReadingResponse, error) {
	var result BulkReadingResponse
	err := svc.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		result = BulkReadingResponse{}
		now := svc.now()
		for _, bookID := range distinct(bookIDs) {
			var status *string
			err := tx.NewSelect().ColumnExpr(StatusExpr(userID, strconv.Itoa(bookID))).Scan(ctx, &status)
			if err != nil {
				return errors.WithStack(err)
			}
			if status != nil && *status == models.ReadingStatusRead {
				result.Skipped++
				continue
			}
			if err := end(ctx, tx, userID, bookID, models.ReadThroughOutcomeFinished, today, now, now); err != nil {
				return err
			}
			result.Changed++
		}
		return nil
	})
	return result, err
}

// BulkNotStarted marks each of bookIDs not started for userID in one
// transaction, as deleting the Read-through underway does for one. A Book
// with nothing underway is skipped. The caller checks the user can reach
// each Book.
func (svc *Service) BulkNotStarted(ctx context.Context, userID int, bookIDs []int) (BulkReadingResponse, error) {
	var result BulkReadingResponse
	err := svc.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		result = BulkReadingResponse{}
		now := svc.now()
		for _, bookID := range distinct(bookIDs) {
			underway, err := retrieveUnderway(ctx, tx, userID, bookID)
			if err != nil {
				return err
			}
			if underway == nil {
				result.Skipped++
				continue
			}
			if err := deleteReadThrough(ctx, tx, underway, now); err != nil {
				return err
			}
			result.Changed++
		}
		return nil
	})
	return result, err
}

// distinct returns ids without repeats, in their first order.
func distinct(ids []int) []int {
	seen := make(map[int]bool, len(ids))
	result := make([]int, 0, len(ids))
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		result = append(result, id)
	}
	return result
}
