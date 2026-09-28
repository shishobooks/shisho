package models

import (
	"encoding/json"
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

func TestFile_MarshalJSONIncludesDisplayName(t *testing.T) {
	t.Parallel()

	name := "Original Title"
	file := &File{FileRole: FileRoleSupplement, Name: &name, Filepath: "/lib/Book/notes.pdf"}

	data, err := json.Marshal([]*File{file})
	require.NoError(t, err)

	var out []map[string]any
	require.NoError(t, json.Unmarshal(data, &out))
	require.Len(t, out, 1)
	assert.Equal(t, "notes.pdf", out[0]["display_name"])
	assert.Equal(t, "Original Title", out[0]["name"], "the stored name is left alone")
	assert.Equal(t, "/lib/Book/notes.pdf", out[0]["filepath"])
	assert.Empty(t, file.DisplayName, "marshaling does not mutate the model")
}

func TestFile_MarshalJSONKeepsPresetDisplayName(t *testing.T) {
	t.Parallel()

	// The Share Link payload resolves the display name and then blanks the
	// path, so a preset value must survive marshaling.
	file := &File{FileRole: FileRoleSupplement, DisplayName: "notes.pdf"}

	data, err := json.Marshal(file)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"display_name":"notes.pdf"`)
}
