package server

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/config"
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/filegen"
	"github.com/shishobooks/shisho/pkg/filegen/filegentest"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/sharelinks"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/shishobooks/shisho/pkg/worker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// streamingFixture serves the real route table over a real HTTP server.
// From newStreamingFixture, every generator of its download cache is a
// filegentest.Pausing the test releases; from newDownloadFixture, the cache
// uses the real generators.
type streamingFixture struct {
	t        *testing.T
	db       *bun.DB
	ts       *httptest.Server
	gen      *filegentest.Pausing
	cache    *downloadcache.Cache
	cacheDir string
	admin    *models.User
	token    string
	book     *models.Book
	file     *models.File
	original []byte
}

const streamingPassword = "test-password-123"

func newStreamingFixture(t *testing.T, gen *filegentest.Pausing) *streamingFixture {
	t.Helper()
	f := newDownloadFixture(t, func(dir string) (string, string) {
		return testgen.GenerateEPUB(t, dir, "Streamed.epub", testgen.EPUBOptions{Title: "Streamed"}), models.FileTypeEPUB
	}, downloadcache.WithGenerators(func(string, string) (filegen.StreamGenerator, error) {
		return gen, nil
	}))
	f.gen = gen
	// Cleanups run last first: release the generator so jobs end before
	// the fixture waits for them.
	t.Cleanup(gen.Release)
	return f
}

// newDownloadFixture serves the real route table with one book whose only
// file addFile writes into the book's directory, returning its path and
// type. Without cache options the cache uses the real generators.
func newDownloadFixture(t *testing.T, addFile func(dir string) (path, fileType string), opts ...downloadcache.Option) *streamingFixture {
	t.Helper()
	ctx := context.Background()
	db := testdb.New(t)
	cfg := newPermissionTestConfig(t)
	authSvc := auth.NewService(db, cfg.JWTSecret, cfg.SessionDuration())
	admin, err := authSvc.CreateFirstAdmin(ctx, "admin", nil, streamingPassword)
	require.NoError(t, err)
	token, err := authSvc.GenerateToken(admin)
	require.NoError(t, err)

	cacheDir := t.TempDir()
	cache := downloadcache.NewCache(cacheDir, 1<<30, opts...)
	srv, err := New(cfg, db, worker.New(&config.Config{WorkerProcesses: 1}, db, nil, nil, nil, nil, nil, nil), nil, nil, nil, cache, nil, nil, nil)
	require.NoError(t, err)
	ts := httptest.NewServer(srv.Handler)
	// Cleanups run last first: stop the server, then wait for the jobs
	// before the directories go.
	t.Cleanup(cache.Wait)
	t.Cleanup(ts.Close)

	lib := &models.Library{Name: "Lib", CoverAspectRatio: "book"}
	_, err = db.NewInsert().Model(lib).Exec(ctx)
	require.NoError(t, err)
	dir := t.TempDir()
	book := &models.Book{
		LibraryID:       lib.ID,
		Title:           "Streamed",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Streamed",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        dir,
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)
	path, fileType := addFile(dir)
	file := &models.File{LibraryID: lib.ID, BookID: book.ID, FileType: fileType, FileRole: models.FileRoleMain, Filepath: path, FilesizeBytes: 1}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)
	original, err := os.ReadFile(path)
	require.NoError(t, err)

	return &streamingFixture{
		t: t, db: db, ts: ts, cache: cache, cacheDir: cacheDir,
		admin: admin, token: token, book: book, file: file, original: original,
	}
}

// route is one download route and how to authenticate to it.
type route struct {
	name   string
	device bool
	kepub  bool
	// strict marks a route that never falls back to the original from a
	// failed Conversion: a Download as entry, or an OPDS download delivered
	// through a Conversion.
	strict bool
	path   func(f *streamingFixture) string
	auth   func(f *streamingFixture, req *http.Request)
}

func opdsRoute() route {
	return route{
		name:   "opds",
		device: true,
		path:   func(f *streamingFixture) string { return fmt.Sprintf("/opds/download/%d", f.file.ID) },
		auth:   func(_ *streamingFixture, req *http.Request) { req.SetBasicAuth("admin", streamingPassword) },
	}
}

