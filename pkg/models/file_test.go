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
