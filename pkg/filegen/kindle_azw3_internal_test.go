package filegen

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A Kindle keeps a reading position per identifier, so a file regenerated
// after a metadata edit carries the same one, and another file its own.
func TestEPUBToAZW3_IdentifierStaysWithTheFile(t *testing.T) {
	t.Parallel()
	src := testgen.GenerateEPUB(t, t.TempDir(), "book.epub", testgen.EPUBOptions{Title: "Book"})
	gen, err := GeneratorFor(models.FileTypeEPUB, models.FileTypeAZW3)
	require.NoError(t, err)
	generate := func(title string, fileID int) []byte {
		dest := filepath.Join(t.TempDir(), "out.azw3")
		require.NoError(t, gen.Generate(context.Background(), src, dest, &models.Book{ID: 3, Title: title}, &models.File{ID: fileID, BookID: 3, FileType: models.FileTypeEPUB}))
		data, err := os.ReadFile(dest)
		require.NoError(t, err)
		return data
	}

	first, renamed, other := generate("Before", 7), generate("After", 7), generate("Before", 8)
	id := []byte(kindleFileIdentifier(&models.File{ID: 7}))
	assert.True(t, bytes.Contains(first, id))
	assert.True(t, bytes.Contains(renamed, id), "a regenerated file keeps its identifier")
	assert.False(t, bytes.Contains(other, id))
	assert.True(t, bytes.Contains(other, []byte(kindleFileIdentifier(&models.File{ID: 8}))))
	assert.NotEqual(t, kindleFileIdentifier(&models.File{ID: 7}), kindleFileIdentifier(&models.File{ID: 8}))
}
