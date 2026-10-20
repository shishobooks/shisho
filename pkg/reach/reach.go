// Package reach decides what a reading app reaches through the channels that
// browse a library, OPDS and the eReader browser: the Delivery Rules that
// pick each book's Files, the Device's scope, and which entries (libraries,
// a library's series and authors, a list) have anything to deliver, so both
// channels show the same books and leave out the same empty entries.
package reach

import (
	"context"
	"slices"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/delivery"
	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/libraries"
	"github.com/shishobooks/shisho/pkg/lists"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/people"
	"github.com/shishobooks/shisho/pkg/series"
	"github.com/uptrace/bun"
)

// Reach is what one request reaches. Without a Device (OPDS signed in with
// the account password) that is every library the user can access,
// delivered by the user's default Delivery Rules. With a Device, it is the
// Device's rules and scope: all the owner's books, one library, or the books
// on one list. A scope whose library or list is gone, or that the owner can
// no longer see, reaches nothing, as on Kobo sync.
type Reach struct {
	User *models.User
	// Device is nil for a request signed in without one.
	Device *devices.Device
	// Rules are the Delivery Rules the request follows. Scope leaves them
	// nil; Resolve loads them.
	Rules []delivery.Rule

	libraryID     *int
	listID        *int
	libraryScoped bool
	closed        bool
}

// LibraryIDs returns the libraries books can come from: nil for every
// library, and empty for none.
func (r *Reach) LibraryIDs() []int {
	switch {
	case r.closed:
		return []int{}
	case r.libraryID != nil:
		return []int{*r.libraryID}
	}
	return r.User.GetAccessibleLibraryIDs()
}

// ListID returns the one list a list-scoped Device reaches, or nil.
func (r *Reach) ListID() *int {
	return r.listID
}

// LibraryScoped reports whether the Device is scoped to one library, even
// one that is gone.
func (r *Reach) LibraryScoped() bool {
	return r.libraryScoped
}

// Closed reports whether the scope reaches nothing.
func (r *Reach) Closed() bool {
	return r.closed
}

// HoldsLibrary reports whether books from libraryID can be in reach. A
// list-scoped Device holds every library the owner can access, but only the
// books on its list.
func (r *Reach) HoldsLibrary(libraryID int) bool {
	ids := r.LibraryIDs()
	return ids == nil || slices.Contains(ids, libraryID)
}

// SourceFileTypes returns the source File Types the rules deliver, the
// filter listings apply in SQL before paging so books with nothing to
// deliver are left out.
func (r *Reach) SourceFileTypes() []string {
	return delivery.SourceFileTypes(r.Rules)
}

// Service resolves Reaches and answers what is in them.
type Service struct {
	db              *bun.DB
	deviceService   *devices.Service
	deliveryService *delivery.Service
	listService     *lists.Service
	libraryService  *libraries.Service
	bookService     *books.Service
	seriesService   *series.Service
	peopleService   *people.Service
}

// NewService returns a Service. bookService is the shared books service.
func NewService(db *bun.DB, bookService *books.Service) *Service {
	return &Service{
		db:              db,
		deviceService:   devices.NewService(db),
		deliveryService: delivery.NewService(db),
		listService:     lists.NewService(db),
		libraryService:  libraries.NewService(db),
		bookService:     bookService,
		seriesService:   series.NewService(db),
		peopleService:   people.NewService(db),
	}
}

// Scope returns what user reaches through device (nil for no Device),
// without loading Delivery Rules, for routes that serve one book.
func (s *Service) Scope(ctx context.Context, user *models.User, device *devices.Device) (*Reach, error) {
	r := &Reach{User: user, Device: device}
	if device == nil {
		return r, nil
	}
	scope := device.Scope()
	switch scope.Type {
	case devices.ScopeLibrary:
		r.libraryScoped = true
		if scope.LibraryID == nil || !user.HasLibraryAccess(*scope.LibraryID) {
			r.closed = true
		} else {
			r.libraryID = scope.LibraryID
		}
	case devices.ScopeList:
		if scope.ListID == nil {
			r.closed = true
			break
		}
		canView, err := s.listService.CanView(ctx, *scope.ListID, user.ID)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		if canView {
			r.listID = scope.ListID
		} else {
			r.closed = true
		}
	}
	return r, nil
}

