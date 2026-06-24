package mp4_test

import (
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/mp4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWriteToFile_RewritesQuickTimeChapterTrackTitles is the regression test for
// the "manual chapters don't show up in the downloaded audiobook" bug.
//
// Real Audible/Apple exports carry chapters in TWO stores: a QuickTime text
// chapter track (audio trak -> tref/chap -> a text trak) AND a Nero chpl box.
// The writer used to rewrite only the chpl box and copy the QuickTime track
// verbatim. Players (and Shisho's own reader, and ffprobe) prefer the QuickTime
// track, so the user's edited titles in chpl were masked by the stale source
// titles in the QuickTime track.
//
// The contract: after a metadata rewrite, the chapters the user saved must be
// the ones the QuickTime chapter track reports.
func TestWriteToFile_RewritesQuickTimeChapterTrackTitles(t *testing.T) {
	t.Parallel()
	testgen.SkipIfNoFFmpeg(t)
	dir := testgen.TempDir(t, "mp4-qtchapters-*")

	// ffmpeg emits a QuickTime chapter text track for these, mirroring the
	// generic numeric titles an Audible export ships with.
	srcPath := testgen.GenerateM4B(t, dir, "src.m4b", testgen.M4BOptions{
		Title:     "Bossypants",
		Duration:  10.0,
		Faststart: true,
		Chapters: []testgen.M4BChapter{
			{Title: "001", Start: 0.0},
			{Title: "002", Start: 3.0},
			{Title: "003", Start: 7.0},
		},
	})

	meta, err := mp4.ParseFull(srcPath)
	require.NoError(t, err)
	// Sanity: the reader is seeing the QuickTime track's titles in the source.
	require.Len(t, meta.Chapters, 3)
	require.Equal(t, "001", meta.Chapters[0].Title, "source should carry generic QuickTime titles")

	// Simulate the user's manual chapter edits in Shisho.
	meta.Chapters = []mp4.Chapter{
		{Title: "Opening Credits", Start: 0, End: 3 * time.Second},
		{Title: "Origin Story", Start: 3 * time.Second, End: 7 * time.Second},
		{Title: "Growing Up and Liking It", Start: 7 * time.Second, End: 10 * time.Second},
	}

	destPath := filepath.Join(dir, "dest.m4b")
	require.NoError(t, mp4.WriteToFile(srcPath, destPath, meta))

	out, err := mp4.ParseFull(destPath)
	require.NoError(t, err)
	require.Len(t, out.Chapters, 3)
	assert.Equal(t, "Opening Credits", out.Chapters[0].Title)
	assert.Equal(t, "Origin Story", out.Chapters[1].Title)
	assert.Equal(t, "Growing Up and Liking It", out.Chapters[2].Title)
}

// TestWriteToFile_ChaptersVisibleToExternalPlayers proves the rewritten chapters
// are what an actual player sees (ffprobe reads the QuickTime track, the same
// one Apple Books/Bound prefer) and that relocating the chapter samples did not
// corrupt the audio (ffmpeg decodes with zero errors).
func TestWriteToFile_ChaptersVisibleToExternalPlayers(t *testing.T) {
	t.Parallel()
	testgen.SkipIfNoFFmpeg(t)
	dir := testgen.TempDir(t, "mp4-qtchapters-ext-*")

	srcPath := testgen.GenerateM4B(t, dir, "src.m4b", testgen.M4BOptions{
		Title:     "Bossypants",
		Duration:  10.0,
		Faststart: true,
		Chapters: []testgen.M4BChapter{
			{Title: "001", Start: 0.0},
			{Title: "002", Start: 4.0},
		},
	})

	meta, err := mp4.ParseFull(srcPath)
	require.NoError(t, err)
	meta.Chapters = []mp4.Chapter{
		{Title: "Opening Credits", Start: 0, End: 4 * time.Second},
		{Title: "Origin Story", Start: 4 * time.Second, End: 10 * time.Second},
	}

	destPath := filepath.Join(dir, "dest.m4b")
	require.NoError(t, mp4.WriteToFile(srcPath, destPath, meta))

	assert.Equal(t, []string{"Opening Credits", "Origin Story"},
		readChaptersWithFFprobe(t, destPath),
		"an external player must see the user's edited chapter titles")
	assertDecodesCleanly(t, destPath)
}

// assertDecodesCleanly runs the file through ffmpeg's decoder and fails if it
// reports any error. This is the strongest available proof that relocating the
// chapter track left the audio mdat and its chunk offsets intact (the #393
// failure mode surfaced here as "channel element ... is not allocated").
func assertDecodesCleanly(t *testing.T, path string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "ffmpeg", "-v", "error", "-i", path, "-f", "null", "-")
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "ffmpeg decode failed: %s", output)
	require.Empty(t, strings.TrimSpace(string(output)), "ffmpeg reported decode errors: %s", output)
}