func opdsKepubRoute() route {
	r := opdsRoute()
	r.name = "opds kepub"
	r.kepub = true
	r.strict = true
	r.path = func(f *streamingFixture) string { return fmt.Sprintf("/opds/download/%d.kepub.epub", f.file.ID) }
	return r
}

func webRoute() route {
	return route{
		name: "web",
		path: func(f *streamingFixture) string { return fmt.Sprintf("/api/books/files/%d/download", f.file.ID) },
		auth: func(f *streamingFixture, req *http.Request) {
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: f.token})
		},
	}
}

func webKepubRoute() route {
	r := webRoute()
	r.name = "web kepub"
	r.kepub = true
	r.path = func(f *streamingFixture) string { return fmt.Sprintf("/api/books/files/%d/download/kepub", f.file.ID) }
	return r
}

func shareKepubRoute() route {
	r := shareRoute()
	r.name = "share link kepub"
	r.kepub = true
	r.strict = true
	r.path = func(f *streamingFixture) string {
		return fmt.Sprintf("/api/share/%s/files/%d/download/kepub", f.shareLink().Token, f.file.ID)
	}
	return r
}

func shareRoute() route {
	return route{
		name:   "share link",
		device: true,
		path: func(f *streamingFixture) string {
			return fmt.Sprintf("/api/share/%s/files/%d/download", f.shareLink().Token, f.file.ID)
		},
		auth: func(*streamingFixture, *http.Request) {},
	}
}

// request sends a request on r and returns the response with its body
// unread. The caller closes it.
func (f *streamingFixture) request(r route, method string, header ...string) *http.Response {
	f.t.Helper()
	req, err := http.NewRequestWithContext(f.t.Context(), method, f.ts.URL+r.path(f), nil)
	require.NoError(f.t, err)
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	r.auth(f, req)
	res, err := f.ts.Client().Do(req)
	require.NoError(f.t, err)
	return res
}

// api sends an authenticated JSON request to the API as the admin.
func (f *streamingFixture) api(method, path, body string) []byte {
	f.t.Helper()
	req, err := http.NewRequestWithContext(f.t.Context(), method, f.ts.URL+path, strings.NewReader(body))
	require.NoError(f.t, err)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: f.token})
	res, err := f.ts.Client().Do(req)
	require.NoError(f.t, err)
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	require.NoError(f.t, err)
	require.Less(f.t, res.StatusCode, 300, string(data))
	return data
}

// shareLink turns sharing on and creates a link to the book, once.
func (f *streamingFixture) shareLink() sharelinks.ShareLinkResponse {
	f.t.Helper()
	var link sharelinks.ShareLinkResponse
	rows, err := f.db.NewSelect().Model((*models.ShareLink)(nil)).Count(context.Background())
	require.NoError(f.t, err)
	if rows == 0 {
		f.api(http.MethodPut, "/api/settings/sharing", `{"enabled":true,"require_expiration":false}`)
		require.NoError(f.t, json.Unmarshal(f.api(http.MethodPost, fmt.Sprintf("/api/books/%d/share-links", f.book.ID), `{}`), &link))
		return link
	}
	var row models.ShareLink
	require.NoError(f.t, f.db.NewSelect().Model(&row).Limit(1).Scan(context.Background()))
	link.Token = row.Token
	link.ID = row.ID
	return link
}

func readExactly(t *testing.T, r io.Reader, n int) string {
	t.Helper()
	buf := make([]byte, n)
	_, err := io.ReadFull(r, buf)
	require.NoError(t, err)
	return string(buf)
}

