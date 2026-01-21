package lists

import (
	"context"
	"database/sql"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

type Service struct {
	db *bun.DB
}

func NewService(db *bun.DB) *Service {
	return &Service{db}
}

type CreateListOptions struct {
	UserID      int
	Name        string
	Description *string
	IsOrdered   bool
	DefaultSort string
}

func (svc *Service) CreateList(ctx context.Context, opts CreateListOptions) (*models.List, error) {
	now := time.Now()

	defaultSort := opts.DefaultSort
	if defaultSort == "" {
		if opts.IsOrdered {
			defaultSort = models.ListSortManual
		} else {
			defaultSort = models.ListSortAddedAtDesc
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

	_, err := svc.db.
		NewInsert().
		Model(list).
		Returning("*").
		Exec(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return list, nil
}

type RetrieveListOptions struct {
	ID *int
}

func (svc *Service) RetrieveList(ctx context.Context, opts RetrieveListOptions) (*models.List, error) {
	list := &models.List{}

	q := svc.db.
		NewSelect().
		Model(list).
		Relation("User")

	if opts.ID != nil {
		q = q.Where("l.id = ?", *opts.ID)
	}

	err := q.Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errcodes.NotFound("List")
		}
		return nil, errors.WithStack(err)
	}

	return list, nil
}

type ListListsOptions struct {
	UserID       int // Required - shows owned + shared lists
	Limit        *int
	Offset       *int
	includeTotal bool
}

func (svc *Service) ListLists(ctx context.Context, opts ListListsOptions) ([]*models.List, error) {
	lists, _, err := svc.listListsWithTotal(ctx, opts)
	return lists, errors.WithStack(err)
}

func (svc *Service) ListListsWithTotal(ctx context.Context, opts ListListsOptions) ([]*models.List, int, error) {
	opts.includeTotal = true
	return svc.listListsWithTotal(ctx, opts)
}

func (svc *Service) listListsWithTotal(ctx context.Context, opts ListListsOptions) ([]*models.List, int, error) {
	var lists []*models.List
	var total int
	var err error

	// Get lists owned by user OR shared with user
	q := svc.db.
		NewSelect().
		Model(&lists).
		Relation("User").
		Where("l.user_id = ? OR l.id IN (SELECT list_id FROM list_shares WHERE user_id = ?)", opts.UserID, opts.UserID).
		Order("l.name ASC")

	if opts.Limit != nil {
		q = q.Limit(*opts.Limit)
	}
	if opts.Offset != nil {
		q = q.Offset(*opts.Offset)
	}

	if opts.includeTotal {
		total, err = q.ScanAndCount(ctx)
	} else {
		err = q.Scan(ctx)
	}
	if err != nil {
		return nil, 0, errors.WithStack(err)
	}

	return lists, total, nil
}

type UpdateListOptions struct {
	Columns []string
}

func (svc *Service) UpdateList(ctx context.Context, list *models.List, opts UpdateListOptions) error {
	if len(opts.Columns) == 0 {
		return nil
	}

	list.UpdatedAt = time.Now()
	columns := append(opts.Columns, "updated_at")

	_, err := svc.db.
		NewUpdate().
		Model(list).
		Column(columns...).
		WherePK().
		Exec(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errcodes.NotFound("List")
		}
		return errors.WithStack(err)
	}
	return nil
}

func (svc *Service) DeleteList(ctx context.Context, listID int) error {
	_, err := svc.db.
		NewDelete().
		Model((*models.List)(nil)).
		Where("id = ?", listID).
		Exec(ctx)
	return errors.WithStack(err)
}

// GetListBookCount returns the number of books in a list visible to the user.
func (svc *Service) GetListBookCount(ctx context.Context, listID int, libraryIDs []int) (int, error) {
	q := svc.db.NewSelect().
		Model((*models.ListBook)(nil)).
		Join("JOIN books b ON b.id = lb.book_id").
		Where("lb.list_id = ?", listID)

	if libraryIDs != nil {
		q = q.Where("b.library_id IN (?)", bun.In(libraryIDs))
	}

	count, err := q.Count(ctx)
	return count, errors.WithStack(err)
}

type ListBooksOptions struct {
	ListID       int
	LibraryIDs   []int // Filter by user's accessible libraries (nil = all)
	Sort         string
	Limit        *int
	Offset       *int
	includeTotal bool
}

func (svc *Service) ListBooks(ctx context.Context, opts ListBooksOptions) ([]*models.ListBook, error) {
	books, _, err := svc.listBooksWithTotal(ctx, opts)
	return books, errors.WithStack(err)
}

func (svc *Service) ListBooksWithTotal(ctx context.Context, opts ListBooksOptions) ([]*models.ListBook, int, error) {
	opts.includeTotal = true
	return svc.listBooksWithTotal(ctx, opts)
}

func (svc *Service) listBooksWithTotal(ctx context.Context, opts ListBooksOptions) ([]*models.ListBook, int, error) {
	var listBooks []*models.ListBook
	var total int
	var err error

	q := svc.db.
		NewSelect().
		Model(&listBooks).
		Relation("Book").
		Relation("Book.Authors").
		Relation("Book.Authors.Person").
		Relation("Book.BookSeries").
		Relation("Book.BookSeries.Series").
		Relation("Book.Files").
		Relation("AddedByUser").
		Where("lb.list_id = ?", opts.ListID)

	// Filter by library access
	if opts.LibraryIDs != nil {
		q = q.Join("JOIN books b ON b.id = lb.book_id").
			Where("b.library_id IN (?)", bun.In(opts.LibraryIDs))
	}

	// Apply sort
	switch opts.Sort {
	case models.ListSortManual:
		q = q.Order("lb.sort_order ASC NULLS LAST", "lb.added_at DESC")
	case models.ListSortAddedAtAsc:
		q = q.Order("lb.added_at ASC")
	case models.ListSortTitleAsc:
		q = q.OrderExpr("(SELECT sort_title FROM books WHERE id = lb.book_id) ASC")
	case models.ListSortTitleDesc:
		q = q.OrderExpr("(SELECT sort_title FROM books WHERE id = lb.book_id) DESC")
	case models.ListSortAuthorAsc:
		q = q.OrderExpr("(SELECT p.sort_name FROM authors a JOIN people p ON p.id = a.person_id WHERE a.book_id = lb.book_id LIMIT 1) ASC NULLS LAST")
	case models.ListSortAuthorDesc:
		q = q.OrderExpr("(SELECT p.sort_name FROM authors a JOIN people p ON p.id = a.person_id WHERE a.book_id = lb.book_id LIMIT 1) DESC NULLS LAST")
	default: // added_at_desc
		q = q.Order("lb.added_at DESC")
	}

	if opts.Limit != nil {
		q = q.Limit(*opts.Limit)
	}
	if opts.Offset != nil {
		q = q.Offset(*opts.Offset)
	}

	if opts.includeTotal {
		total, err = q.ScanAndCount(ctx)
	} else {
		err = q.Scan(ctx)
	}
	if err != nil {
		return nil, 0, errors.WithStack(err)
	}

	return listBooks, total, nil
}

type AddBooksOptions struct {
	ListID        int
	BookIDs       []int
	AddedByUserID int
}

func (svc *Service) AddBooks(ctx context.Context, opts AddBooksOptions) error {
	if len(opts.BookIDs) == 0 {
		return nil
	}

	return svc.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		// Get the list to check if ordered
		list := &models.List{}
		err := tx.NewSelect().Model(list).Where("id = ?", opts.ListID).Scan(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// Get max sort_order if ordered list
		var maxSortOrder int
		if list.IsOrdered {
			err = tx.NewSelect().
				Model((*models.ListBook)(nil)).
				ColumnExpr("COALESCE(MAX(sort_order), 0)").
				Where("list_id = ?", opts.ListID).
				Scan(ctx, &maxSortOrder)
			if err != nil {
				return errors.WithStack(err)
			}
		}

		now := time.Now()
		listBooks := make([]*models.ListBook, 0, len(opts.BookIDs))

		for i, bookID := range opts.BookIDs {
			lb := &models.ListBook{
				ListID:        opts.ListID,
				BookID:        bookID,
				AddedAt:       now,
				AddedByUserID: &opts.AddedByUserID,
			}
			if list.IsOrdered {
				sortOrder := maxSortOrder + i + 1
				lb.SortOrder = &sortOrder
			}
			listBooks = append(listBooks, lb)
		}

		_, err = tx.NewInsert().
			Model(&listBooks).
			On("CONFLICT (list_id, book_id) DO NOTHING").
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// Update list's updated_at
		_, err = tx.NewUpdate().
			Model((*models.List)(nil)).
			Set("updated_at = ?", now).
			Where("id = ?", opts.ListID).
			Exec(ctx)
		return errors.WithStack(err)
	})
}

