package filegen_test

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robinjoseph08/golib/pointerutil"
	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/filegen"
	"github.com/shishobooks/shisho/pkg/filegen/filegentest"
	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/shishobooks/shisho/pkg/mobi"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/mp4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// streamGenerator returns the built-in generator for fileType as a
// StreamGenerator, failing the test when it does not stream.
func streamGenerator(t *testing.T, fileType string) filegen.StreamGenerator {
	t.Helper()
	gen, err := filegen.GetGenerator(fileType)
	require.NoError(t, err)
	sg, ok := gen.(filegen.StreamGenerator)
	require.True(t, ok, "the %s generator does not stream", fileType)
	return sg
}

// generatedByPath builds the file through the path-based Generate, which
// downloads used before the generator streamed.
func generatedByPath(t *testing.T, fileType, srcPath string, book *models.Book, file *models.File) []byte {
	t.Helper()
	gen, err := filegen.GetGenerator(fileType)
	require.NoError(t, err)
	destPath := filepath.Join(t.TempDir(), "out."+fileType)
	require.NoError(t, gen.Generate(context.Background(), srcPath, destPath, book, file))
	out, err := os.ReadFile(destPath)
	require.NoError(t, err)
	return out
}

func writeOutput(t *testing.T, name string, output []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, output, 0o600))
	return path
}

func brokenSource(t *testing.T) string {
	t.Helper()
	return writeOutput(t, "broken.bin", []byte(strings.Repeat("not a book ", 20)))
}

func TestM4BGenerator_Conformance(t *testing.T) {
	t.Parallel()
	testgen.SkipIfNoFFmpeg(t)

	for name, opts := range map[string]testgen.M4BOptions{
		"mdat first": {},
		"faststart":  {Faststart: true},
		"QuickTime chapter track": {Faststart: true, Chapters: []testgen.M4BChapter{
			{Title: "Source One", Start: 0},
			{Title: "Source Two", Start: 10},
		}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := testgen.TempDir(t, "m4b-stream-*")
			opts.Title = "Original Title"
			opts.Artist = "Original Author"
			opts.Duration = 30
			srcPath := testgen.GenerateM4B(t, dir, "source.m4b", opts)
			cover := coverPNG(t)
			require.NoError(t, os.WriteFile(filepath.Join(dir, "source.m4b.cover.png"), cover, 0o600))

			start, second, third := int64(0), int64(4000), int64(20000)
			book := &models.Book{
				Title:   "New Title",
				Authors: []*models.Author{{Person: &models.Person{Name: "New Author"}}},
			}
			file := &models.File{
				FileType:           models.FileTypeM4B,
				Filepath:           srcPath,
				CoverImageFilename: pointerutil.String("source.m4b.cover.png"),
				CoverMimeType:      pointerutil.String("image/png"),
				Narrators:          []*models.Narrator{{Person: &models.Person{Name: "New Narrator"}}},
				Chapters: []*models.Chapter{
					{Title: "Edited One", SortOrder: 0, StartTimestampMs: &start},
					{Title: "Edited Two", SortOrder: 1, StartTimestampMs: &second},
					{Title: "Edited Three", SortOrder: 2, StartTimestampMs: &third},
				},
			}
			want := generatedByPath(t, models.FileTypeM4B, srcPath, book, file)

			filegentest.Run(t, filegentest.Case{
				Generator: streamGenerator(t, models.FileTypeM4B),
				Source:    srcPath,
				Book:      book,
				File:      file,
				Broken:    brokenSource(t),
				Check: func(t *testing.T, output []byte) {
					assert.Equal(t, want, output, "the streamed file differs from the one built by path")
					path := writeOutput(t, "out.m4b", output)
					meta, err := mp4.ParseFull(path)
					require.NoError(t, err)
					assert.Equal(t, "New Title", meta.Title)
					assert.Equal(t, []mediafile.ParsedAuthor{{Name: "New Author"}}, meta.Authors)
					assert.Equal(t, []string{"New Narrator"}, meta.Narrators)
					assert.Equal(t, cover, meta.CoverData)
					var titles []string
					for _, ch := range meta.Chapters {
						titles = append(titles, ch.Title)
					}
					assert.Equal(t, []string{"Edited One", "Edited Two", "Edited Three"}, titles)
					assertDecodesCleanly(t, path)
				},
			})
		})
	}
}

