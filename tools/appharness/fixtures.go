package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

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

// fixtures are small and the same each time, made by the harness and
// cached between its runs (writeFixtures), so there's no store of test books
// to keep by hand.
// Quiet Lighthouse is short enough to listen to the end of; Harbor Lights is
// long enough to listen to for a while, and small enough that the player
// buffers all of it, so the flows can listen offline.
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
// failure panics with the message, which makeFixtures turns into an error.
type fixtureT struct {
	ctx context.Context
}

type fixtureFailure struct{ message string }

func (t fixtureT) Helper()                  {}
func (t fixtureT) Context() context.Context { return t.ctx }
func (t fixtureT) Fatalf(format string, args ...any) {
	panic(fixtureFailure{message: fmt.Sprintf(format, args...)})
}

// fixtureSources are what the fixtures are made from, relative to the
// repository root: their definitions above, the generators and the packages
// of this module they use (TestFixtureSourcesCoverTheGenerators checks the
// list), and go.mod and go.sum for the rest. A change to any of them makes the fixtures
// again. CI keys its cache of them on fixtureKey too (`appharness fixtures
// -key`).
var fixtureSources = []string{
	"tools/appharness/fixtures.go",
	"internal/testgen",
	"internal/mobigen",
	"pkg/mediafile",
	"pkg/models",
	"pkg/pdf",
	"go.mod",
	"go.sum",
}

// unusedFixturesKept is how long a set of cached fixtures no run has used
// stays, so worktrees on branches whose fixtures differ each keep theirs.
const unusedFixturesKept = 7 * 24 * time.Hour

// fixtureCacheRoot is the folder the cached fixtures are kept in.
func fixtureCacheRoot() (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", errors.WithStack(err)
	}
	return filepath.Join(cache, "shisho-appharness", "fixtures"), nil
}

// writeFixtures writes each fixture into its own folder under dir, and the
// annex's under annexDir(dir), copied from the fixtures cached between runs
// for this checkout's fixtureSources (root is the repository's root). Making
// them takes ffmpeg and several seconds; copying them, a moment.
// SHISHO_HARNESS_FRESH makes them anew in dir instead.
func writeFixtures(ctx context.Context, root, dir string) error {
	if freshRun() {
		return makeFixtures(ctx, dir)
	}
	key, err := fixtureKey(root)
	if err != nil {
		return err
	}
	cacheRoot, err := fixtureCacheRoot()
	if err != nil {
		return err
	}
	copyCached := func() error {
		cached, err := fillFixtureCache(cacheRoot, key, func(made string) error {
			fmt.Fprintln(os.Stderr, "appharness: making the fixtures, cached in", cacheRoot)
			return makeFixtures(ctx, filepath.Join(made, "library"))
		})
		if err != nil {
			return err
		}
		if err := os.CopyFS(dir, os.DirFS(filepath.Join(cached, "library"))); err != nil {
			return errors.WithStack(err)
		}
		return errors.WithStack(os.CopyFS(annexDir(dir), os.DirFS(annexDir(filepath.Join(cached, "library")))))
	}
	// Another run may prune a long unused set while this one copies it; then
	// it's made again.
	if err := copyCached(); err != nil {
		_ = os.RemoveAll(dir)
		_ = os.RemoveAll(annexDir(dir))
		return copyCached()
	}
	return nil
}

// fixtureKey hashes the files under root's fixtureSources: their paths and
// contents.
func fixtureKey(root string) (string, error) {
	hash := sha256.New()
	for _, source := range fixtureSources {
		err := filepath.WalkDir(filepath.Join(root, source), func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return err
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return errors.WithStack(err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return errors.WithStack(err)
			}
			fmt.Fprintf(hash, "%s\x00%d\x00", filepath.ToSlash(rel), len(data))
			hash.Write(data)
			return nil
		})
		if err != nil {
			return "", errors.Wrapf(err, "hash the fixtures' sources; run the harness from the repository's root")
		}
	}
	return hex.EncodeToString(hash.Sum(nil))[:16], nil
}

// fillFixtureCache returns the folder under cacheRoot holding key's
// fixtures, calling makeFixtures to fill it when there's none yet. A set,
// once there, never changes. makeFixtures fills a scratch folder that becomes
// key's only once it succeeds, so two runs at once, or one interrupted, never
// leave a partial set. Sets unused for unusedFixturesKept, and scratch
// folders an interrupted run left, are removed.
func fillFixtureCache(cacheRoot, key string, makeFixtures func(dir string) error) (string, error) {
	cached := filepath.Join(cacheRoot, key)
	defer pruneFixtureCache(cacheRoot, key)
	if _, err := os.Stat(cached); err == nil {
		// Its last use, for pruneFixtureCache.
		now := time.Now()
		_ = os.Chtimes(cached, now, now)
		return cached, nil
	}
	if err := os.MkdirAll(cacheRoot, 0o755); err != nil {
		return "", errors.WithStack(err)
	}
	scratch, err := os.MkdirTemp(cacheRoot, ".making-")
	if err != nil {
		return "", errors.WithStack(err)
	}
	defer os.RemoveAll(scratch)
	if err := makeFixtures(scratch); err != nil {
		return "", err
	}
	// Another run may have made them meanwhile; either copy will do.
	if err := os.Rename(scratch, cached); err != nil {
		if _, statErr := os.Stat(cached); statErr != nil {
			return "", errors.WithStack(err)
		}
	}
	return cached, nil
}

// pruneFixtureCache removes the sets other than key's unused for
// unusedFixturesKept, and scratch folders as old, which a run killed while
// making fixtures left.
func pruneFixtureCache(cacheRoot, key string) {
	entries, err := os.ReadDir(cacheRoot)
	if err != nil {
		return
	}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil || entry.Name() == key || time.Since(info.ModTime()) < unusedFixturesKept {
			continue
		}
		_ = os.RemoveAll(filepath.Join(cacheRoot, entry.Name()))
	}
}

// makeFixtures writes each fixture into its own folder under dir, and the
// annex's under annexDir(dir): an EPUB and a CBZ from the Go test helpers, an
// M4B of tones with known chapters (ffmpeg), and a PDF.
func makeFixtures(ctx context.Context, dir string) (err error) {
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