type RemoveBooksOptions struct {
	ListID  int
	BookIDs []int
}

func (svc *Service) RemoveBooks(ctx context.Context, opts RemoveBooksOptions) error {
	if len(opts.BookIDs) == 0 {
		return nil
	}

	return svc.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		_, err := tx.NewDelete().
			Model((*models.ListBook)(nil)).
			Where("list_id = ?", opts.ListID).
			Where("book_id IN (?)", bun.In(opts.BookIDs)).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// Update list's updated_at
		_, err = tx.NewUpdate().
			Model((*models.List)(nil)).
			Set("updated_at = ?", time.Now()).
			Where("id = ?", opts.ListID).
			Exec(ctx)
		return errors.WithStack(err)
	})
}

type ReorderBooksOptions struct {
	ListID  int
	BookIDs []int // New order - book IDs in desired sequence
}

func (svc *Service) ReorderBooks(ctx context.Context, opts ReorderBooksOptions) error {
	return svc.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		for i, bookID := range opts.BookIDs {
			sortOrder := i + 1
			_, err := tx.NewUpdate().
				Model((*models.ListBook)(nil)).
				Set("sort_order = ?", sortOrder).
				Where("list_id = ? AND book_id = ?", opts.ListID, bookID).
				Exec(ctx)
			if err != nil {
				return errors.WithStack(err)
			}
		}

		// Update list's updated_at
		_, err := tx.NewUpdate().
			Model((*models.List)(nil)).
			Set("updated_at = ?", time.Now()).
			Where("id = ?", opts.ListID).
			Exec(ctx)
		return errors.WithStack(err)
	})
}