func (f *streamingFixture) cacheEntries() []string {
	f.t.Helper()
	entries, err := os.ReadDir(f.cacheDir)
	require.NoError(f.t, err)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// Headers and the first bytes reach the client before generation finishes,
// on every kind of download route, and a second request at the same time
// gets bytes before it finishes too. Both receive the whole file, built
// once, with a Content-Length that matches.
func TestGeneratedDownload_StreamsBeforeGenerationFinishes(t *testing.T) {
	t.Parallel()
	for _, r := range []route{opdsRoute(), webRoute(), webKepubRoute(), shareRoute()} {
		t.Run(r.name, func(t *testing.T) {
			t.Parallel()
			f := newStreamingFixture(t, filegentest.NewPausing("first bytes,", " the rest"))
			want := string(f.gen.Output())

			a := f.request(r, http.MethodGet)
			defer a.Body.Close()
			require.Equal(t, http.StatusOK, a.StatusCode)
			assert.Equal(t, strconv.Itoa(len(want)), a.Header.Get("Content-Length"))
			assert.Equal(t, "private, no-store", a.Header.Get("Cache-Control"))
			assert.Contains(t, a.Header.Get("Content-Disposition"), "attachment")
			assert.Equal(t, "first bytes,", readExactly(t, a.Body, len(f.gen.First)))

			b := f.request(r, http.MethodGet)
			defer b.Body.Close()
			require.Equal(t, http.StatusOK, b.StatusCode)
			assert.Equal(t, "first bytes,", readExactly(t, b.Body, len(f.gen.First)))

			f.gen.Release()
			restA, err := io.ReadAll(a.Body)
			require.NoError(t, err)
			restB, err := io.ReadAll(b.Body)
			require.NoError(t, err)
			assert.Equal(t, want, "first bytes,"+string(restA))
			assert.Equal(t, want, "first bytes,"+string(restB))
			assert.Equal(t, 1, f.gen.Writes())
		})
	}
}

// A size not known up front goes out chunked, with no Content-Length.
func TestGeneratedDownload_UnknownSizeIsChunked(t *testing.T) {
	t.Parallel()
	gen := filegentest.NewPausing("first bytes,", " the rest")
	gen.UnknownSize = true
	f := newStreamingFixture(t, gen)

	res := f.request(opdsRoute(), http.MethodGet)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
	assert.Equal(t, int64(-1), res.ContentLength)
	assert.Equal(t, []string{"chunked"}, res.TransferEncoding)
	f.gen.Release()
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	assert.Equal(t, f.gen.Output(), body)
}

// A client that disconnects mid-download leaves the build running: the file
// lands in the cache and the next request is served from there, with Range
// support.
func TestGeneratedDownload_DisconnectStillCaches(t *testing.T) {
	t.Parallel()
	f := newStreamingFixture(t, filegentest.NewPausing("first bytes,", " the rest"))

	res := f.request(opdsRoute(), http.MethodGet)
	readExactly(t, res.Body, len(f.gen.First))
	require.NoError(t, res.Body.Close())

	f.gen.Release()
	f.cache.Wait()

	again := f.request(opdsRoute(), http.MethodGet, "Range", "bytes=6-")
	defer again.Body.Close()
	require.Equal(t, http.StatusPartialContent, again.StatusCode)
	body, err := io.ReadAll(again.Body)
	require.NoError(t, err)
	assert.Equal(t, string(f.gen.Output()[6:]), string(body))
	assert.Equal(t, 1, f.gen.Writes(), "the cached file was built again")
}

// A Range request against a file still being built gets the whole file with
// a 200.
func TestGeneratedDownload_RangeOnPartialFileGetsWholeFile(t *testing.T) {
	t.Parallel()
	f := newStreamingFixture(t, filegentest.NewPausing("first bytes,", " the rest"))

	res := f.request(opdsRoute(), http.MethodGet, "Range", "bytes=6-")
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
	f.gen.Release()
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	assert.Equal(t, f.gen.Output(), body)
}

// HEAD runs only the preparation step: it answers with the real status,
// filename, and size, starts no generation, and creates no partial file.
func TestGeneratedDownload_HeadPreparesOnly(t *testing.T) {
	t.Parallel()
	for _, r := range []route{opdsRoute(), webRoute(), shareRoute()} {
		t.Run(r.name, func(t *testing.T) {
			t.Parallel()
			f := newStreamingFixture(t, filegentest.NewPausing("first bytes,", " the rest"))

			res := f.request(r, http.MethodHead)
			require.NoError(t, res.Body.Close())
			require.Equal(t, http.StatusOK, res.StatusCode)
			assert.Equal(t, strconv.Itoa(len(f.gen.Output())), res.Header.Get("Content-Length"))
			assert.Contains(t, res.Header.Get("Content-Disposition"), "Streamed")
			assert.Equal(t, 1, f.gen.Prepares())
			assert.Equal(t, 0, f.gen.Writes())
			assert.Empty(t, f.cacheEntries())
		})
	}
}

// A preparation failure behaves as before streaming: device routes serve the
// Original File, and web UI routes return the error, for GET and HEAD.
func TestGeneratedDownload_PrepareFailure(t *testing.T) {
	t.Parallel()
	for _, r := range []route{opdsRoute(), shareRoute(), webRoute()} {
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			t.Run(r.name+" "+method, func(t *testing.T) {
				t.Parallel()
				gen := filegentest.NewPausing("first bytes,", " the rest")
				gen.PrepareErr = filegen.NewGenerationError(models.FileTypeEPUB, nil, "broken")
				f := newStreamingFixture(t, gen)

				res := f.request(r, method)
				defer res.Body.Close()
				body, err := io.ReadAll(res.Body)
				require.NoError(t, err)
				if r.device {
					require.Equal(t, http.StatusOK, res.StatusCode)
					assert.Contains(t, res.Header.Get("Content-Disposition"), "Streamed.epub")
					if method == http.MethodGet {
						assert.Equal(t, f.original, body)
					}
				} else {
					require.Equal(t, http.StatusInternalServerError, res.StatusCode)
					assert.Empty(t, res.Header.Get("Content-Disposition"))
				}
				assert.Equal(t, 0, f.gen.Writes())
				assert.Empty(t, f.cacheEntries())
			})
		}
	}
}

