package reach

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/delivery"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bookFiles adds a book to lib with a main File of each of mainTypes and a
// supplement of each of supplementTypes, and returns it with its Files set,
// as delivery.Resolve reads it.
func (f *fixture) bookFiles(lib *models.Library, title string, mainTypes []string, supplementTypes ...string) *models.Book {
	f.t.Helper()
	ctx := context.Background()
	b := &models.Book{LibraryID: lib.ID, Title: title, TitleSource: models.DataSourceFilepath, SortTitle: title, SortTitleSource: models.DataSourceFilepath, AuthorSource: models.DataSourceFilepath, Filepath: "/" + title}
	_, err := f.db.NewInsert().Model(b).Exec(ctx)
	require.NoError(f.t, err)
	add := func(fileType, role string) {
		file := &models.File{LibraryID: lib.ID, BookID: b.ID, FileType: fileType, FileRole: role, Filepath: fmt.Sprintf("%s/%s.%s", b.Filepath, role, fileType), FilesizeBytes: 1}
		_, err := f.db.NewInsert().Model(file).Exec(ctx)
		require.NoError(f.t, err)
		b.Files = append(b.Files, file)
	}
	for _, fileType := range mainTypes {
		add(fileType, models.FileRoleMain)
	}
	for _, fileType := range supplementTypes {
		add(fileType, models.FileRoleSupplement)
	}
	return b
}

func deliversAs(rules []delivery.Rule, book *models.Book, fileType string) bool {
	return slices.ContainsFunc(delivery.Resolve(rules, book).All(), func(d delivery.Delivery) bool {
		return d.DeliveredFileType == fileType
	})
}

// The type filter's SQL picks exactly the books delivery.Resolve delivers
// as the chosen File Type, for every File Type and every list of rules, and
// the filter offers exactly the File Types some book is delivered as, in
// rule order.
func TestDeliveredAs_MatchesResolve(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	lib := f.library("Books")
	all := []*models.Book{
		f.bookFiles(lib, "KePub only", []string{models.FileTypeKePub}),
		f.bookFiles(lib, "EPUB only", []string{models.FileTypeEPUB}),
		f.bookFiles(lib, "EPUB and CBZ", []string{models.FileTypeEPUB, models.FileTypeCBZ}),
		f.bookFiles(lib, "CBZ only", []string{models.FileTypeCBZ}),
		f.bookFiles(lib, "KePub and EPUB", []string{models.FileTypeKePub, models.FileTypeEPUB}),
		f.bookFiles(lib, "EPUB and M4B", []string{models.FileTypeEPUB, models.FileTypeM4B}),
		f.bookFiles(lib, "M4B only", []string{models.FileTypeM4B}),
		f.bookFiles(lib, "MOBI and AZW3", []string{models.FileTypeMOBI, models.FileTypeAZW3}),
		f.bookFiles(lib, "PDF only", []string{models.FileTypePDF}),
		f.bookFiles(lib, "EPUB with PDF supplement", []string{models.FileTypeEPUB}, models.FileTypePDF),
		f.bookFiles(lib, "CBZ with EPUB supplement", []string{models.FileTypeCBZ}, models.FileTypeEPUB),
		f.bookFiles(lib, "Supplement only", nil, models.FileTypeEPUB),
	}

	ruleLists := map[string][]delivery.Rule{
		"built-in": delivery.BuiltInRules(),
		"cbz only": cbzOnly,
		"kobo-like": {
			{SourceFileType: models.FileTypeKePub, DeliveredFileType: models.FileTypeKePub},
			{SourceFileType: models.FileTypeEPUB, DeliveredFileType: models.FileTypeKePub},
			{SourceFileType: models.FileTypeCBZ, DeliveredFileType: models.FileTypeKePub},
			{SourceFileType: models.FileTypeM4B, DeliveredFileType: models.FileTypeM4B},
		},
		"comics first, as EPUB": {
			{SourceFileType: models.FileTypeCBZ, DeliveredFileType: models.FileTypeEPUB},
			{SourceFileType: models.FileTypeEPUB, DeliveredFileType: models.FileTypeEPUB},
			{SourceFileType: models.FileTypePDF, DeliveredFileType: models.FileTypePDF},
		},
		// Rules with no chain count as unmatched, so the EPUB rule after
		// them still wins for a book that also has a PDF or an M4B.
		"no chain first": {
			{SourceFileType: models.FileTypePDF, DeliveredFileType: models.FileTypeEPUB},
			{SourceFileType: models.FileTypeM4B, DeliveredFileType: models.FileTypeEPUB},
			{SourceFileType: models.FileTypeEPUB, DeliveredFileType: models.FileTypeEPUB},
			{SourceFileType: models.FileTypeMOBI, DeliveredFileType: models.FileTypeMOBI},
			{SourceFileType: models.FileTypeAZW3, DeliveredFileType: models.FileTypeAZW3},
		},
		"nothing": {},
	}

	for name, rules := range ruleLists {
		t.Run(name, func(t *testing.T) {
			wantTypes := []string{}
			for _, rule := range rules {
				fileType := rule.DeliveredFileType
				if slices.Contains(wantTypes, fileType) {
					continue
				}
				if slices.ContainsFunc(all, func(b *models.Book) bool { return deliversAs(rules, b, fileType) }) {
					wantTypes = append(wantTypes, fileType)
				}
			}
			r := &Reach{User: f.user, Rules: rules}
			gotTypes, err := f.svc.DeliveredFileTypesWithBooks(ctx, r, books.ListBooksOptions{LibraryID: &lib.ID})
			require.NoError(t, err)
			assert.Equal(t, wantTypes, gotTypes)

			for _, fileType := range models.BuiltInFileTypes {
				want := []string{}
				for _, b := range all {
					if deliversAs(rules, b, fileType) {
						want = append(want, b.Title)
					}
				}
				got, _, err := f.svc.bookService.ListBooksWithTotal(ctx, books.ListBooksOptions{
					LibraryID:       &lib.ID,
					MainFileMatches: DeliveredAs(rules, fileType),
				})
				require.NoError(t, err)
				titles := []string{}
				for _, b := range got {
					titles = append(titles, b.Title)
				}
				assert.ElementsMatch(t, want, titles, "delivered as %s", fileType)
			}
		})
	}
}

// Under the built-in rules, a KePub-only book is offered and filtered as
// KePub, and a book with an EPUB and a CBZ is not under CBZ, since only its
// EPUB is sent.
func TestDeliveredAs_BuiltInRules(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	lib := f.library("Books")
	f.bookFiles(lib, "KePub only", []string{models.FileTypeKePub})
	f.bookFiles(lib, "EPUB and CBZ", []string{models.FileTypeEPUB, models.FileTypeCBZ})

	r := &Reach{User: f.user, Rules: delivery.BuiltInRules()}
	types, err := f.svc.DeliveredFileTypesWithBooks(ctx, r, books.ListBooksOptions{LibraryID: &lib.ID})
	require.NoError(t, err)
	assert.Equal(t, []string{models.FileTypeEPUB, models.FileTypeKePub}, types)

	has, err := f.svc.HasBooks(ctx, r, books.ListBooksOptions{LibraryID: &lib.ID, MainFileMatches: DeliveredAs(r.Rules, models.FileTypeKePub)})
	require.NoError(t, err)
	assert.True(t, has, "the KePub is sent as itself")
	has, err = f.svc.HasBooks(ctx, r, books.ListBooksOptions{LibraryID: &lib.ID, MainFileMatches: DeliveredAs(r.Rules, models.FileTypeCBZ)})
	require.NoError(t, err)
	assert.False(t, has, "nothing is sent as CBZ")
}
