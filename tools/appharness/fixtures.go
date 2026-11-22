package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/internal/testgen"
)

// Fixture is a Book the flows look for, by the title the scan gives it.
type Fixture struct {
	Title    string
	Author   string
	FileType string
	// Series is an EPUB's Series, which it's number 1 of.
	Series string
	// Duration is an audiobook's length in seconds, and Chapters where its
	// chapters start.
	Duration float64
	Chapters []testgen.M4BChapter
}

// fixtures are made fresh for every run, small and the same each time, so
// there's no shared store of test books to keep. Quiet Lighthouse is short
// enough to listen to the end of; Harbor Lights is long enough to listen to
// for a while, and small enough that the player buffers all of it, so the
// flows can listen offline.
var fixtures = []Fixture{
	{Title: "The Harness Novel", Author: "Ada Writer", FileType: "epub", Series: "Harness Series"},
	{Title: "Harness Comic", Author: "Pat Penciller", FileType: "cbz"},
	{Title: "Quiet Lighthouse", Author: "Ann Author", FileType: "m4b", Duration: 9,
		Chapters: []testgen.M4BChapter{{Title: "Chapter 1", Start: 0}, {Title: "Chapter 2", Start: 3}, {Title: "Chapter 3", Start: 6}}},
	{Title: "Harbor Lights", Author: "Ann Author", FileType: "m4b", Duration: 240,
		Chapters: []testgen.M4BChapter{{Title: "The Harbor", Start: 0}, {Title: "The Lights", Start: 120}}},
	{Title: "Harness Manual", FileType: "pdf"},
}

// annexFixtures are the second Library's, Harness Annex, which only the
// harness user can access: switching accounts then shows each its own
// Libraries, and the Library picker has a choice to keep per account. One
// Book, so the first Library's Books still fit on a phone's screen.
var annexFixtures = []Fixture{
	{Title: "Annex Atlas", Author: "Bo Mapmaker", FileType: "epub"},
}

// annexDir is the second Library's folder, beside the first's.
func annexDir(library string) string {
	return library + "-annex"
}

// fixtureT lets the test file generators run outside a test: a fatal
// failure panics with the message, which writeFixtures turns into an error.
type fixtureT struct {
	ctx context.Context
}

type fixtureFailure struct{ message string }

func (t fixtureT) Helper()                  {}
func (t fixtureT) Context() context.Context { return t.ctx }
func (t fixtureT) Fatalf(format string, args ...any) {
	panic(fixtureFailure{message: fmt.Sprintf(format, args...)})
}

// writeFixtures writes each fixture into its own folder under dir, and the
// annex's under annexDir(dir): an EPUB
// and a CBZ from the Go test helpers, an M4B of tones with known chapters
// (ffmpeg), and a PDF.
func writeFixtures(ctx context.Context, dir string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			failure, ok := r.(fixtureFailure)
			if !ok {
				panic(r)
			}
			err = errors.New(failure.message)
		}
	}()
	if !testgen.FFmpegAvailable() {
		return errors.New("ffmpeg is needed to make the M4B fixture; install it (brew install ffmpeg)")
	}
	t := fixtureT{ctx: ctx}
	one := 1.0
	all := map[string][]Fixture{dir: fixtures, annexDir(dir): annexFixtures}
	for root, list := range all {
		for _, f := range list {
			writeFixture(t, filepath.Join(root, f.Title), f, &one)
		}
	}
	return nil
}

func writeFixture(t fixtureT, folder string, f Fixture, one *float64) {
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatalf("make %s: %v", folder, err)
	}
	switch f.FileType {
	case "epub":
		options := testgen.EPUBOptions{Title: f.Title, Authors: []string{f.Author}, Series: f.Series, HasCover: true}
		if f.Series != "" {
			options.SeriesNumber = one
		}
		testgen.GenerateEPUB(t, folder, "novel.epub", options)
	case "cbz":
		testgen.GenerateCBZ(t, folder, "comic.cbz", testgen.CBZOptions{
			Title: f.Title, Penciller: f.Author, HasComicInfo: true, PageCount: 3,
		})
	case "m4b":
		testgen.GenerateM4B(t, folder, "audiobook.m4b", testgen.M4BOptions{
			Title: f.Title, Artist: f.Author, Composer: "Ned Narrator", Duration: f.Duration, HasCover: true,
			Chapters: f.Chapters,
		})
	case "pdf":
		testgen.GeneratePDF(t, folder, "manual.pdf", testgen.PDFOptions{PageCount: 2})
	}
}