// A Share Link download is counted once the GET starts sending, before the
// file is complete.
func TestGeneratedDownload_ShareLinkCountedWhenStarted(t *testing.T) {
	t.Parallel()
	f := newStreamingFixture(t, filegentest.NewPausing("first bytes,", " the rest"))
	link := f.shareLink()
	count := func() int {
		var row models.ShareLink
		require.NoError(t, f.db.NewSelect().Model(&row).Where("id = ?", link.ID).Scan(context.Background()))
		return row.DownloadCount
	}

	head := f.request(shareRoute(), http.MethodHead)
	require.NoError(t, head.Body.Close())
	assert.Equal(t, 0, count(), "a HEAD counted")

	res := f.request(shareRoute(), http.MethodGet)
	defer res.Body.Close()
	readExactly(t, res.Body, len(f.gen.First))
	assert.Equal(t, 1, count(), "the download was not counted when it started")

	f.gen.Release()
	_, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	assert.Equal(t, 1, count())
}

// A build that fails after bytes have been sent cuts the connection, with or
// without a Content-Length, so the client sees a failed download rather than
// a short file that ends cleanly. The failure is remembered: device routes
// then serve the Original File and web UI routes return the error, for GET
// and HEAD, without building again, until the book's metadata changes.
func TestGeneratedDownload_MidStreamFailure(t *testing.T) {
	t.Parallel()
	for _, r := range []route{opdsRoute(), shareRoute(), webRoute(), webKepubRoute()} {
		for _, unknownSize := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s unknown size %t", r.name, unknownSize), func(t *testing.T) {
				t.Parallel()
				gen := filegentest.NewPausing("first bytes,", " the rest")
				gen.UnknownSize = unknownSize
				gen.WriteErr = errors.New("page 3 is corrupt")
				f := newStreamingFixture(t, gen)

				res := f.request(r, http.MethodGet)
				require.Equal(t, http.StatusOK, res.StatusCode)
				assert.Equal(t, "first bytes,", readExactly(t, res.Body, len(gen.First)))
				gen.Release()
				_, err := io.ReadAll(res.Body)
				require.Error(t, err, "the cut download looked complete")
				require.NoError(t, res.Body.Close())
				f.cache.Wait()

				for _, method := range []string{http.MethodGet, http.MethodHead} {
					again := f.request(r, method)
					body, err := io.ReadAll(again.Body)
					require.NoError(t, err)
					require.NoError(t, again.Body.Close())
					if r.device {
						require.Equal(t, http.StatusOK, again.StatusCode, method)
						assert.Contains(t, again.Header.Get("Content-Disposition"), "Streamed.epub")
						if method == http.MethodGet {
							assert.Equal(t, f.original, body)
						}
					} else {
						// The failure was logged when it happened, so the
						// web UI gets a 422 rather than another server error.
						require.Equal(t, http.StatusUnprocessableEntity, again.StatusCode, method)
						assert.Empty(t, again.Header.Get("Content-Disposition"))
						if method == http.MethodGet {
							assertFailedPartwayBody(t, body)
						}
					}
				}
				assert.Equal(t, 1, gen.Writes(), "a remembered failure was built again")

				_, err = f.db.NewUpdate().Model((*models.Book)(nil)).Set("title = ?", "Fixed").Where("id = ?", f.book.ID).Exec(context.Background())
				require.NoError(t, err)
				gen.WriteErr = nil
				fixed := f.request(r, http.MethodGet)
				body, err := io.ReadAll(fixed.Body)
				require.NoError(t, err)
				require.NoError(t, fixed.Body.Close())
				require.Equal(t, http.StatusOK, fixed.StatusCode)
				assert.Equal(t, gen.Output(), body)
				assert.Equal(t, 2, gen.Writes())
			})
		}
	}
}

