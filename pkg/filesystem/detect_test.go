package filesystem

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/binder"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// touch creates an empty file at root/rel, with any parent directories.
func touch(t *testing.T, root, rel string) {
	t.Helper()
	path := filepath.Join(root, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, nil, 0o644))
}

func newDetectEcho(t *testing.T) *echo.Echo {
	t.Helper()
	e := echo.New()
	b, err := binder.New()
	require.NoError(t, err)
	e.Binder = b
	e.HTTPErrorHandler = errcodes.NewHandler().Handle
	return e
}

// detect posts paths to the handler h and returns the status and response.
func detect(t *testing.T, h *handler, body string) (int, DetectFileTypesResponse, string) {
	t.Helper()
	e := newDetectEcho(t)
	e.POST("/filesystem/detect", h.detect)
	req := httptest.NewRequest(http.MethodPost, "/filesystem/detect", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	var resp DetectFileTypesResponse
	if rec.Code == http.StatusOK {
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp), rec.Body.String())
	}
	return rec.Code, resp, rec.Body.String()
}

func pathsBody(t *testing.T, paths ...string) string {
	t.Helper()
	b, err := json.Marshal(DetectFileTypesPayload{Paths: paths})
	require.NoError(t, err)
	return string(b)
}

func newDetectHandler() *handler {
	return &handler{
		filesystemService: NewService(),
		detectBudget:      defaultDetectLimits,
	}
}

func TestDetect_CountsRecognizedFilesByCategory(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for _, rel := range []string{
		"Austen/Pride and Prejudice.epub",
		"Austen/Emma.kepub.epub",
		"Kindle/Dune.azw3",
		"Kindle/Old.MOBI",
		"Papers/Report.pdf",
		"Audio/Dune.m4b",
		"Audio/Emma.m4a",
		"Manga/Vol 1.cbz",
		// Not recognized: covers, sidecars, and anything else.
		"Austen/cover.jpg",
		"Austen/Pride and Prejudice.metadata.json",
		"notes.txt",
	} {
		touch(t, root, rel)
	}

	status, resp, body := detect(t, newDetectHandler(), pathsBody(t, root))

	require.Equal(t, http.StatusOK, status, body)
	assert.Equal(t, DetectFileTypesResponse{Ebooks: 5, Audiobooks: 2, Comics: 1}, resp)
}

func TestDetect_SumsEveryPathAndCountsAnOverlapOnce(t *testing.T) {
	t.Parallel()
	books := t.TempDir()
	audio := t.TempDir()
	touch(t, books, "a.epub")
	touch(t, books, "nested/b.epub")
	touch(t, audio, "c.m4b")

	// The nested path lies inside the first, so its file is counted once.
	status, resp, body := detect(t, newDetectHandler(), pathsBody(t, books, audio, filepath.Join(books, "nested")))

	require.Equal(t, http.StatusOK, status, body)
	assert.Equal(t, DetectFileTypesResponse{Ebooks: 2, Audiobooks: 1}, resp)
}

func TestDetect_FileBudgetStopsEarly(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for _, name := range []string{"a.epub", "b.epub", "c.epub", "d.epub", "e.epub"} {
		touch(t, root, name)
	}
	h := newDetectHandler()
	h.detectBudget.Files = 3

	status, resp, body := detect(t, h, pathsBody(t, root))

	require.Equal(t, http.StatusOK, status, body)
	assert.Equal(t, DetectFileTypesResponse{Ebooks: 3, StoppedEarly: true}, resp)
}

func TestDetect_FileBudgetReachedExactlyIsNotEarly(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for _, name := range []string{"a.epub", "b.epub", "c.epub"} {
		touch(t, root, name)
	}
	h := newDetectHandler()
	h.detectBudget.Files = 3

	status, resp, body := detect(t, h, pathsBody(t, root))

	require.Equal(t, http.StatusOK, status, body)
	assert.Equal(t, DetectFileTypesResponse{Ebooks: 3}, resp, "every file was looked at")
}

