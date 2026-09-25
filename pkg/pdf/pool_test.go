package pdf

import (
	"bufio"
	"bytes"
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/robinjoseph08/golib/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tests in this file hold the process-global pdfium instance and change
// package-level state (the Scan wait, the logger output), so none of them may
// call t.Parallel().

// fixedTimeoutBound is how quickly a Scan-path call must give up when the
// Scan wait is shortened in a test. It is well under InteractivePdfiumTimeout,
// so a caller that waited a fixed 30 seconds instead of scanPdfiumTimeout
// fails the bound.
const fixedTimeoutBound = InteractivePdfiumTimeout / 3

func TestScanPdfiumTimeout_Default(t *testing.T) {
	// No other test here runs with the real Scan wait, so this pins it.
	assert.Equal(t, 5*time.Minute, scanPdfiumTimeout)
	assert.Greater(t, scanPdfiumTimeout, InteractivePdfiumTimeout,
		"Scans should wait longer for pdfium than interactive requests")
}

// setScanPdfiumTimeout changes the Scan-path pdfium wait for one test.
func setScanPdfiumTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	prev := scanPdfiumTimeout
	scanPdfiumTimeout = d
	t.Cleanup(func() { scanPdfiumTimeout = prev })
}

// holdPdfiumInstance takes the shared pdfium instance and returns a function
// that gives it back. The instance is also given back when the test ends.
func holdPdfiumInstance(t *testing.T) func() {
	t.Helper()
	instance, err := PdfiumInstance(InteractivePdfiumTimeout)
	require.NoError(t, err)
	var once sync.Once
	release := func() {
		once.Do(func() { _ = instance.Close() })
	}
	t.Cleanup(release)
	return release
}

type capturedLogLine struct {
	Level   string         `json:"level"`
	Message string         `json:"message"`
	Data    map[string]any `json:"data"`
}

// captureLogs redirects loggers created during the test into a buffer.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	// logger.New reads LOG_LEVEL, and a level above warn would hide the lines
	// under test.
	t.Setenv("LOG_LEVEL", "")
	var buf bytes.Buffer
	prev := logger.Output()
	logger.SetOutput(&buf)
	t.Cleanup(func() { logger.SetOutput(prev) })
	return &buf
}

func warnLines(t *testing.T, buf *bytes.Buffer) []capturedLogLine {
	t.Helper()
	var lines []capturedLogLine
	scanner := bufio.NewScanner(bytes.NewReader(buf.Bytes()))
	for scanner.Scan() {
		var line capturedLogLine
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &line), "log line: %s", scanner.Text())
		if line.Level == "warn" {
			lines = append(lines, line)
		}
	}
	require.NoError(t, scanner.Err())
	return lines
}

// writeCoverlessPDFWithOutline writes a PDF with no embedded images, so the
// cover has to be rendered by pdfium, and with two bookmarks.
func writeCoverlessPDFWithOutline(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "coverless-with-outline.pdf")
	require.NoError(t, writeRawPDFWithOutline(path, 3, []outlineFixture{
		{title: "Introduction", pageIndex: 0},
		{title: "Main Content", pageIndex: 1},
	}))
	return path
}

func TestParse_LogsCoverAndOutlineFailures(t *testing.T) {
	path := writeCoverlessPDFWithOutline(t)

	// Hold the instance for the whole Parse so every pdfium wait times out.
	_ = holdPdfiumInstance(t)
	setScanPdfiumTimeout(t, 50*time.Millisecond)
	buf := captureLogs(t)

	start := time.Now()
	meta, err := Parse(path)
	require.NoError(t, err, "cover and outline extraction are best-effort")
	assert.Less(t, time.Since(start), fixedTimeoutBound,
		"Parse should give up after scanPdfiumTimeout, not a fixed wait")

	assert.Empty(t, meta.CoverData)
	assert.Empty(t, meta.Chapters)
	require.NotNil(t, meta.CoverPage)
	assert.Equal(t, 0, *meta.CoverPage)

	warns := warnLines(t, buf)
	messages := make([]string, 0, len(warns))
	for _, w := range warns {
		messages = append(messages, w.Message)
		assert.Equal(t, path, w.Data["path"], "warning %q should name the file", w.Message)
		assert.NotEmpty(t, w.Data["error"], "warning %q should carry the error", w.Message)
	}
	assert.ElementsMatch(t, []string{
		"failed to extract PDF cover, continuing without it",
		"failed to extract PDF outline, continuing without chapters",
	}, messages)
}

func TestParse_OutwaitsHeldPdfiumInstance(t *testing.T) {
	path := writeCoverlessPDFWithOutline(t)

	// A caller with a wait shorter than the hold gives up. The Scan wait
	// stays well above the hold.
	const shortWait = 50 * time.Millisecond
	const hold = 500 * time.Millisecond
	setScanPdfiumTimeout(t, 30*time.Second)
	release := holdPdfiumInstance(t)

	// Sanity check: a short wait really does fail while the instance is held.
	_, err := PdfiumInstance(shortWait)
	require.Error(t, err, "a short wait should time out while the instance is held")

	go func() {
		time.Sleep(hold)
		release()
	}()

	start := time.Now()
	meta, err := Parse(path)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, time.Since(start), hold-shortWait,
		"Parse should have waited for the held instance")

	assert.NotEmpty(t, meta.CoverData, "Scan-path cover render should outwait the hold")
	assert.Equal(t, "image/jpeg", meta.CoverMimeType)
	require.Len(t, meta.Chapters, 2, "Scan-path outline extraction should outwait the hold")
	assert.Equal(t, "Introduction", meta.Chapters[0].Title)
	assert.Equal(t, "Main Content", meta.Chapters[1].Title)
}

func TestRenderPageJPEG_UsesScanWait(t *testing.T) {
	path := writeCoverlessPDFWithOutline(t)

	_ = holdPdfiumInstance(t)
	setScanPdfiumTimeout(t, 50*time.Millisecond)

	start := time.Now()
	_, _, err := RenderPageJPEG(path, 0, 72, 85)
	require.Error(t, err, "render should time out while the instance is held")
	assert.Less(t, time.Since(start), fixedTimeoutBound,
		"RenderPageJPEG runs during Scans and should wait scanPdfiumTimeout, not a fixed wait")
}