// A real CBZ with a page that cannot be read midway fails partway through
// its first download, as a CBZ or a KePub: the client sees a cut download.
// Then device routes serve the Original File of a CBZ, OPDS KePub downloads
// and a Share Link's Download as entry return an error, since a failed
// Conversion never serves another File Type, and web UI routes return the
// error.
func TestGeneratedDownload_CorruptCBZPageFailsMidStream(t *testing.T) {
	t.Parallel()
	for _, r := range []route{opdsRoute(), webRoute(), opdsKepubRoute(), webKepubRoute(), shareKepubRoute()} {
		t.Run(r.name, func(t *testing.T) {
			t.Parallel()
			builtIn, err := filegen.GetGenerator(models.FileTypeCBZ)
			if r.kepub {
				builtIn, err = filegen.GeneratorFor(models.FileTypeCBZ, models.FileTypeKePub)
			}
			require.NoError(t, err)
			sg, ok := builtIn.(filegen.StreamGenerator)
			require.True(t, ok, "the generator does not stream")
			gen := &gatedGenerator{StreamGenerator: sg, gate: make(chan struct{})}
			f := newDownloadFixture(t, func(dir string) (string, string) {
				return testgen.GenerateCorruptCBZ(t, dir, "Streamed.cbz", 40, 20), models.FileTypeCBZ
			}, downloadcache.WithGenerators(func(string, string) (filegen.StreamGenerator, error) {
				return gen, nil
			}))
			t.Cleanup(gen.release)

			res := f.request(r, http.MethodGet)
			require.Equal(t, http.StatusOK, res.StatusCode)
			// The download is attached, so the failure reaches it partway.
			gen.release()
			assert.Empty(t, res.Header.Get("Content-Length"))
			body, err := io.ReadAll(res.Body)
			require.Error(t, err, "the cut download looked complete")
			assert.NotEmpty(t, body, "the download failed before any page was sent")
			require.NoError(t, res.Body.Close())
			f.cache.Wait()

			again := f.request(r, http.MethodGet)
			body, err = io.ReadAll(again.Body)
			require.NoError(t, err)
			require.NoError(t, again.Body.Close())
			switch {
			case r.strict:
				// The route asked for this File Type, and the original is
				// another.
				require.Equal(t, http.StatusInternalServerError, again.StatusCode)
				assert.NotEqual(t, f.original, body)
				assert.Empty(t, again.Header.Get("Content-Disposition"))
			case r.device:
				require.Equal(t, http.StatusOK, again.StatusCode)
				assert.Equal(t, f.original, body)
			default:
				require.Equal(t, http.StatusUnprocessableEntity, again.StatusCode)
				assertFailedPartwayBody(t, body)
			}
		})
	}
}

