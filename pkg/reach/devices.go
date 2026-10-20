package reach

import (
	"context"
	"slices"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/delivery"
	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/kobo"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// What each of a user's Devices gets, for the book page and a Device's
// settings: the same scope and Delivery Rules the channels use, applied
// with delivery.Resolve, and, on a Kobo, what its next sync would do to the
// books already on it (kobo.Service.PlanChanges).

// BookDevices returns what each of user's Devices gets of book, which must
// be loaded with its Files. The caller checks the user's library access.
func (s *Service) BookDevices(ctx context.Context, user *models.User, book *models.Book) ([]BookDevice, error) {
	list, err := s.deviceService.List(ctx, user.ID)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	rules, err := s.deviceService.RulesFor(ctx, list)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	result := make([]BookDevice, 0, len(list))
	for _, device := range list {
		line := BookDevice{
			DeviceID:     device.ID,
			DeviceName:   device.Name,
			DeviceType:   device.DeviceType,
			Deliveries:   []BookDelivery{},
			EditionMoves: []EditionMove{},
		}
		r, err := s.Scope(ctx, user, device)
		if err != nil {
			return nil, err
		}
		holds, err := s.HoldsBook(ctx, r, book.LibraryID, book.ID)
		if err != nil {
			return nil, err
		}
		if !holds {
			line.NotDeliveredReason = NotDeliveredOutsideScope
			result = append(result, line)
			continue
		}

		var delivered []kobo.DeliveredFile
		for _, d := range delivery.Resolve(rules[device.ID], book).All() {
			line.Deliveries = append(line.Deliveries, BookDelivery{FileID: d.File.ID, SourceFileType: d.File.FileType, DeliveredFileType: d.DeliveredFileType})
			delivered = append(delivered, kobo.DeliveredFile{FileID: d.File.ID, BookID: book.ID, DeliveredFileType: d.DeliveredFileType})
		}
		if len(line.Deliveries) == 0 {
			line.NotDeliveredReason = NotDeliveredNoRule
		}
		if device.DeviceType == devices.DeviceTypeKobo {
			changes, err := s.koboService.PlanChanges(ctx, device.ID, delivered, []int{book.ID})
			if err != nil {
				return nil, errors.WithStack(err)
			}
			for _, move := range changes.Moves {
				line.EditionMoves = append(line.EditionMoves, EditionMove{FromFileID: move.FromFileID, ToFileID: move.ToFileID})
			}
			// PlanChanges records at most one switch per book.
			if len(changes.TypeSwitches) > 0 {
				sw := changes.TypeSwitches[0]
				line.TypeSwitch = &TypeSwitch{FromFileType: sw.From, ToFileType: sw.To}
			}
		}
		result = append(result, line)
	}
	return result, nil
}

// Preview returns what device would receive within its scope under rules:
// how many books it gets something of, how many it gets nothing of by File
// Type, and, on a Kobo, how many books already on it would move to another
// edition or File Type.
func (s *Service) Preview(ctx context.Context, user *models.User, device *devices.Device, rules []delivery.Rule) (*DeliveryPreviewResponse, error) {
	r, err := s.Scope(ctx, user, device)
	if err != nil {
		return nil, err
	}
	files, err := s.mainFiles(ctx, r)
	if err != nil {
		return nil, err
	}

	preview := &DeliveryPreviewResponse{NotReceived: []NotReceivedCount{}}
	notReceived := map[string]int{}
	var delivered []kobo.DeliveredFile
	for _, book := range booksOf(files) {
		deliveries := delivery.Resolve(rules, book).All()
		if len(deliveries) == 0 {
			var types []string
			for _, f := range book.Files {
				if !slices.Contains(types, f.FileType) {
					types = append(types, f.FileType)
					notReceived[f.FileType]++
				}
			}
			continue
		}
		preview.ReceivedBooks++
		for _, d := range deliveries {
			delivered = append(delivered, kobo.DeliveredFile{FileID: d.File.ID, BookID: book.ID, DeliveredFileType: d.DeliveredFileType})
		}
	}
	// Built-in File Types in the built-in rules' order, then any other, such
	// as a plugin's, by name.
	var order []string
	for _, builtIn := range delivery.BuiltInRules() {
		order = append(order, builtIn.SourceFileType)
	}
	var others []string
	for fileType := range notReceived {
		if !slices.Contains(order, fileType) {
			others = append(others, fileType)
		}
	}
	slices.Sort(others)
	for _, fileType := range append(order, others...) {
		if n := notReceived[fileType]; n > 0 {
			preview.NotReceived = append(preview.NotReceived, NotReceivedCount{SourceFileType: fileType, Books: n})
		}
	}

	if device.DeviceType == devices.DeviceTypeKobo {
		changes, err := s.koboService.PlanChanges(ctx, device.ID, delivered, nil)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		movedBooks := map[int]bool{}
		for _, move := range changes.Moves {
			movedBooks[move.BookID] = true
		}
		preview.MovedBooks = len(movedBooks)
		preview.RetypedBooks = len(changes.TypeSwitches)
	}
	return preview, nil
}

// mainFiles returns the main Files of every book in r, ordered by book. Its
// scope matches kobo.Service.scopedFilesQuery, which Kobo sync uses, so a
// Kobo's preview counts what it syncs; change both together.
func (s *Service) mainFiles(ctx context.Context, r *Reach) ([]*models.File, error) {
	files := []*models.File{}
	if r.closed {
		return files, nil
	}
	q := s.db.NewSelect().
		Model(&files).
		Column("f.id", "f.book_id", "f.file_type", "f.file_role").
		Where("f.file_role = ?", models.FileRoleMain).
		Order("f.book_id ASC", "f.id ASC")
	if ids := r.LibraryIDs(); ids != nil {
		if len(ids) == 0 {
			return files, nil
		}
		q = q.Where("f.library_id IN (?)", bun.List(ids))
	}
	if r.listID != nil {
		q = q.Join("JOIN list_books AS lb ON lb.book_id = f.book_id").Where("lb.list_id = ?", *r.listID)
	}
	if err := q.Scan(ctx); err != nil {
		return nil, errors.WithStack(err)
	}
	return files, nil
}

// booksOf groups files, ordered by book, into books holding them.
func booksOf(files []*models.File) []*models.Book {
	var books []*models.Book
	for _, f := range files {
		if len(books) == 0 || books[len(books)-1].ID != f.BookID {
			books = append(books, &models.Book{ID: f.BookID})
		}
		last := books[len(books)-1]
		last.Files = append(last.Files, f)
	}
	return books
}
