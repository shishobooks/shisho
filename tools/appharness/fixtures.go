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
}

// fixtures are made fresh for every run, small and the same each time, so
// there's no shared store of test books to keep.
var fixtures = []Fixture{
	{Title: "The Harness Novel", Author: "Ada Writer", FileType: "epub"},
	{Title: "Harness Comic", Author: "Pat Penciller", FileType: "cbz"},
	{Title: "Quiet Lighthouse", Author: "Ann Author", FileType: "m4b"},
	{Title: "Harness Manual", FileType: "pdf"},
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

// writeFixtures writes each fixture into its own folder under dir: an EPUB
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
	for _, f := range fixtures {
		folder := filepath.Join(dir, f.Title)
		if err := os.MkdirAll(folder, 0o755); err != nil {
			return errors.WithStack(err)
		}
		switch f.FileType {
		case "epub":
			testgen.GenerateEPUB(t, folder, "novel.epub", testgen.EPUBOptions{
				Title: f.Title, Authors: []string{f.Author}, Series: "Harness Series", SeriesNumber: &one, HasCover: true,
			})
		case "cbz":
			testgen.GenerateCBZ(t, folder, "comic.cbz", testgen.CBZOptions{
				Title: f.Title, Penciller: f.Author, HasComicInfo: true, PageCount: 3,
			})
		case "m4b":
			testgen.GenerateM4B(t, folder, "lighthouse.m4b", testgen.M4BOptions{
				Title: f.Title, Artist: f.Author, Composer: "Ned Narrator", Duration: 9, HasCover: true,
				Chapters: []testgen.M4BChapter{{Title: "Chapter 1", Start: 0}, {Title: "Chapter 2", Start: 3}, {Title: "Chapter 3", Start: 6}},
			})
		case "pdf":
			testgen.GeneratePDF(t, folder, "manual.pdf", testgen.PDFOptions{PageCount: 2})
		}
	}
	return nil
}