// A source that changes while it is copied fails the write before its last
// byte, so a download sent with Content-Length never looks complete.
func TestM4BGenerator_SourceChangedDuringCopy(t *testing.T) {
	t.Parallel()
	testgen.SkipIfNoFFmpeg(t)
	dir := testgen.TempDir(t, "m4b-stream-*")
	srcPath := testgen.GenerateM4B(t, dir, "source.m4b", testgen.M4BOptions{Title: "Original", Duration: 10})
	file := &models.File{FileType: models.FileTypeM4B, Filepath: srcPath}

	src, err := filegen.OpenSource(srcPath)
	require.NoError(t, err)
	defer src.Close()
	prepared, err := streamGenerator(t, models.FileTypeM4B).Prepare(context.Background(), src, &models.Book{Title: "New"}, file)
	require.NoError(t, err)
	size, ok := prepared.Size()
	require.True(t, ok)

	w := &touchOnFirstWrite{path: srcPath}
	err = prepared.Write(context.Background(), w)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "changed")
	assert.Less(t, w.written, size, "every byte was written, so the download would look complete")
}

// touchOnFirstWrite counts the bytes written to it and appends to the file
// at path when the first of them arrive.
type touchOnFirstWrite struct {
	path    string
	written int64
}

func (w *touchOnFirstWrite) Write(p []byte) (int, error) {
	if w.written == 0 {
		f, err := os.OpenFile(w.path, os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			return 0, err
		}
		_, err = f.Write([]byte("more"))
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return 0, err
		}
	}
	w.written += int64(len(p))
	return len(p), nil
}

func TestMOBIGenerator_Conformance(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		kind     testgen.MOBIKind
		fileType string
	}{
		"MOBI6": {testgen.MOBIKindMOBI6, models.FileTypeMOBI},
		"AZW3":  {testgen.MOBIKindKF8, models.FileTypeAZW3},
		"combo": {testgen.MOBIKindCombo, models.FileTypeMOBI},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			filename := "book." + tc.fileType
			srcPath := testgen.GenerateMOBI(t, dir, filename, testgen.MOBIOptions{
				Kind:     tc.kind,
				Title:    "Original Title",
				Authors:  []string{"Original Author"},
				HasCover: true,
				// Large enough that preparation reads well under half of it.
				ExtraImageBytes: 64 << 10,
			})
			require.NoError(t, os.WriteFile(filepath.Join(dir, filename+".cover.png"), coverPNG(t), 0o600))
			book := &models.Book{
				Title:       "New Title",
				Description: pointerutil.String("New description."),
				Authors:     []*models.Author{{Person: &models.Person{Name: "New Author"}}},
			}
			file := &models.File{
				FileType:           tc.fileType,
				Filepath:           srcPath,
				CoverImageFilename: pointerutil.String(filename + ".cover.png"),
				CoverMimeType:      pointerutil.String("image/png"),
				Identifiers:        []*models.FileIdentifier{{Type: "isbn_13", Value: "9780141036144"}},
			}
			want := generatedByPath(t, tc.fileType, srcPath, book, file)

			filegentest.Run(t, filegentest.Case{
				Generator: streamGenerator(t, tc.fileType),
				Source:    srcPath,
				Book:      book,
				File:      file,
				Broken:    brokenSource(t),
				Check: func(t *testing.T, output []byte) {
					assert.Equal(t, want, output, "the streamed file differs from the one built by path")
					meta, err := mobi.Parse(writeOutput(t, "out."+tc.fileType, output))
					require.NoError(t, err)
					assert.Equal(t, "New Title", meta.Title)
					assert.Equal(t, []mediafile.ParsedAuthor{{Name: "New Author"}}, meta.Authors)
					assert.Equal(t, "New description.", meta.Description)
					assert.Equal(t, []mediafile.ParsedIdentifier{{Type: "isbn_13", Value: "9780141036144"}}, meta.Identifiers)
					assert.Equal(t, "image/jpeg", meta.CoverMimeType)
					assert.NotEmpty(t, meta.CoverData)
				},
			})
		})
	}
}

// assertDecodesCleanly runs the file through ffmpeg's decoder, which fails
// on chunk offsets that no longer point at the audio.
func assertDecodesCleanly(t *testing.T, path string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "ffmpeg", "-v", "error", "-i", path, "-f", "null", "-")
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "ffmpeg decode failed: %s", output)
	require.Empty(t, strings.TrimSpace(string(output)), "ffmpeg reported decode errors: %s", output)
}

func coverPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 30, 45))
	for y := range 45 {
		for x := range 30 {
			img.Set(x, y, color.RGBA{uint8(x * 8), uint8(y * 5), 120, 255}) //nolint:gosec // small test values
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}
