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

// fakePlugins stands in for the plugin manager: the extensions file parsers
// read, each with the MIME types its parser declares, and the extensions
// input converters read, each with the type it converts to.
type fakePlugins struct {
	parsers    map[string][]string
	converters map[string]string
}

func (f fakePlugins) FileParserMIMETypes() map[string][]string { return f.parsers }
func (f fakePlugins) ConverterTargetTypes() map[string]string  { return f.converters }

func TestDetect_CountsFileTypesPluginsParse(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for _, rel := range []string{
		"Books/Emma.epub",
		"Books/Emma.fb2",
		"Audio/Dune.MP3",
		"Comics/Saga.cbr",
		// No plugin parses these.
		"notes.txt",
		"Audio/cover.jpg",
	} {
		touch(t, root, rel)
	}
	h := newDetectHandler()
	h.plugins = fakePlugins{parsers: map[string][]string{
		"fb2": {"application/x-fictionbook+xml"},
		"mp3": {"audio/mpeg"},
		"cbr": {"application/vnd.comicbook-rar"},
	}}

	status, resp, body := detect(t, h, pathsBody(t, root))

	require.Equal(t, http.StatusOK, status, body)
	assert.Equal(t, DetectFileTypesResponse{Ebooks: 2, Audiobooks: 1, Comics: 1}, resp)
}

func TestDetect_CountsFileTypesPluginsConvert(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for _, rel := range []string{
		"Books/Emma.docx",
		"Comics/Saga.cb7",
		"Audio/Dune.aax",
	} {
		touch(t, root, rel)
	}
	h := newDetectHandler()
	h.plugins = fakePlugins{converters: map[string]string{
		"docx": "epub",
		"cb7":  "cbz",
		"aax":  "m4b",
	}}

	status, resp, body := detect(t, h, pathsBody(t, root))

	require.Equal(t, http.StatusOK, status, body)
	assert.Equal(t, DetectFileTypesResponse{Ebooks: 1, Audiobooks: 1, Comics: 1}, resp)
}

func TestDetect_CountsComicArchivesAsComicsWhateverTheirMIMEType(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	touch(t, root, "Saga.cbr")
	touch(t, root, "Saga 2.cbt")
	h := newDetectHandler()
	h.plugins = fakePlugins{parsers: map[string][]string{
		"cbr": {"application/x-rar-compressed"},
		"cbt": {"application/x-tar"},
	}}

	status, resp, body := detect(t, h, pathsBody(t, root))

	require.Equal(t, http.StatusOK, status, body)
	assert.Equal(t, DetectFileTypesResponse{Comics: 2}, resp)
}

func TestDetect_MatchesPluginTypesAsTheScannerDoes(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	touch(t, root, "Emma.fb2")
	h := newDetectHandler()
	// The scanner compares a file's lowercased extension with the type as
	// the manifest spells it, so it never imports these files.
	h.plugins = fakePlugins{parsers: map[string][]string{"FB2": nil}}

	status, resp, body := detect(t, h, pathsBody(t, root))

	require.Equal(t, http.StatusOK, status, body)
	assert.Equal(t, DetectFileTypesResponse{}, resp)
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

func TestDetect_NamesFoldersThatDontExist(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	touch(t, root, "a.epub")
	touch(t, root, "file.txt")
	missing := filepath.Join(root, "missing")
	notAFolder := filepath.Join(root, "file.txt")

	// A library made from a path that isn't a folder never scans, so Setup
	// names each one; the others are still counted.
	status, resp, body := detect(t, newDetectHandler(), pathsBody(t, missing, root, notAFolder))

	require.Equal(t, http.StatusOK, status, body)
	assert.Equal(t, DetectFileTypesResponse{Ebooks: 1, MissingPaths: []string{missing, notAFolder}}, resp)
}

func TestDetect_NamesAFolderInsideOneThatCantBeOpened(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root opens every folder")
	}
	parent := t.TempDir()
	locked := filepath.Join(parent, "locked")
	inside := filepath.Join(locked, "books")
	require.NoError(t, os.MkdirAll(inside, 0o755))
	require.NoError(t, os.Chmod(locked, 0o000))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	status, resp, body := detect(t, newDetectHandler(), pathsBody(t, inside))

	require.Equal(t, http.StatusOK, status, body)
	assert.Equal(t, []string{inside}, resp.MissingPaths)
}

// A folder the server finds but can't read, as when a container runs as a
// user the folder doesn't let in, never scans either.
func TestDetect_NamesAFolderThatCantBeRead(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root reads every folder")
	}
	locked := filepath.Join(t.TempDir(), "books")
	require.NoError(t, os.Mkdir(locked, 0o755))
	touch(t, locked, "a.epub")
	require.NoError(t, os.Chmod(locked, 0o000))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	status, resp, body := detect(t, newDetectHandler(), pathsBody(t, locked))

	require.Equal(t, http.StatusOK, status, body)
	assert.Equal(t, DetectFileTypesResponse{MissingPaths: []string{locked}}, resp)
}

// Checking the folders alone answers without walking them, so Setup can
// check them on Continue without waiting on a large library.
func TestDetect_FoldersOnlySkipsTheWalk(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	touch(t, root, "a.epub")
	missing := filepath.Join(root, "missing")
	h := newDetectHandler()
	h.detectBudget.walk = func(string, fs.WalkDirFunc) error {
		t.Error("the folders-only check walked a folder")
		return nil
	}

	body, err := json.Marshal(DetectFileTypesPayload{Paths: []string{root, missing}, FoldersOnly: true})
	require.NoError(t, err)
	status, resp, raw := detect(t, h, string(body))

	require.Equal(t, http.StatusOK, status, raw)
	assert.Equal(t, DetectFileTypesResponse{MissingPaths: []string{missing}}, resp)
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

	_, err := NewService().DetectFileTypes(ctx, []string{root}, defaultDetectLimits, nil)

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