// readChaptersWithFFprobe returns chapter titles in order as ffprobe reports
// them, which reads the QuickTime chapter track in preference to the Nero chpl.
func readChaptersWithFFprobe(t *testing.T, path string) []string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "ffprobe",
		"-v", "error", "-show_chapters", "-of", "compact", path)
	output, err := cmd.Output()
	require.NoError(t, err)

	var titles []string
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		const marker = "tag:title="
		if idx := strings.Index(line, marker); idx >= 0 {
			titles = append(titles, line[idx+len(marker):])
		}
	}
	return titles
}

// readChapterEndTimesWithFFprobe returns each chapter's end time (in seconds) in
// order, as ffprobe reports them from the QuickTime chapter track.
func readChapterEndTimesWithFFprobe(t *testing.T, path string) []float64 {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "ffprobe",
		"-v", "error", "-show_chapters", "-of", "compact", path)
	output, err := cmd.Output()
	require.NoError(t, err)

	var ends []float64
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		for _, field := range strings.Split(line, "|") {
			const marker = "end_time="
			if strings.HasPrefix(field, marker) {
				v, perr := strconv.ParseFloat(field[len(marker):], 64)
				require.NoError(t, perr)
				ends = append(ends, v)
			}
		}
	}
	return ends
}

// TestWriteToFile_RewritesChapterTimestamps verifies the rebuilt chapter track
// carries the DB timestamps, not the source's, so a user who re-timed chapters
// sees the new boundaries.
func TestWriteToFile_RewritesChapterTimestamps(t *testing.T) {
	t.Parallel()
	testgen.SkipIfNoFFmpeg(t)
	dir := testgen.TempDir(t, "mp4-qtchapters-ts-*")

	srcPath := testgen.GenerateM4B(t, dir, "src.m4b", testgen.M4BOptions{
		Title:     "Bossypants",
		Duration:  10.0,
		Faststart: true,
		Chapters: []testgen.M4BChapter{
			{Title: "001", Start: 0.0},
			{Title: "002", Start: 5.0},
		},
	})

	meta, err := mp4.ParseFull(srcPath)
	require.NoError(t, err)
	meta.Chapters = []mp4.Chapter{
		{Title: "One", Start: 0, End: 2 * time.Second},
		{Title: "Two", Start: 2 * time.Second, End: 6 * time.Second},
		{Title: "Three", Start: 6 * time.Second, End: 10 * time.Second},
	}

	destPath := filepath.Join(dir, "dest.m4b")
	require.NoError(t, mp4.WriteToFile(srcPath, destPath, meta))

	out, err := mp4.ParseFull(destPath)
	require.NoError(t, err)
	require.Len(t, out.Chapters, 3)
	assert.InDelta(t, 0, out.Chapters[0].Start.Milliseconds(), 50)
	assert.InDelta(t, 2000, out.Chapters[1].Start.Milliseconds(), 50)
	assert.InDelta(t, 6000, out.Chapters[2].Start.Milliseconds(), 50)
}

// TestWriteToFile_RewritesNonContiguousChapterTimestamps guards the timing math
// for chapters that are not back-to-back. The rebuilt track's per-sample deltas
// must come from the gaps between consecutive starts (so a reader accumulating
// them reproduces the starts), not from each chapter's own End-Start span. A
// regression to per-chapter End-Start would still pass the contiguous-chapter
// tests but diverge here.
func TestWriteToFile_RewritesNonContiguousChapterTimestamps(t *testing.T) {
	t.Parallel()
	testgen.SkipIfNoFFmpeg(t)
	dir := testgen.TempDir(t, "mp4-qtchapters-gap-*")

	srcPath := testgen.GenerateM4B(t, dir, "src.m4b", testgen.M4BOptions{
		Title:     "Bossypants",
		Duration:  10.0,
		Faststart: true,
		Chapters: []testgen.M4BChapter{
			{Title: "001", Start: 0.0},
			{Title: "002", Start: 5.0},
		},
	})

	meta, err := mp4.ParseFull(srcPath)
	require.NoError(t, err)
	// Gaps between chapters: End[i] != Start[i+1].
	meta.Chapters = []mp4.Chapter{
		{Title: "One", Start: 0, End: 1 * time.Second},
		{Title: "Two", Start: 3 * time.Second, End: 5 * time.Second},
		{Title: "Three", Start: 7 * time.Second, End: 10 * time.Second},
	}

	destPath := filepath.Join(dir, "dest.m4b")
	require.NoError(t, mp4.WriteToFile(srcPath, destPath, meta))

	out, err := mp4.ParseFull(destPath)
	require.NoError(t, err)
	require.Len(t, out.Chapters, 3)
	// Starts must be reproduced from the gap-based deltas. Under an End-Start
	// regression these would come back as 0, 1000, 3000.
	assert.InDelta(t, 0, out.Chapters[0].Start.Milliseconds(), 50)
	assert.InDelta(t, 3000, out.Chapters[1].Start.Milliseconds(), 50)
	assert.InDelta(t, 7000, out.Chapters[2].Start.Milliseconds(), 50)

	// The final sample's delta comes from the last chapter's End, which ffprobe
	// reports as the last chapter's end time.
	ends := readChapterEndTimesWithFFprobe(t, destPath)
	require.NotEmpty(t, ends)
	assert.InDelta(t, 10.0, ends[len(ends)-1], 0.1, "last chapter must end at the final End time")
}

