package filegen_test

import (
	"context"
	"io"
	"testing"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/filegen"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/require"
)

// Every File Type a File is offered as can be delivered: a chain the graph
// finds but whose steps cannot feed each other would be offered in the
// download menu and fail every download.
func TestReachableFileTypes_EveryChainRuns(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	samples := map[string]string{
		models.FileTypeEPUB:  testgen.GenerateEPUB(t, dir, "book.epub", testgen.EPUBOptions{Title: "Book"}),
		models.FileTypeCBZ:   testgen.GenerateCBZ(t, dir, "book.cbz", testgen.CBZOptions{Title: "Book"}),
		models.FileTypeKePub: testgen.GenerateEPUB(t, dir, "book.kepub.epub", testgen.EPUBOptions{Title: "Book", KoboMarkup: true}),
		models.FileTypeAZW3:  testgen.GenerateMOBI(t, dir, "book.azw3", testgen.KF8Sample(testgen.MOBIKindKF8)),
		models.FileTypeMOBI:  testgen.GenerateMOBI(t, dir, "book.mobi", testgen.KF8Sample(testgen.MOBIKindCombo)),
	}
	for _, fileType := range models.BuiltInFileTypes {
		for _, to := range models.ReachableFileTypes(fileType)[1:] {
			t.Run(fileType+" to "+to, func(t *testing.T) {
				t.Parallel()
				sample, ok := samples[fileType]
				require.True(t, ok, "add a %s sample: it now reaches %s", fileType, to)
				gen, err := filegen.GeneratorFor(fileType, to)
				require.NoError(t, err)
				sg, ok := gen.(filegen.StreamGenerator)
				require.True(t, ok)
				src, err := filegen.OpenSource(sample)
				require.NoError(t, err)
				defer src.Close()
				prepared, err := sg.Prepare(context.Background(), src, &models.Book{Title: "Book"}, &models.File{FileType: fileType})
				require.NoError(t, err)
				require.NoError(t, prepared.Write(context.Background(), io.Discard))
			})
		}
	}
}
