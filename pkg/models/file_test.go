package models

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFile_ResolveDisplayName(t *testing.T) {
	t.Parallel()

	str := func(s string) *string { return &s }
	tests := []struct {
		name string
		file File
		want string
	}{
		{
			name: "main file uses its name",
			file: File{FileRole: FileRoleMain, Name: str("Edition Title"), Filepath: "/lib/Book/book.epub"},
			want: "Edition Title",
		},
		{
			// Matches ResolveTitle: a blank name is not a label.
			name: "main file with a whitespace-only name falls back to the filename",
			file: File{FileRole: FileRoleMain, Name: str("  "), Filepath: "/lib/Book/book.epub"},
			want: "book.epub",
		},
		{
			name: "supplement with a whitespace-only manual name shows its filename",
			file: File{
				FileRole:   FileRoleSupplement,
				Name:       str(" "),
				NameSource: str(DataSourceManual),
				Filepath:   "/lib/Book/maps.pdf",
			},
			want: "maps.pdf",
		},
		{
			name: "main file without a name falls back to the filename",
			file: File{FileRole: FileRoleMain, Filepath: "/lib/Book/book.epub"},
			want: "book.epub",
		},
		{
			name: "supplement shows its filename, not the name the scanner stored",
			file: File{
				FileRole:   FileRoleSupplement,
				Name:       str("Original Title"),
				NameSource: str(DataSourceFilepath),
				Filepath:   "/lib/The Lighthouse Keeper/The Lighthouse Keeper.pdf",
			},
			want: "The Lighthouse Keeper.pdf",
		},
		{
			name: "supplement keeps a name the user set",
			file: File{
				FileRole:   FileRoleSupplement,
				Name:       str("Maps and Charts"),
				NameSource: str(DataSourceManual),
				Filepath:   "/lib/Book/maps.pdf",
			},
			want: "Maps and Charts",
		},
		{
			// A sidecar name is not proof the user set it: editing the book
			// writes every file's sidecar, supplements included, with the
			// scanner-stored stem, and a rescan restores that stem with
			// source sidecar. Trusting it would bring back the pre-rename
			// title this label exists to avoid.
			name: "supplement shows its filename over a sidecar-sourced name",
			file: File{
				FileRole:   FileRoleSupplement,
				Name:       str("Original Title"),
				NameSource: str(DataSourceSidecar),
				Filepath:   "/lib/The Lighthouse Keeper/The Lighthouse Keeper.pdf",
			},
			want: "The Lighthouse Keeper.pdf",
		},
		{
			name: "no name and no path gives an empty name",
			file: File{FileRole: FileRoleSupplement},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.file.ResolveDisplayName())
		})
	}
}

func TestFile_ResolveTitle(t *testing.T) {
	t.Parallel()

	str := func(s string) *string { return &s }
	tests := []struct {
		name string
		file *File
		want string
	}{
		{
			name: "a named file uses its name",
			file: &File{Name: str("Harry Potter and the Sorcerer's Stone")},
			want: "Harry Potter and the Sorcerer's Stone",
		},
		{
			name: "a file without a name uses the book's title",
			file: &File{},
			want: "Harry Potter and the Philosopher's Stone",
		},
		{
			name: "an empty name uses the book's title",
			file: &File{Name: str("")},
			want: "Harry Potter and the Philosopher's Stone",
		},
		{
			// Nothing a reader could recognize, so it does not count as set.
			name: "a whitespace-only name uses the book's title",
			file: &File{Name: str(" \t ")},
			want: "Harry Potter and the Philosopher's Stone",
		},
		{
			// Unlike ResolveDisplayName, the filename is never the title.
			name: "a supplement without a name uses the book's title, not its filename",
			file: &File{FileRole: FileRoleSupplement, Filepath: "/lib/Book/notes.pdf"},
			want: "Harry Potter and the Philosopher's Stone",
		},
		{
			name: "a nil file uses the book's title",
			file: nil,
			want: "Harry Potter and the Philosopher's Stone",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.file.ResolveTitle("Harry Potter and the Philosopher's Stone"))
		})
	}
}

