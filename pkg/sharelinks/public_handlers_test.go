package sharelinks

import (
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
)

func TestBlankForRecipient_MainFileLabel(t *testing.T) {
	t.Parallel()

	str := func(s string) *string { return &s }
	tests := []struct {
		name string
		file *models.File
		want string
	}{
		{
			name: "a named main file keeps its name",
			file: &models.File{FileRole: models.FileRoleMain, Name: str("Special Edition"), Filepath: "/lib/Book/book.epub"},
			want: "Special Edition",
		},
		{
			name: "an unnamed main file gets no label, not its filename",
			file: &models.File{FileRole: models.FileRoleMain, Filepath: "/lib/Book/book.epub"},
			want: "",
		},
		{
			name: "a whitespace-only name counts as no name",
			file: &models.File{FileRole: models.FileRoleMain, Name: str("  "), Filepath: "/lib/Book/book.epub"},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			book := &models.Book{Files: []*models.File{tt.file}}
			blankForRecipient(book)
			assert.Equal(t, tt.want, book.Files[0].DisplayName)
			assert.Empty(t, book.Files[0].Filepath)
		})
	}
}