// Resolve returns what user reaches through device (nil for no Device),
// with the Delivery Rules it follows: the Device's, or the user's default.
func (s *Service) Resolve(ctx context.Context, user *models.User, device *devices.Device) (*Reach, error) {
	r, err := s.Scope(ctx, user, device)
	if err != nil {
		return nil, err
	}
	if device != nil {
		r.Rules, err = s.deviceService.Rules(ctx, device)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		return r, nil
	}
	userDefault, err := s.deliveryService.UserDefault(ctx, user.ID)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	r.Rules = userDefault.Rules
	return r, nil
}

// HoldsBook reports whether bookID, in libraryID, is in reach. The caller
// checks the user's own library access first.
func (s *Service) HoldsBook(ctx context.Context, r *Reach, libraryID, bookID int) (bool, error) {
	if !r.HoldsLibrary(libraryID) {
		return false, nil
	}
	if r.listID == nil {
		return true, nil
	}
	onList, err := s.listService.ContainsBook(ctx, *r.listID, bookID)
	return onList, errors.WithStack(err)
}

// HasBooks reports whether opts, narrowed to what r reaches, matches a book
// with something to deliver. A Reach without Rules counts every book.
func (s *Service) HasBooks(ctx context.Context, r *Reach, opts books.ListBooksOptions) (bool, error) {
	if r.closed {
		return false, nil
	}
	if r.Rules != nil {
		opts.MainFileTypes = r.SourceFileTypes()
	}
	opts.ListID = r.listID
	if opts.LibraryID == nil {
		// The books filter reads an empty list as no filter, so a user who
		// can access no library has nothing to check.
		ids := r.LibraryIDs()
		if ids != nil && len(ids) == 0 {
			return false, nil
		}
		opts.LibraryIDs = ids
	} else if !r.HoldsLibrary(*opts.LibraryID) {
		return false, nil
	}
	limit := 1
	opts.Limit = &limit
	_, total, err := s.bookService.ListBooksWithTotal(ctx, opts)
	if err != nil {
		return false, errors.WithStack(err)
	}
	return total > 0, nil
}

// Libraries returns the libraries in reach that have a book with something
// to deliver, so a channel never lists one that opens empty.
func (s *Service) Libraries(ctx context.Context, r *Reach) ([]*models.Library, error) {
	all, err := s.libraryService.ListLibraries(ctx, libraries.ListLibrariesOptions{LibraryIDs: r.LibraryIDs()})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var libs []*models.Library
	for _, lib := range all {
		has, err := s.HasBooks(ctx, r, books.ListBooksOptions{LibraryID: &lib.ID})
		if err != nil {
			return nil, err
		}
		if has {
			libs = append(libs, lib)
		}
	}
	return libs, nil
}

// LibrarySections says which of a library's browsing sections have
// something to deliver.
type LibrarySections struct {
	Series  bool
	Authors bool
}

// Sections returns which of libraryID's series and authors sections have
// something in reach to deliver, so a channel leaves out a section that
// would open empty.
func (s *Service) Sections(ctx context.Context, r *Reach, libraryID int) (LibrarySections, error) {
	if r.closed {
		return LibrarySections{}, nil
	}
	fileTypes := r.SourceFileTypes()
	limit := 1
	_, seriesTotal, err := s.seriesService.ListSeriesWithTotal(ctx, series.ListSeriesOptions{
		LibraryID:     &libraryID,
		ListID:        r.listID,
		MainFileTypes: fileTypes,
		Limit:         &limit,
	})
	if err != nil {
		return LibrarySections{}, errors.WithStack(err)
	}
	_, authorsTotal, err := s.peopleService.ListPeopleWithTotal(ctx, people.ListPeopleOptions{
		LibraryID:     &libraryID,
		ListID:        r.listID,
		MainFileTypes: fileTypes,
		Limit:         &limit,
	})
	if err != nil {
		return LibrarySections{}, errors.WithStack(err)
	}
	return LibrarySections{Series: seriesTotal > 0, Authors: authorsTotal > 0}, nil
}

// Deliverable returns the filter that keeps lists holding a book in reach
// with something to deliver.
func (r *Reach) Deliverable() *lists.DeliverableBooks {
	return &lists.DeliverableBooks{LibraryIDs: r.LibraryIDs(), MainFileTypes: r.SourceFileTypes()}
}
