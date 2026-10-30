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
	"github.com/shishobooks/shisho/pkg/kobo"
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
// deliver are left out. A Reach from Scope, with no Rules loaded, returns
// nil, which filters nothing; rules that deliver nothing return an empty
// list, which matches no book.
func (r *Reach) SourceFileTypes() []string {
	if r.Rules == nil {
		return nil
	}
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
	koboService     *kobo.Service
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
		koboService:     kobo.NewService(db),
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

// ReaderID is the id of r's user, whose reading data the reading sorts and
// filters read, or nil without one.
func (r *Reach) ReaderID() *int {
	if r.User == nil {
		return nil
	}
	return &r.User.ID
}

// narrow returns opts narrowed to what r reaches: its rules' source File
// Types, its list, and its libraries. Its user reads the reading sorts. It returns false when r reaches
// nothing opts could match, since the books filters read an empty library
// list as no filter.
func (r *Reach) narrow(opts books.ListBooksOptions) (books.ListBooksOptions, bool) {
	if r.closed {
		return opts, false
	}
	opts.MainFileTypes = r.SourceFileTypes()
	opts.ListID = r.listID
	opts.ReaderID = r.ReaderID()
	if opts.LibraryID != nil {
		return opts, r.HoldsLibrary(*opts.LibraryID)
	}
	ids := r.LibraryIDs()
	if ids != nil && len(ids) == 0 {
		return opts, false
	}
	opts.LibraryIDs = ids
	return opts, true
}

// HasBooks reports whether opts, narrowed to what r reaches, matches a book
// with something to deliver.
func (s *Service) HasBooks(ctx context.Context, r *Reach, opts books.ListBooksOptions) (bool, error) {
	opts, ok := r.narrow(opts)
	if !ok {
		return false, nil
	}
	has, err := s.bookService.HasBooks(ctx, opts)
	return has, errors.WithStack(err)
}

// ListBooks returns a page of the books opts matches, narrowed to what r
// reaches, and how many there are in all.
func (s *Service) ListBooks(ctx context.Context, r *Reach, opts books.ListBooksOptions) ([]*models.Book, int, error) {
	opts, ok := r.narrow(opts)
	if !ok {
		return []*models.Book{}, 0, nil
	}
	result, total, err := s.bookService.ListBooksWithTotal(ctx, opts)
	return result, total, errors.WithStack(err)
}

// Libraries returns the libraries in reach that have a book with something
// to deliver, so a channel never lists one that opens empty. One query
// finds them, however many libraries there are.
func (s *Service) Libraries(ctx context.Context, r *Reach) ([]*models.Library, error) {
	opts, ok := r.narrow(books.ListBooksOptions{})
	if !ok {
		return []*models.Library{}, nil
	}
	ids, err := s.bookService.LibraryIDsWithBooks(ctx, opts)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	libs, err := s.libraryService.ListLibraries(ctx, libraries.ListLibrariesOptions{LibraryIDs: ids})
	return libs, errors.WithStack(err)
}

// DeliveredAs returns the book filter matching the books the rules deliver
// some File of as fileType (delivery.DeliveredAs).
func DeliveredAs(rules []delivery.Rule, fileType string) []books.MainFileMatch {
	matches := []books.MainFileMatch{}
	for _, m := range delivery.DeliveredAs(rules, fileType) {
		matches = append(matches, books.MainFileMatch{FileType: m.SourceFileType, Unless: m.Unless})
	}
	return matches
}

// DeliveredFileTypesWithBooks returns the File Types, in rule order, that
// the rules deliver some book opts matches in reach as, so a type filter
// offers only types that match something. It runs one query per distinct
// delivered File Type, a handful at most, rather than one query that
// resolves every book.
func (s *Service) DeliveredFileTypesWithBooks(ctx context.Context, r *Reach, opts books.ListBooksOptions) ([]string, error) {
	types := []string{}
	var checked []string
	for _, rule := range r.Rules {
		fileType := rule.DeliveredFileType
		if slices.Contains(checked, fileType) {
			continue
		}
		checked = append(checked, fileType)
		opts.MainFileMatches = DeliveredAs(r.Rules, fileType)
		if len(opts.MainFileMatches) == 0 {
			continue
		}
		has, err := s.HasBooks(ctx, r, opts)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		if has {
			types = append(types, fileType)
		}
	}
	return types, nil
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
