package koreaderplugin

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/identifiers"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/reach"
	"github.com/uptrace/bun"
)

const (
	// maxSuggestions is how many Files are suggested for an unrecognized
	// copy, and maxCandidates how many a search finds.
	maxSuggestions = 5
	maxCandidates  = 20
)

// linkScope is what a user reaches through a Device, worked out once per
// request: the Device's reach, and the libraries it covers.
type linkScope struct {
	user      *models.User
	reach     *reach.Reach
	libraries []int
}

// linkScope returns what user reaches through device, which is no library
// for a Device whose scope reaches nothing.
func (h *handler) linkScope(ctx context.Context, user *models.User, device *devices.Device) (*linkScope, error) {
	r, err := h.reach.Scope(ctx, user, device)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	scope := &linkScope{user: user, reach: r, libraries: r.LibraryIDs()}
	if scope.libraries == nil {
		scope.libraries = []int{}
		err = h.db.NewSelect().Model((*models.Library)(nil)).Column("l.id").OrderExpr("l.id").Scan(ctx, &scope.libraries)
	}
	return scope, errors.WithStack(err)
}

// suggest returns Files an unrecognized copy could be linked to: ones whose
// identifiers match the copy's first, then ones a search for its title and
// authors finds, all in scope.
func (h *handler) suggest(ctx context.Context, scope *linkScope, book *models.KOReaderUnrecognizedBook) ([]LinkCandidate, error) {
	byIdentifier, err := h.filesByIdentifier(ctx, scope, book.Identifiers)
	if err != nil {
		return nil, err
	}
	suggestions, err := h.candidates(ctx, byIdentifier, LinkMatchIdentifier)
	if err != nil {
		return nil, err
	}
	if len(suggestions) >= maxSuggestions || book.Title == nil {
		return capCandidates(suggestions, maxSuggestions), nil
	}
	query := *book.Title
	if len(book.Authors) > 0 {
		query += " " + book.Authors[0]
	}
	found, err := h.searchFiles(ctx, scope, query, maxSuggestions)
	if err != nil {
		return nil, err
	}
	if len(found) == 0 && len(book.Authors) > 0 {
		// An author named differently in the library still finds the title.
		found, err = h.searchFiles(ctx, scope, *book.Title, maxSuggestions)
		if err != nil {
			return nil, err
		}
	}
	bySearch, err := h.candidates(ctx, found, LinkMatchSearch)
	if err != nil {
		return nil, err
	}
	seen := map[int]bool{}
	for _, c := range suggestions {
		seen[c.FileID] = true
	}
	for _, c := range bySearch {
		if !seen[c.FileID] {
			suggestions = append(suggestions, c)
			seen[c.FileID] = true
		}
	}
	return capCandidates(suggestions, maxSuggestions), nil
}

func capCandidates(candidates []LinkCandidate, n int) []LinkCandidate {
	if len(candidates) > n {
		return candidates[:n]
	}
	return candidates
}

// reachable keeps the Files of files a copy can be linked to that are in
// scope, in order.
func (h *handler) reachable(ctx context.Context, scope *linkScope, files []*models.File) ([]*models.File, error) {
	out := []*models.File{}
	for _, file := range files {
		if !linkable(file) || !scope.user.HasLibraryAccess(file.LibraryID) {
			continue
		}
		holds, err := h.reach.HoldsBook(ctx, scope.reach, file.LibraryID, file.BookID)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		if holds {
			out = append(out, file)
		}
	}
	return out, nil
}

// identifierForms is every form a stored identifier matching one KOReader
// holds could take. KOReader keeps a scheme before some, as in
// "isbn:9780316769488" or "uuid:...".
func identifierForms(values []string) []string {
	seen := map[string]bool{}
	var forms []string
	add := func(value string) {
		for _, form := range identifiers.CandidateForms(value) {
			if !seen[form] {
				seen[form] = true
				forms = append(forms, form)
			}
		}
	}
	for _, value := range values {
		add(value)
		if _, rest, ok := strings.Cut(value, ":"); ok {
			add(rest)
		}
	}
	return forms
}

// filesByIdentifier returns the Files in scope whose identifiers match one
// of values.
func (h *handler) filesByIdentifier(ctx context.Context, scope *linkScope, values []string) ([]*models.File, error) {
	forms := identifierForms(values)
	if len(forms) == 0 || len(scope.libraries) == 0 {
		return nil, nil
	}
	var files []*models.File
	err := h.db.NewSelect().
		Model(&files).
		Where("f.id IN (SELECT fi.file_id FROM file_identifiers fi WHERE fi.value IN (?))", bun.List(forms)).
		Where("f.library_id IN (?)", bun.List(scope.libraries)).
		OrderExpr("f.id").
		Limit(maxCandidates).
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return h.reachable(ctx, scope, files)
}

// searchFiles returns up to maxCandidates Files of the Books a search for
// query finds in scope, best match first, searching limit Books in each
// library.
func (h *handler) searchFiles(ctx context.Context, scope *linkScope, query string, limit int) ([]*models.File, error) {
	var bookIDs []int
	for _, libraryID := range scope.libraries {
		results, _, err := h.search.SearchBooks(ctx, libraryID, query, nil, limit, 0)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		for _, result := range results {
			bookIDs = append(bookIDs, result.ID)
		}
	}
	if len(bookIDs) == 0 {
		return nil, nil
	}
	var files []*models.File
	err := h.db.NewSelect().Model(&files).Where("f.book_id IN (?)", bun.List(bookIDs)).OrderExpr("f.id").Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	// In the order the search ranked their Books.
	byBook := map[int][]*models.File{}
	for _, file := range files {
		byBook[file.BookID] = append(byBook[file.BookID], file)
	}
	var ranked []*models.File
	for _, id := range bookIDs {
		ranked = append(ranked, byBook[id]...)
		delete(byBook, id)
	}
	ranked, err = h.reachable(ctx, scope, ranked)
	if err != nil {
		return nil, err
	}
	return ranked[:min(len(ranked), maxCandidates)], nil
}

// candidates describes files as Files a copy could be linked to, found by
// match, which is empty for the File a copy is already linked to.
func (h *handler) candidates(ctx context.Context, files []*models.File, match string) ([]LinkCandidate, error) {
	out := []LinkCandidate{}
	if len(files) == 0 {
		return out, nil
	}
	bookIDs := make([]int, 0, len(files))
	for _, file := range files {
		bookIDs = append(bookIDs, file.BookID)
	}
	var books []*models.Book
	err := h.db.NewSelect().
		Model(&books).
		Relation("Library").
		Relation("Authors", func(sq *bun.SelectQuery) *bun.SelectQuery { return sq.Order("a.sort_order ASC") }).
		Relation("Authors.Person").
		Where("b.id IN (?)", bun.List(bookIDs)).
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	byID := map[int]*models.Book{}
	for _, book := range books {
		byID[book.ID] = book
	}
	for _, file := range files {
		book := byID[file.BookID]
		if book == nil {
			continue
		}
		candidate := LinkCandidate{
			FileID: file.ID, BookID: book.ID, LibraryID: book.LibraryID, Title: file.ResolveTitle(book.Title),
			Authors: []string{}, FileType: file.FileType, Match: match,
		}
		if book.Library != nil {
			candidate.LibraryName = book.Library.Name
		}
		for _, author := range book.Authors {
			if author.Person != nil {
				candidate.Authors = append(candidate.Authors, author.Person.Name)
			}
		}
		out = append(out, candidate)
	}
	return out, nil
}