func TestFile_AfterScanRowResolvesDisplayName(t *testing.T) {
	t.Parallel()

	name := "Original Title"
	file := &File{FileRole: FileRoleSupplement, Name: &name, Filepath: "/lib/Book/notes.pdf"}
	require.NoError(t, file.AfterScanRow(context.Background()))
	assert.Equal(t, "notes.pdf", file.DisplayName)
	assert.Equal(t, "Original Title", *file.Name, "the stored name is left alone")
}

func TestResolveFileDisplayNames(t *testing.T) {
	t.Parallel()

	books := []*Book{{Files: []*File{{FileRole: FileRoleSupplement, Filepath: "/lib/Book/a.pdf"}, nil}}, nil}
	ResolveBookFileDisplayNames(books...)
	assert.Equal(t, "a.pdf", books[0].Files[0].DisplayName)
}

func TestFileTypeForPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path string
		want string
	}{
		{"/lib/Book/book.epub", FileTypeEPUB},
		{"/lib/Book/book.kepub.epub", FileTypeKePub},
		{"/lib/Book/Book.KEPUB.EPUB", FileTypeKePub},
		{"/lib/Book/book.kepub", "kepub"},
		{"/lib/Book.kepub/book.epub", FileTypeEPUB},
		{"/lib/Book/book.mobi", FileTypeMOBI},
		{"/lib/Book/book.azw", FileTypeMOBI},
		{"/lib/Book/book.PRC", FileTypeMOBI},
		{"/lib/Book/book.azw3", FileTypeAZW3},
		{"/lib/Book/book.AZW3", FileTypeAZW3},
		{"/lib/Book/book.cbz", FileTypeCBZ},
		{"/lib/Book/notes.TXT", "txt"},
		{"/lib/Book/book.fb2", "fb2"},
		{"/lib/Book/README", ""},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, FileTypeForPath(tt.path), tt.path)
	}
}

func TestIsBuiltInFileType(t *testing.T) {
	t.Parallel()

	for _, ft := range []string{FileTypeEPUB, FileTypeKePub, FileTypeCBZ, FileTypeM4B, FileTypePDF, FileTypeMOBI, FileTypeAZW3} {
		assert.True(t, IsBuiltInFileType(ft), ft)
	}
	for _, ft := range []string{"azw", "prc", "txt", "fb2", ""} {
		assert.False(t, IsBuiltInFileType(ft), ft)
	}
}

func TestIsEbookFileType(t *testing.T) {
	t.Parallel()

	for _, ft := range []string{FileTypeEPUB, FileTypeKePub, FileTypeAZW3, FileTypeMOBI, FileTypeCBZ, FileTypePDF} {
		assert.True(t, IsEbookFileType(ft), ft)
	}
	for _, ft := range []string{FileTypeM4B, "txt", ""} {
		assert.False(t, IsEbookFileType(ft), ft)
	}
}

func TestEbookCoverRank(t *testing.T) {
	t.Parallel()

	assert.Less(t, EbookCoverRank(FileTypeEPUB), EbookCoverRank(FileTypeKePub))
	assert.Less(t, EbookCoverRank(FileTypeKePub), EbookCoverRank(FileTypeAZW3))
	assert.Less(t, EbookCoverRank(FileTypeAZW3), EbookCoverRank(FileTypeMOBI))
	assert.Less(t, EbookCoverRank(FileTypeMOBI), EbookCoverRank(FileTypeCBZ))
	assert.Equal(t, EbookCoverRank(FileTypeCBZ), EbookCoverRank(FileTypePDF), "the other ebook formats keep their existing order")
}

func TestFileTypeMimeType_MOBI(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "application/x-mobipocket-ebook", FileTypeMimeType(FileTypeMOBI))
	assert.Equal(t, "application/vnd.amazon.mobi8-ebook", FileTypeMimeType(FileTypeAZW3))
}

func TestFileExtensionForPath(t *testing.T) {
	t.Parallel()

	assert.Equal(t, ".epub", FileExtensionForPath("/lib/Book/book.epub"))
	assert.Equal(t, ".kepub.epub", FileExtensionForPath("/lib/Book/book.kepub.epub"))
	assert.Equal(t, ".KePub.EPUB", FileExtensionForPath("/lib/Book/book.KePub.EPUB"), "keeps the path's case")
	assert.Equal(t, ".m4b", FileExtensionForPath("/lib/Book/book.m4b"))
	assert.Empty(t, FileExtensionForPath("/lib/Book/README"))
}
