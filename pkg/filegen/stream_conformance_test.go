package filegen_test

import (
	"bytes"
	"image"
	"image/color"
	_ "image/jpeg" // decode the converted MOBI cover
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

			filegentest.Run(t, filegentest.Case{
				Generator: streamGenerator(t, models.FileTypeM4B),
				Source:    srcPath,
				Book:      book,
				File:      file,
				Broken:    brokenSource(t),
				Check: func(t *testing.T, output []byte) {
					path := writeOutput(t, "out.m4b", output)
					meta, err := mp4.ParseFull(path)
					require.NoError(t, err)
					assert.Equal(t, "New Title", meta.Title)
					assert.Equal(t, []mediafile.ParsedAuthor{{Name: "New Author"}}, meta.Authors)
					assert.Equal(t, []string{"New Narrator"}, meta.Narrators)
					assert.Equal(t, cover, meta.CoverData)
					var titles []string
					var starts []time.Duration
					for _, ch := range meta.Chapters {
						titles = append(titles, ch.Title)
						starts = append(starts, ch.Start)
					}
					assert.Equal(t, []string{"Edited One", "Edited Two", "Edited Three"}, titles)
					assert.Equal(t, []time.Duration{0, 4 * time.Second, 20 * time.Second}, starts)
					if len(opts.Chapters) > 0 {
						// A QuickTime chapter track is rebuilt, never added.
						assert.Equal(t, []string{"Edited One", "Edited Two", "Edited Three"}, ffprobeChapters(t, path), "players see the edited chapters")
					}
					assertDecodesCleanly(t, path)
				},
			})
		})
	}
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

			filegentest.Run(t, filegentest.Case{
				Generator: streamGenerator(t, tc.fileType),
				Source:    srcPath,
				Book:      book,
				File:      file,
				Broken:    brokenSource(t),
				Check: func(t *testing.T, output []byte) {
					meta, err := mobi.Parse(writeOutput(t, "out."+tc.fileType, output))
					require.NoError(t, err)
					assert.Equal(t, "New Title", meta.Title)
					assert.Equal(t, []mediafile.ParsedAuthor{{Name: "New Author"}}, meta.Authors)
					assert.Equal(t, "New description.", meta.Description)
					assert.Equal(t, []mediafile.ParsedIdentifier{{Type: "isbn_13", Value: "9780141036144"}}, meta.Identifiers)
					assert.Equal(t, "image/jpeg", meta.CoverMimeType, "a PNG cover is written as JPEG")
					cover, _, err := image.Decode(bytes.NewReader(meta.CoverData))
					require.NoError(t, err)
					assert.Equal(t, image.Rect(0, 0, 30, 45), cover.Bounds(), "the file's own cover was kept")
				},
			})
		})
	}
}

// ffprobeChapters returns the chapter titles ffprobe reports, which come
// from the QuickTime chapter track when there is one.
func ffprobeChapters(t *testing.T, path string) []string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "ffprobe", "-v", "error", "-show_entries", "chapter_tags=title", "-of", "csv=p=0", path)
	output, err := cmd.Output()
	require.NoError(t, err)
	return strings.Split(strings.TrimSpace(string(output)), "\n")
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