// A PDF that needs a password to open cannot get Shisho's metadata: device
// routes serve the Original File, and web UI routes answer with an
// invalid_state error the download dialog shows beside Download Original,
// not a server error.
func TestGeneratedDownload_PasswordProtectedPDF(t *testing.T) {
	t.Parallel()
	for _, r := range []route{opdsRoute(), shareRoute(), webRoute()} {
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			t.Run(r.name+" "+method, func(t *testing.T) {
				t.Parallel()
				f := newDownloadFixture(t, func(dir string) (string, string) {
					return testgen.GeneratePDF(t, dir, "Locked.pdf", testgen.PDFOptions{UserPassword: "secret"}), models.FileTypePDF
				})

				res := f.request(r, method)
				defer res.Body.Close()
				body, err := io.ReadAll(res.Body)
				require.NoError(t, err)
				if r.device {
					require.Equal(t, http.StatusOK, res.StatusCode)
					if method == http.MethodGet {
						assert.Equal(t, f.original, body)
					}
					return
				}
				require.Equal(t, http.StatusUnprocessableEntity, res.StatusCode)
				if method == http.MethodGet {
					var errRes errcodes.ErrorResponse
					require.NoError(t, json.Unmarshal(body, &errRes), string(body))
					assert.Equal(t, "invalid_state", errRes.Error.Code)
					assert.Equal(t, books.CannotDecryptMessage, errRes.Error.Message)
				}
			})
		}
	}
}

// gatedGenerator is a real generator whose output after its first write
// waits until the test releases it, so a download is attached before the
// build gets far.
type gatedGenerator struct {
	filegen.StreamGenerator
	gate chan struct{}
	once sync.Once
}

func (g *gatedGenerator) release() { g.once.Do(func() { close(g.gate) }) }

func (g *gatedGenerator) Prepare(ctx context.Context, src filegen.Source, book *models.Book, file *models.File) (filegen.Prepared, error) {
	p, err := g.StreamGenerator.Prepare(ctx, src, book, file)
	if err != nil {
		return nil, err
	}
	return gatedPrepared{Prepared: p, gate: g.gate}, nil
}

type gatedPrepared struct {
	filegen.Prepared
	gate chan struct{}
}

func (p gatedPrepared) Write(ctx context.Context, w io.Writer) error {
	return p.Prepared.Write(ctx, &gatedWriter{ctx: ctx, w: w, gate: p.gate})
}

type gatedWriter struct {
	ctx    context.Context
	w      io.Writer
	gate   chan struct{}
	passed bool
}

func (g *gatedWriter) Write(b []byte) (int, error) {
	if g.passed {
		select {
		case <-g.gate:
		case <-g.ctx.Done():
			return 0, g.ctx.Err()
		}
	}
	g.passed = true
	return g.w.Write(b)
}

// assertFailedPartwayBody checks a web UI route's answer to a remembered
// failure: an invalid_state error whose message the download dialog shows.
func assertFailedPartwayBody(t *testing.T, body []byte) {
	t.Helper()
	var res errcodes.ErrorResponse
	require.NoError(t, json.Unmarshal(body, &res), string(body))
	assert.Equal(t, "invalid_state", res.Error.Code)
	assert.Equal(t, books.FailedPartwayMessage, res.Error.Message)
}