func TestDetect_TimeBudgetStopsEarly(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for _, name := range []string{"a.epub", "b.epub", "c.epub", "d.epub", "e.epub"} {
		touch(t, root, name)
	}
	// Each reading of the clock moves it a second, so a budget of two and a
	// half seconds runs out partway through the walk.
	clock := time.Unix(0, 0)
	h := newDetectHandler()
	h.detectBudget.Duration = 2500 * time.Millisecond
	h.detectBudget.now = func() time.Time {
		clock = clock.Add(time.Second)
		return clock
	}

	status, resp, body := detect(t, h, pathsBody(t, root))

	require.Equal(t, http.StatusOK, status, body)
	assert.True(t, resp.StoppedEarly)
	assert.Less(t, resp.Ebooks, 5, "the walk stopped before the last file")
}

func TestDetect_MissingPathCountsNothing(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	touch(t, root, "a.epub")

	// A folder that isn't mounted yet is what the enrichment step's "nothing
	// found" outcome is for, so it is not an error.
	status, resp, body := detect(t, newDetectHandler(), pathsBody(t, filepath.Join(root, "missing"), root))

	require.Equal(t, http.StatusOK, status, body)
	assert.Equal(t, DetectFileTypesResponse{Ebooks: 1}, resp)
}

func TestDetect_FollowsASymlinkedPath(t *testing.T) {
	t.Parallel()
	target := t.TempDir()
	touch(t, target, "a.cbz")
	link := filepath.Join(t.TempDir(), "link")
	require.NoError(t, os.Symlink(target, link))

	// The directory picker resolves symlinks, and so does detection.
	status, resp, body := detect(t, newDetectHandler(), pathsBody(t, link))

	require.Equal(t, http.StatusOK, status, body)
	assert.Equal(t, DetectFileTypesResponse{Comics: 1}, resp)
}

func TestDetect_SkipsAnUnreadableFolder(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root reads every folder")
	}
	root := t.TempDir()
	touch(t, root, "a.epub")
	touch(t, root, "locked/b.epub")
	locked := filepath.Join(root, "locked")
	require.NoError(t, os.Chmod(locked, 0o000))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	status, resp, body := detect(t, newDetectHandler(), pathsBody(t, root))

	require.Equal(t, http.StatusOK, status, body)
	assert.Equal(t, DetectFileTypesResponse{Ebooks: 1}, resp)
}

func TestDetect_RequiresAPath(t *testing.T) {
	t.Parallel()
	for _, body := range []string{`{"paths":[]}`, `{}`, `{"paths":[""]}`} {
		status, _, resp := detect(t, newDetectHandler(), body)
		assert.Equal(t, http.StatusUnprocessableEntity, status, "%s: %s", body, resp)
		assert.Contains(t, resp, "validation_error", body)
	}
}

// A folder whose read never returns, as on a hung network mount, must not
// hold Setup past the time budget: the walk is checked only between entries,
// so the deadline has to stop the request itself.
func TestDetect_TimeBudgetStopsAHungFolderRead(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	touch(t, root, "a.epub")
	info, err := os.Stat(filepath.Join(root, "a.epub"))
	require.NoError(t, err)
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })

	h := newDetectHandler()
	h.detectBudget.Duration = 50 * time.Millisecond
	h.detectBudget.walk = func(walkRoot string, fn fs.WalkDirFunc) error {
		if err := fn(filepath.Join(walkRoot, "a.epub"), fs.FileInfoToDirEntry(info), nil); err != nil {
			return err
		}
		<-release // the next folder's read hangs
		return nil
	}

	status, resp, body := detect(t, h, pathsBody(t, root))

	require.Equal(t, http.StatusOK, status, body)
	assert.Equal(t, DetectFileTypesResponse{Ebooks: 1, StoppedEarly: true}, resp, "the files found before the hang are kept")
}

func TestDetect_CanceledRequestStops(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	touch(t, root, "a.epub")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := NewService().DetectFileTypes(ctx, []string{root}, defaultDetectLimits)

	require.ErrorIs(t, err, context.Canceled)
}

func TestDetect_SkipsAppleDoubleFiles(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	touch(t, root, "Book.epub")
	// macOS writes these beside files on network shares; the scan rejects
	// them, so they aren't counted as books.
	touch(t, root, "._Book.epub")

	status, resp, body := detect(t, newDetectHandler(), pathsBody(t, root))

	require.Equal(t, http.StatusOK, status, body)
	assert.Equal(t, DetectFileTypesResponse{Ebooks: 1}, resp)
}