// GetBookLists returns lists that contain a specific book (for the user).
func (svc *Service) GetBookLists(ctx context.Context, bookID, userID int) ([]*models.List, error) {
	var lists []*models.List

	err := svc.db.NewSelect().
		Model(&lists).
		Where("l.id IN (SELECT list_id FROM list_books WHERE book_id = ?)", bookID).
		Where("l.user_id = ? OR l.id IN (SELECT list_id FROM list_shares WHERE user_id = ?)", userID, userID).
		Order("l.name ASC").
		Scan(ctx)

	return lists, errors.WithStack(err)
}

// Permission check helpers

// getListOwnerID returns the owner ID for a list, or 0 and false if not found.
func (svc *Service) getListOwnerID(ctx context.Context, listID int) (ownerID int, found bool, err error) {
	err = svc.db.NewSelect().
		Model((*models.List)(nil)).
		Column("user_id").
		Where("id = ?", listID).
		Scan(ctx, &ownerID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, false, nil
		}
		return 0, false, errors.WithStack(err)
	}
	return ownerID, true, nil
}

// CanView returns true if the user can view the list.
func (svc *Service) CanView(ctx context.Context, listID, userID int) (bool, error) {
	ownerID, found, err := svc.getListOwnerID(ctx, listID)
	if err != nil || !found {
		return false, err
	}
	if ownerID == userID {
		return true, nil
	}

	// Check shares
	count, err := svc.db.NewSelect().
		Model((*models.ListShare)(nil)).
		Where("list_id = ? AND user_id = ?", listID, userID).
		Count(ctx)
	return count > 0, errors.WithStack(err)
}