// A real KePub conversion that fails partway, on an EPUB whose chapter data
// is corrupt, cuts the download. Afterwards OPDS returns a 500, never the
// EPUB in the KePub's place, and the web route returns the error.
func TestGeneratedDownload_CorruptEPUBChapterFailsKepubMidStream(t *testing.T) {
	t.Parallel()
	for _, r := range []route{opdsKepubRoute(), webKepubRoute()} {
		t.Run(r.name, func(t *testing.T) {
			t.Parallel()
			gen := &gatedGenerator{StreamGenerator: kepubEPUBGenerator(t), gate: make(chan struct{})}
			f := newDownloadFixture(t, func(dir string) (string, string) {
				return writeEPUBWithCorruptChapter(t, dir), models.FileTypeEPUB
			}, downloadcache.WithGenerators(func(string, string) (filegen.StreamGenerator, error) {
				return gen, nil
			}))
			t.Cleanup(gen.release)

			res := f.request(r, http.MethodGet)
			require.Equal(t, http.StatusOK, res.StatusCode)
			// The download is attached, so the failure reaches it partway.
			gen.release()
			assert.Empty(t, res.Header.Get("Content-Length"))
			body, err := io.ReadAll(res.Body)
			require.Error(t, err, "the cut download looked complete")
			assert.NotEmpty(t, body, "the download failed before any entry was sent")
			require.NoError(t, res.Body.Close())
			f.cache.Wait()

			again := f.request(r, http.MethodGet)
			body, err = io.ReadAll(again.Body)
			require.NoError(t, err)
			require.NoError(t, again.Body.Close())
			switch {
			case r.strict:
				// The route asked for this File Type, and the original is
				// another.
				require.Equal(t, http.StatusInternalServerError, again.StatusCode)
				assert.NotEqual(t, f.original, body)
				assert.Empty(t, again.Header.Get("Content-Disposition"))
			case r.device:
				require.Equal(t, http.StatusOK, again.StatusCode)
				assert.Equal(t, f.original, body)
			default:
				require.Equal(t, http.StatusUnprocessableEntity, again.StatusCode)
				assertFailedPartwayBody(t, body)
			}
		})
	}
}

// writeEPUBWithCorruptChapter writes an EPUB into dir whose chapter has
// compressed data that is not valid deflate, and returns its path. A large
// stored entry comes before the chapter, so the conversion writes past
// gatedGenerator's gate before it reaches the chapter and fails.
func writeEPUBWithCorruptChapter(t *testing.T, dir string) string {
	t.Helper()
	const chapter = "OEBPS/chapter1.xhtml"
	src, err := zip.OpenReader(testgen.GenerateEPUB(t, t.TempDir(), "Streamed.epub", testgen.EPUBOptions{Title: "Streamed"}))
	require.NoError(t, err)
	defer src.Close()

	path := filepath.Join(dir, "Streamed.epub")
	out, err := os.Create(path)
	require.NoError(t, err)
	zw := zip.NewWriter(out)
	for _, f := range src.File {
		if f.Name == chapter {
			w, err := zw.CreateHeader(&zip.FileHeader{Name: "OEBPS/plate.bin", Method: zip.Store})
			require.NoError(t, err)
			_, err = w.Write(bytes.Repeat([]byte{0x5a}, 64*1024))
			require.NoError(t, err)
		}
		require.NoError(t, zw.Copy(f))
	}
	require.NoError(t, zw.Close())
	require.NoError(t, out.Close())

	zr, err := zip.OpenReader(path)
	require.NoError(t, err)
	var offset, size int64
	for _, f := range zr.File {
		if f.Name == chapter {
			require.Equal(t, zip.Deflate, f.Method)
			offset, err = f.DataOffset()
			require.NoError(t, err)
			size = int64(f.CompressedSize64)
		}
	}
	require.NoError(t, zr.Close())
	require.NotZero(t, size)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	for i := offset; i < offset+size; i++ {
		data[i] = 0xff
	}
	require.NoError(t, os.WriteFile(path, data, 0o600))
	return path
}

func kepubEPUBGenerator(t *testing.T) filegen.StreamGenerator {
	t.Helper()
	gen, err := filegen.GeneratorFor(models.FileTypeEPUB, models.FileTypeKePub)
	require.NoError(t, err)
	sg, ok := gen.(filegen.StreamGenerator)
	require.True(t, ok)
	return sg
}