// TestWriteToFile_ChapterCountDiffersFromSource verifies the rebuilt track is
// sized to the DB chapters, whether the user removed chapters (fewer than the
// source's QuickTime track) or added them (more).
func TestWriteToFile_ChapterCountDiffersFromSource(t *testing.T) {
	t.Parallel()
	testgen.SkipIfNoFFmpeg(t)

	build := func(t *testing.T, dbChapters []mp4.Chapter) []mp4.Chapter {
		t.Helper()
		dir := testgen.TempDir(t, "mp4-qtchapters-count-*")
		srcPath := testgen.GenerateM4B(t, dir, "src.m4b", testgen.M4BOptions{
			Title:     "Bossypants",
			Duration:  12.0,
			Faststart: true,
			Chapters: []testgen.M4BChapter{
				{Title: "001", Start: 0.0},
				{Title: "002", Start: 4.0},
				{Title: "003", Start: 8.0},
			},
		})
		meta, err := mp4.ParseFull(srcPath)
		require.NoError(t, err)
		meta.Chapters = dbChapters
		destPath := filepath.Join(dir, "dest.m4b")
		require.NoError(t, mp4.WriteToFile(srcPath, destPath, meta))
		assertDecodesCleanly(t, destPath)
		out, err := mp4.ParseFull(destPath)
		require.NoError(t, err)
		return out.Chapters
	}

	t.Run("fewer than source", func(t *testing.T) {
		t.Parallel()
		got := build(t, []mp4.Chapter{
			{Title: "Solo", Start: 0, End: 12 * time.Second},
		})
		require.Len(t, got, 1)
		assert.Equal(t, "Solo", got[0].Title)
	})

	t.Run("more than source", func(t *testing.T) {
		t.Parallel()
		got := build(t, []mp4.Chapter{
			{Title: "A", Start: 0, End: 3 * time.Second},
			{Title: "B", Start: 3 * time.Second, End: 6 * time.Second},
			{Title: "C", Start: 6 * time.Second, End: 9 * time.Second},
			{Title: "D", Start: 9 * time.Second, End: 12 * time.Second},
		})
		require.Len(t, got, 4)
		assert.Equal(t, []string{"A", "B", "C", "D"},
			[]string{got[0].Title, got[1].Title, got[2].Title, got[3].Title})
	})
}

// TestWriteToFile_MdatFirstLayoutRewritesChapterTrack covers the non-faststart
// layout (mdat before moov, ffmpeg's default). The audio mdat does not move, so
// no audio offset shift happens, but the chapter track must still be rebuilt
// from the DB chapters and the file must decode cleanly.
func TestWriteToFile_MdatFirstLayoutRewritesChapterTrack(t *testing.T) {
	t.Parallel()
	testgen.SkipIfNoFFmpeg(t)
	dir := testgen.TempDir(t, "mp4-qtchapters-mdatfirst-*")

	srcPath := testgen.GenerateM4B(t, dir, "src.m4b", testgen.M4BOptions{
		Title:    "Bossypants",
		Duration: 8.0,
		// Faststart omitted: mdat comes before moov.
		Chapters: []testgen.M4BChapter{
			{Title: "001", Start: 0.0},
			{Title: "002", Start: 4.0},
		},
	})

	meta, err := mp4.ParseFull(srcPath)
	require.NoError(t, err)
	meta.Chapters = []mp4.Chapter{
		{Title: "Intro", Start: 0, End: 4 * time.Second},
		{Title: "Body", Start: 4 * time.Second, End: 8 * time.Second},
	}

	destPath := filepath.Join(dir, "dest.m4b")
	require.NoError(t, mp4.WriteToFile(srcPath, destPath, meta))

	out, err := mp4.ParseFull(destPath)
	require.NoError(t, err)
	require.Len(t, out.Chapters, 2)
	assert.Equal(t, "Intro", out.Chapters[0].Title)
	assert.Equal(t, "Body", out.Chapters[1].Title)
	assertDecodesCleanly(t, destPath)
}