// CanEdit returns true if the user can add/remove books.
func (svc *Service) CanEdit(ctx context.Context, listID, userID int) (bool, error) {
	ownerID, found, err := svc.getListOwnerID(ctx, listID)
	if err != nil || !found {
		return false, err
	}
	if ownerID == userID {
		return true, nil
	}

	// Check shares for editor or manager permission
	count, err := svc.db.NewSelect().
		Model((*models.ListShare)(nil)).
		Where("list_id = ? AND user_id = ?", listID, userID).
		Where("permission IN (?)", bun.In([]string{models.ListPermissionEditor, models.ListPermissionManager})).
		Count(ctx)
	return count > 0, errors.WithStack(err)
}

// CanManage returns true if the user can edit metadata and share.
func (svc *Service) CanManage(ctx context.Context, listID, userID int) (bool, error) {
	ownerID, found, err := svc.getListOwnerID(ctx, listID)
	if err != nil || !found {
		return false, err
	}
	if ownerID == userID {
		return true, nil
	}

	// Check shares for manager permission
	count, err := svc.db.NewSelect().
		Model((*models.ListShare)(nil)).
		Where("list_id = ? AND user_id = ? AND permission = ?", listID, userID, models.ListPermissionManager).
		Count(ctx)
	return count > 0, errors.WithStack(err)
}

// IsOwner returns true if the user owns the list.
func (svc *Service) IsOwner(ctx context.Context, listID, userID int) (bool, error) {
	ownerID, found, err := svc.getListOwnerID(ctx, listID)
	if err != nil || !found {
		return false, err
	}
	return ownerID == userID, nil
}

// Sharing operations

func (svc *Service) ListShares(ctx context.Context, listID int) ([]*models.ListShare, error) {
	var shares []*models.ListShare

	err := svc.db.NewSelect().
		Model(&shares).
		Relation("User").
		Relation("SharedByUser").
		Where("ls.list_id = ?", listID).
		Order("ls.created_at ASC").
		Scan(ctx)

	return shares, errors.WithStack(err)
}

type CreateShareOptions struct {
	ListID         int
	UserID         int
	Permission     string
	SharedByUserID int
}

func (svc *Service) CreateShare(ctx context.Context, opts CreateShareOptions) (*models.ListShare, error) {
	share := &models.ListShare{
		ListID:         opts.ListID,
		UserID:         opts.UserID,
		Permission:     opts.Permission,
		CreatedAt:      time.Now(),
		SharedByUserID: &opts.SharedByUserID,
	}

	_, err := svc.db.
		NewInsert().
		Model(share).
		Returning("*").
		Exec(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// Load relations
	err = svc.db.NewSelect().
		Model(share).
		Relation("User").
		Relation("SharedByUser").
		WherePK().
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return share, nil
}

func (svc *Service) UpdateShare(ctx context.Context, shareID int, permission string) error {
	_, err := svc.db.NewUpdate().
		Model((*models.ListShare)(nil)).
		Set("permission = ?", permission).
		Where("id = ?", shareID).
		Exec(ctx)
	return errors.WithStack(err)
}

func (svc *Service) DeleteShare(ctx context.Context, shareID int) error {
	_, err := svc.db.NewDelete().
		Model((*models.ListShare)(nil)).
		Where("id = ?", shareID).
		Exec(ctx)
	return errors.WithStack(err)
}

// CheckBookVisibility returns counts of visible/total books for a user.
func (svc *Service) CheckBookVisibility(ctx context.Context, listID int, targetUserLibraryIDs []int) (visible, total int, err error) {
	// Total books in list
	total, err = svc.db.NewSelect().
		Model((*models.ListBook)(nil)).
		Where("list_id = ?", listID).
		Count(ctx)
	if err != nil {
		return 0, 0, errors.WithStack(err)
	}

	// Visible books (filtered by target user's library access)
	q := svc.db.NewSelect().
		Model((*models.ListBook)(nil)).
		Join("JOIN books b ON b.id = lb.book_id").
		Where("lb.list_id = ?", listID)

	if targetUserLibraryIDs != nil {
		q = q.Where("b.library_id IN (?)", bun.In(targetUserLibraryIDs))
	}

	visible, err = q.Count(ctx)
	return visible, total, errors.WithStack(err)
}
