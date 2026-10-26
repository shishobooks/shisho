package filegen_test

import (
	"context"
	"io"
	"path/filepath"
	"testing"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/filegen"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/require"
)

// chainSamples returns a sample file of every File Type a Conversion
// starts from.
func chainSamples(t *testing.T) map[string]string {
	t.Helper()
	dir := t.TempDir()
	return map[string]string{
		models.FileTypeEPUB:  testgen.GenerateEPUB(t, dir, "book.epub", testgen.EPUBOptions{Title: "Book"}),
		models.FileTypeCBZ:   testgen.GenerateCBZ(t, dir, "book.cbz", testgen.CBZOptions{Title: "Book"}),
		models.FileTypeKePub: testgen.GenerateEPUB(t, dir, "book.kepub.epub", testgen.EPUBOptions{Title: "Book", KoboMarkup: true}),
		models.FileTypeAZW3:  testgen.GenerateMOBI(t, dir, "book.azw3", testgen.KF8Sample(testgen.MOBIKindKF8)),
		models.FileTypeMOBI:  testgen.GenerateMOBI(t, dir, "book.mobi", testgen.KF8Sample(testgen.MOBIKindCombo)),
	}
}

// Every File Type a File is offered as can be delivered: a chain the graph
// finds but whose steps cannot feed each other would be offered in the
// download menu and fail every download.
func TestReachableFileTypes_EveryChainRuns(t *testing.T) {
	t.Parallel()
	samples := chainSamples(t)
	for _, fileType := range models.BuiltInFileTypes {
		for _, to := range models.ReachableFileTypes(fileType)[1:] {
			t.Run(fileType+" to "+to, func(t *testing.T) {
				t.Parallel()
				sample, ok := samples[fileType]
				require.True(t, ok, "add a %s sample: it now reaches %s", fileType, to)
				gen, err := filegen.GeneratorFor(fileType, to)
				require.NoError(t, err)
				book, file := &models.Book{Title: "Book"}, &models.File{FileType: fileType}
				sg, ok := gen.(filegen.StreamGenerator)
				if !ok {
					// A File Type that can't stream is built in full.
					require.NoError(t, gen.Generate(context.Background(), sample, filepath.Join(t.TempDir(), "out."+to), book, file))
					return
				}
				src, err := filegen.OpenSource(sample)
				require.NoError(t, err)
				defer src.Close()
				prepared, err := sg.Prepare(context.Background(), src, book, file)
				require.NoError(t, err)
				require.NoError(t, prepared.Write(context.Background(), io.Discard))
			})
		}
	}
}
