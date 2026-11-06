//go:build koreaderharness

package main

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/cbzpages"
	"github.com/shishobooks/shisho/pkg/config"
	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/shishobooks/shisho/pkg/events"
	"github.com/shishobooks/shisho/pkg/fingerprint"
	"github.com/shishobooks/shisho/pkg/fingerprints"
	"github.com/shishobooks/shisho/pkg/joblogs"
	"github.com/shishobooks/shisho/pkg/kepub"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/pdfpages"
	"github.com/shishobooks/shisho/pkg/plugins"
	"github.com/shishobooks/shisho/pkg/reading"
	"github.com/shishobooks/shisho/pkg/reflowable"
	"github.com/shishobooks/shisho/pkg/server"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/shishobooks/shisho/pkg/worker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// scenario is a running Shisho with a scanned library, a KOReader Device, and
// a KOReader signed in to it.
type scenario struct {
	t            *testing.T
	ctx          context.Context
	db           *bun.DB
	url          string
	h            *Harness
	user         *models.User
	device       *devices.Device
	devices      *devices.Service
	reading      *reading.Service
	fingerprints *fingerprints.Service
	worker       *worker.Worker
	jobLog       *joblogs.JobLogger
	cache        *downloadcache.Cache
	library      string
}

func newScenario(t *testing.T) *scenario {
	t.Helper()
	ctx := logger.New().WithContext(context.Background())
	db := testdb.NewShared(t)
	base := t.TempDir()

	library := filepath.Join(base, "library")
	require.NoError(t, os.MkdirAll(filepath.Join(library, "Long Book"), 0o755))
	writeLongEPUB(t, filepath.Join(library, "Long Book", "Long Book.epub"), "Long Book")
	require.NoError(t, os.MkdirAll(filepath.Join(library, "Comic"), 0o755))
	testgen.GenerateCBZ(t, filepath.Join(library, "Comic"), "Comic.cbz", testgen.CBZOptions{Title: "Comic", PageCount: 30, HasComicInfo: true})

	cfg := config.NewForTest()
	cfg.CacheDir = filepath.Join(base, "cache")
	cfg.PluginDir = filepath.Join(base, "plugins")
	cfg.PluginDataDir = filepath.Join(base, "plugin-data")
	cfg.JWTSecret = "koreader-harness-test-secret-at-least-32-chars"

	require.NoError(t, os.MkdirAll(filepath.Join(cfg.CacheDir, "downloads"), 0o755))
	fingerprintService := fingerprints.NewService(db)
	cache := downloadcache.NewCache(filepath.Join(cfg.CacheDir, "downloads"), math.MaxInt64,
		downloadcache.WithDocumentHashRecorder(func(ctx context.Context, fileID int, deliveredType, hash string) error {
			return fingerprintService.RecordDocumentHash(ctx, fileID, deliveredType, hash)
		}))
	t.Cleanup(cache.Wait)
	pluginService := plugins.NewService(db)
	pluginManager := plugins.NewManager(pluginService, cfg.PluginDir, cfg.PluginDataDir)
	broker := events.NewBroker()
	cbz := cbzpages.NewCache(cfg.CacheDir)
	pdf := pdfpages.NewCache(cfg.CacheDir, cfg.PDFRenderDPI, cfg.PDFRenderQuality)
	w := worker.New(cfg, db, pluginService, pluginManager, broker, cache, cbz, pdf)
	srv, err := server.New(cfg, db, w, pluginService, pluginManager, broker, cache, cbz, pdf, nil)
	require.NoError(t, err)

	listener, err := net.Listen("tcp", "0.0.0.0:0")
	require.NoError(t, err)
	go func() { _ = srv.Serve(listener) }()
	t.Cleanup(func() { _ = srv.Close() })
	port := listener.Addr().(*net.TCPAddr).Port

	// The user, the library, and the Device.
	role := &models.Role{Name: "harness", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	_, err = db.NewInsert().Model(role).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(&models.Permission{RoleID: role.ID, Resource: models.ResourceBooks, Operation: models.OperationRead}).Exec(ctx)
	require.NoError(t, err)
	hash, err := auth.HashPassword("secret")
	require.NoError(t, err)
	user := &models.User{Username: "reader", PasswordHash: hash, RoleID: role.ID, IsActive: true, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	_, err = db.NewInsert().Model(user).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(&models.UserLibraryAccess{UserID: user.ID}).Exec(ctx)
	require.NoError(t, err)
	lib := &models.Library{Name: "Books", CoverAspectRatio: "book", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	_, err = db.NewInsert().Model(lib).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(&models.LibraryPath{LibraryID: lib.ID, Filepath: library, CreatedAt: time.Now(), UpdatedAt: time.Now()}).Exec(ctx)
	require.NoError(t, err)

	deviceService := devices.NewService(db)
	device, err := deviceService.Create(ctx, user.ID, "Harness KOReader", devices.DeviceTypeKOReader)
	require.NoError(t, err)

	s := &scenario{
		t: t, ctx: ctx, db: db, url: fmt.Sprintf("http://localhost:%d", port),
		user: user, device: device, devices: deviceService,
		reading: reading.NewService(db), fingerprints: fingerprintService,
		worker: w, cache: cache, library: library,
		jobLog: joblogs.NewService(db).NewJobLogger(ctx, 0, logger.FromContext(ctx)),
		h: &Harness{
			Dir:        filepath.Join(base, "koreader"),
			ReleaseDir: filepath.Join("..", "..", "tmp", "koreader", "release"),
		},
	}
	require.NoError(t, w.ProcessScanJob(ctx, nil, s.jobLog))
	job := &models.Job{Type: models.JobTypeHashGeneration, DataParsed: &models.JobHashGenerationData{LibraryID: lib.ID}}
	require.NoError(t, w.ProcessHashGenerationJob(ctx, job, s.jobLog))

	require.Equal(t, http.StatusOK, s.getStatus("/health"))
	return s
}

func (s *scenario) getStatus(path string) int {
	s.t.Helper()
	resp, err := http.Get(s.url + path)
	require.NoError(s.t, err)
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

func (s *scenario) run(a Args) *Result {
	s.t.Helper()
	result, err := s.h.Run(s.ctx, a)
	require.NoError(s.t, err)
	require.True(s.t, result.OK, "%s: %s", a.Command, result.Error)
	return result
}

func (s *scenario) login(book string) {
	s.t.Helper()
	result := s.run(Args{Command: "login", File: book, Server: s.url + "/koreader", Username: s.user.Username, Password: s.device.Credential})
	require.True(s.t, result.LoggedIn, "shown: %v", result.Shown)
}

// downloadOPDS downloads the book titled title over OPDS with the Device's
// credential, as KOReader's catalog does.
func (s *scenario) downloadOPDS(title, output string) *Result {
	s.t.Helper()
	target, err := findAcquisition(s.ctx, s.url, s.user.Username, s.device.Credential, title)
	require.NoError(s.t, err)
	return s.run(Args{Command: "download", URL: target, Output: output, Username: s.user.Username, Password: s.device.Credential})
}

func (s *scenario) file(fileType string) *models.File {
	s.t.Helper()
	file := &models.File{}
	err := s.db.NewSelect().Model(file).Where("f.file_type = ?", fileType).Limit(1).Scan(s.ctx)
	require.NoError(s.t, err)
	return file
}

func (s *scenario) outcome(file *models.File) string {
	s.t.Helper()
	var outcome string
	err := s.db.NewSelect().Model((*models.ReadThrough)(nil)).Column("rt.outcome").
		Where("rt.user_id = ? AND rt.book_id = ?", s.user.ID, file.BookID).
		OrderExpr("rt.id DESC").Limit(1).Scan(s.ctx, &outcome)
	require.NoError(s.t, err)
	return outcome
}

func (s *scenario) requireMapped(hash string, file *models.File, deliveredType string) {
	s.t.Helper()
	found, err := s.fingerprints.FindDocumentHash(s.ctx, hash)
	require.NoError(s.t, err)
	require.Len(s.t, found, 1, "KOReader's hash %s of these bytes isn't mapped", hash)
	assert.Equal(s.t, file.ID, found[0].FileID)
	assert.Equal(s.t, deliveredType, found[0].DeliveredFileType)
}

// The tests here don't run in parallel because they'd race to download and
// unpack the shared KOReader release.

// Every hash Shisho computes matches KOReader's on the same bytes, including
// files smaller than the largest sample offset.
func TestPartialMD5MatchesKOReader(t *testing.T) {
	h := &Harness{Dir: t.TempDir(), ReleaseDir: filepath.Join("..", "..", "tmp", "koreader", "release")}
	for _, size := range []int{0, 1, 500, 1024, 1025, 4096, 5000, 70000, 3145728} {
		content := make([]byte, size)
		for i := range content {
			content[i] = byte((i * 31) ^ (i >> 8) ^ (i >> 16))
		}
		path := filepath.Join(t.TempDir(), fmt.Sprintf("%d.bin", size))
		require.NoError(t, os.WriteFile(path, content, 0o644))

		want, err := h.Run(context.Background(), Args{Command: "digest", File: path})
		require.NoError(t, err)
		require.True(t, want.OK, want.Error)
		got, err := fingerprint.ComputeKOReaderPartialMD5(path)
		require.NoError(t, err)
		assert.Equal(t, want.Hash, got, "size %d", size)
	}
}

func TestRoundTrips(t *testing.T) {
	s := newScenario(t)
	epub := s.file(models.FileTypeEPUB)
	cbz := s.file(models.FileTypeCBZ)

	// Downloads over OPDS: a fresh generation and then a cache hit, each
	// hashed the same by KOReader and recorded at publish.
	fresh := s.downloadOPDS("Long Book", "fresh.epub")
	s.requireMapped(fresh.Hash, epub, models.FileTypeEPUB)
	hit := s.downloadOPDS("Long Book", "hit.epub")
	require.Equal(t, fresh.Hash, hit.Hash)

	opened := s.run(Args{Command: "open", File: "fresh.epub"})
	require.Equal(t, fresh.Hash, opened.Hash, "the sync plugin's hash and the partial MD5 agree")
	require.Greater(t, opened.PageCount, 20)

	// Sign-in.
	s.login("fresh.epub")
	wrong := s.run(Args{Command: "login", File: "fresh.epub", Server: s.url + "/koreader", Username: s.user.Username, Password: "wrong"})
	assert.False(t, wrong.LoggedIn)
	assert.Contains(t, strings.Join(wrong.Shown, " "), "Wrong username or password")
	register := s.run(Args{Command: "register", File: "fresh.epub", Server: s.url + "/koreader", Username: "someone-new", Password: "password"})
	assert.False(t, register.LoggedIn)
	assert.Contains(t, strings.Join(register.Shown, " "), "Add a KOReader Device")
	s.login("fresh.epub")

	// Open, push, and the book is Currently Reading.
	thirty := 30.0
	at30 := s.run(Args{Command: "goto", File: "fresh.epub", Percent: &thirty})
	require.True(t, strings.HasPrefix(at30.Progress, "/body/"), at30.Progress)
	pushed := s.run(Args{Command: "push", File: "fresh.epub"})
	assert.Contains(t, pushed.Shown, "Progress has been pushed.")
	assert.Equal(t, models.ReadThroughOutcomeUnderway, s.outcome(epub))
	position, err := s.reading.RetrievePosition(s.ctx, s.user.ID, epub.ID)
	require.NoError(t, err)
	require.NotNil(t, position.XPointer)
	assert.Equal(t, at30.Progress, *position.XPointer)

	// KOReader ignores its own position, so a pull never moves the device
	// that pushed it, even after it moved back.
	ten := 10.0
	at10 := s.run(Args{Command: "goto", File: "fresh.epub", Percent: &ten})
	require.NotEqual(t, at30.Progress, at10.Progress)
	for _, mode := range []string{"interactive", "silent", "prompt"} {
		own := s.run(Args{Command: "pull", File: "fresh.epub", Mode: mode})
		assert.Equal(t, at10.Progress, own.Progress, "%s pull moved the device that pushed: shown %v", mode, own.Shown)
		assert.Empty(t, own.Prompted, mode)
	}
	own := s.run(Args{Command: "pull", File: "fresh.epub", Mode: "interactive"})
	assert.Contains(t, own.Shown, "Latest progress is coming from this device.")

	// A copy downloaded before a metadata edit still syncs, beside the new one.
	_, err = s.db.NewUpdate().Model((*models.Book)(nil)).Set("title = ?", "Long Book, Revised").Where("id = ?", epub.BookID).Exec(s.ctx)
	require.NoError(t, err)
	// A Generated File takes its title from the File when it has a name.
	_, err = s.db.NewUpdate().Model((*models.File)(nil)).Set("name = ?", "Long Book, Revised").Where("id = ?", epub.ID).Exec(s.ctx)
	require.NoError(t, err)
	revised := s.downloadOPDS("Long Book, Revised", "revised.epub")
	require.NotEqual(t, fresh.Hash, revised.Hash)
	s.requireMapped(revised.Hash, epub, models.FileTypeEPUB)
	s.requireMapped(fresh.Hash, epub, models.FileTypeEPUB)
	forty := 40.0
	at40 := s.run(Args{Command: "goto", File: "fresh.epub", Percent: &forty})
	s.run(Args{Command: "push", File: "fresh.epub"})
	position, err = s.reading.RetrievePosition(s.ctx, s.user.ID, epub.ID)
	require.NoError(t, err)
	assert.Equal(t, at40.Progress, *position.XPointer)

	// A file inside a bulk download hashes to what was recorded.
	s.requireBulkDownloadMapped(epub, cbz)

	// A library file copied over by hand syncs.
	sideloaded := s.run(Args{Command: "goto", File: epub.Filepath, Percent: &ten})
	s.requireMapped(sideloaded.Hash, epub, models.FileTypeEPUB)
	require.NotEqual(t, fresh.Hash, sideloaded.Hash, "the Original File isn't the Generated File")
	s.run(Args{Command: "push", File: filepath.Base(epub.Filepath)})
	position, err = s.reading.RetrievePosition(s.ctx, s.user.ID, epub.ID)
	require.NoError(t, err)
	assert.Equal(t, sideloaded.Progress, *position.XPointer)

	// Pages carry across exactly, to and from the web reader.
	s.downloadOPDS("Comic", "comic.cbz")
	at20 := s.run(Args{Command: "goto", File: "comic.cbz", Page: 20})
	assert.Equal(t, "20", at20.Progress)
	s.run(Args{Command: "push", File: "comic.cbz"})
	comicPosition, err := s.reading.RetrievePosition(s.ctx, s.user.ID, cbz.ID)
	require.NoError(t, err)
	require.NotNil(t, comicPosition.Page)
	assert.Equal(t, 19, *comicPosition.Page, "a stored page counts from 0")
	webPage := 24
	_, err = s.reading.UpdatePosition(s.ctx, reading.PositionUpdate{
		UserID: s.user.ID, FileID: cbz.ID, Source: models.ReadingSourceWebReader,
		Locator: models.ReadingLocator{Page: &webPage}, Percentage: 25.0 / 30,
	})
	require.NoError(t, err)
	// Prompt mode, KOReader's pull on opening and waking, offers the jump
	// under the web reader's name, and waits for an answer.
	prompted := s.run(Args{Command: "pull", File: "comic.cbz", Mode: "prompt"})
	assert.Contains(t, prompted.Prompted, "Shisho web reader", "shown: %v", prompted.Shown)
	assert.Equal(t, "20", prompted.Progress, "the jump waits for an answer")
	confirmed := s.run(Args{Command: "pull", File: "comic.cbz", Mode: "prompt", Confirm: true})
	assert.Equal(t, "25", confirmed.Progress)

	// Silent mode jumps with no dialog, and the menu's pull always jumps.
	for _, mode := range []string{"silent", "interactive"} {
		s.run(Args{Command: "goto", File: "comic.cbz", Page: 20})
		fromWeb := s.run(Args{Command: "pull", File: "comic.cbz", Mode: mode})
		assert.Equal(t, "25", fromWeb.Progress, "%s: shown %v", mode, fromWeb.Shown)
		assert.Equal(t, 25, fromWeb.Page, mode)
		assert.Empty(t, fromWeb.Prompted, mode)
	}

	// An offline push queues, and arrives when drained.
	twentyEight := s.run(Args{Command: "goto", File: "comic.cbz", Page: 28})
	queued := s.run(Args{Command: "queue-push", File: "comic.cbz"})
	assert.Equal(t, 1, queued.Queued)
	comicPosition, err = s.reading.RetrievePosition(s.ctx, s.user.ID, cbz.ID)
	require.NoError(t, err)
	assert.Equal(t, 24, *comicPosition.Page, "nothing arrived while offline")
	drained := s.run(Args{Command: "drain", File: "comic.cbz"})
	assert.Equal(t, 1, drained.Drained)
	assert.Equal(t, 0, drained.Queued)
	comicPosition, err = s.reading.RetrievePosition(s.ctx, s.user.ID, cbz.ID)
	require.NoError(t, err)
	assert.Equal(t, 27, *comicPosition.Page)
	assert.Equal(t, "28", twentyEight.Progress)

	// Page view reports exactly 100% on the last page. Continuous view never
	// does: it reports the top of the last screen, one screen short of the end,
	// which is why the last 1% finishes the book. Going to its end and pushing
	// marks the book Read.
	hundred := 100.0
	pageEnd := s.run(Args{Command: "goto", File: "fresh.epub", Percent: &hundred})
	require.InDelta(t, 1, pageEnd.Percentage, 1e-9)
	scrollEnd := s.run(Args{Command: "goto", File: "fresh.epub", Percent: &hundred, View: "scroll"})
	require.Less(t, scrollEnd.Percentage, 1.0, "continuous view reached 100%%, so the 99%% rule may no longer be needed")
	require.GreaterOrEqual(t, scrollEnd.Percentage, 0.99)
	s.run(Args{Command: "push", File: "fresh.epub", View: "scroll"})
	assert.Equal(t, models.ReadThroughOutcomeFinished, s.outcome(epub))

	// Screenshots show the rendered page.
	shot := filepath.Join(t.TempDir(), "page.png")
	s.run(Args{Command: "screenshot", File: "fresh.epub", Screenshot: shot})
	info, err := os.Stat(shot)
	require.NoError(t, err)
	assert.Positive(t, info.Size())

	// Regenerating the credential stops the old one, and the failure isn't
	// queued to retry with it.
	regenerated, err := s.devices.RegenerateCredential(s.ctx, s.user.ID, s.device.ID)
	require.NoError(t, err)
	before, err := s.reading.RetrievePosition(s.ctx, s.user.ID, cbz.ID)
	require.NoError(t, err)
	s.run(Args{Command: "goto", File: "comic.cbz", Page: 5})
	failed := s.run(Args{Command: "push", File: "comic.cbz"})
	assert.NotContains(t, failed.Shown, "Progress has been pushed.")
	after, err := s.reading.RetrievePosition(s.ctx, s.user.ID, cbz.ID)
	require.NoError(t, err)
	assert.Equal(t, *before.Page, *after.Page)
	s.device = regenerated
	s.login("comic.cbz")
	s.run(Args{Command: "push", File: "comic.cbz"})
	after, err = s.reading.RetrievePosition(s.ctx, s.user.ID, cbz.ID)
	require.NoError(t, err)
	assert.Equal(t, 4, *after.Page)
}

// requireBulkDownloadMapped runs a real bulk download and checks KOReader's
// hash of each file inside it.
func (s *scenario) requireBulkDownloadMapped(files ...*models.File) {
	s.t.Helper()
	ids := make([]int, len(files))
	for i, file := range files {
		ids[i] = file.ID
	}
	data := models.JobBulkDownloadData{FileIDs: ids}
	encoded, err := json.Marshal(data)
	require.NoError(s.t, err)
	job := &models.Job{Type: models.JobTypeBulkDownload, Status: models.JobStatusInProgress, Data: string(encoded)}
	require.NoError(s.t, s.worker.ProcessBulkDownloadJob(s.ctx, job, s.jobLog))

	var result models.JobBulkDownloadData
	require.NoError(s.t, json.Unmarshal([]byte(job.Data), &result))
	archive, err := zip.OpenReader(s.cache.BulkZipPath(result.FingerprintHash))
	require.NoError(s.t, err)
	defer archive.Close()
	require.Len(s.t, archive.File, len(files))
	for _, member := range archive.File {
		path := filepath.Join(s.t.TempDir(), filepath.Base(member.Name))
		in, err := member.Open()
		require.NoError(s.t, err)
		out, err := os.Create(path)
		require.NoError(s.t, err)
		_, err = io.Copy(out, in)
		require.NoError(s.t, err)
		require.NoError(s.t, in.Close())
		require.NoError(s.t, out.Close())

		digest := s.run(Args{Command: "digest", File: path})
		deliveredType := strings.TrimPrefix(filepath.Ext(member.Name), ".")
		file := files[0]
		for _, candidate := range files {
			if candidate.FileType == deliveredType {
				file = candidate
			}
		}
		s.requireMapped(digest.Hash, file, deliveredType)
	}
}

// writeLongEPUB writes an EPUB of about 200 screens in KOReader, a short real
// book: long enough that the last screen of continuous view is past 99%.
func writeLongEPUB(t *testing.T, path, title string) {
	t.Helper()
	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()
	zw := zip.NewWriter(f)
	write := func(name, content string, method uint16) {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: method})
		require.NoError(t, err)
		_, err = io.WriteString(w, content)
		require.NoError(t, err)
	}
	write("mimetype", "application/epub+zip", zip.Store)
	write("META-INF/container.xml", `<?xml version="1.0"?><container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`, zip.Deflate)
	const chapters = 12
	// The package document comes first so a metadata edit lands in the bytes
	// KOReader samples and changes the hash, as it does in most real EPUBs.
	var manifest, spine strings.Builder
	for i := range chapters {
		fmt.Fprintf(&manifest, `<item id="c%d" href="c%d.xhtml" media-type="application/xhtml+xml"/>`, i, i)
		fmt.Fprintf(&spine, `<itemref idref="c%d"/>`, i)
	}
	write("OEBPS/content.opf", fmt.Sprintf(`<?xml version="1.0"?><package xmlns="http://www.idpf.org/2007/opf" version="2.0" unique-identifier="id"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>%s</dc:title><dc:identifier id="id">urn:harness:1</dc:identifier><dc:language>en</dc:language></metadata><manifest>%s</manifest><spine>%s</spine></package>`, title, manifest.String(), spine.String()), zip.Deflate)
	for i := range chapters {
		var body strings.Builder
		for p := range 40 {
			fmt.Fprintf(&body, "<p>Chapter %d paragraph %d. %s</p>", i, p, strings.Repeat("Lorem ipsum dolor sit amet consectetur. ", 12))
		}
		write(fmt.Sprintf("OEBPS/c%d.xhtml", i), fmt.Sprintf(`<html xmlns="http://www.w3.org/1999/xhtml"><head><title>Chapter %d</title></head><body><h1>Chapter %d</h1>%s</body></html>`, i, i, body.String()), zip.Deflate)
	}
	require.NoError(t, zw.Close())
}

// paragraph is the path of the paragraph an XPointer in Long Book points
// into, such as /body/DocFragment[4]/body/p[20].
func paragraph(t *testing.T, xp string) string {
	t.Helper()
	i := strings.Index(xp, "/p[")
	require.Positive(t, i, xp)
	end := strings.Index(xp[i+1:], "/")
	if end < 0 {
		return xp
	}
	return xp[:i+1+end]
}

// Switching between the web reader and KOReader keeps the place, whether
// KOReader holds the EPUB or its KePub. Continuous view reports the top of
// the screen, which after a jump is the line the place is on.
func TestHandoffs(t *testing.T) {
	s := newScenario(t)
	epub := s.file(models.FileTypeEPUB)
	cbz := s.file(models.FileTypeCBZ)
	book, err := reflowable.Open(s.ctx, epub.Filepath, false)
	require.NoError(t, err)
	kobo, err := reflowable.Open(s.ctx, epub.Filepath, true)
	require.NoError(t, err)

	s.downloadOPDS("Long Book", "long.epub")
	s.login("long.epub")

	// KOReader holds the KePub too, as a Device whose Delivery Rules ask for
	// one would get it.
	kepubPath := filepath.Join(t.TempDir(), "long.kepub.epub")
	require.NoError(t, kepub.NewConverter().ConvertEPUB(s.ctx, epub.Filepath, kepubPath))
	kepubCopy := s.run(Args{Command: "digest", File: kepubPath})
	require.NoError(t, s.fingerprints.RecordDocumentHash(s.ctx, epub.ID, models.FileTypeKePub, kepubCopy.Hash))
	s.run(Args{Command: "open", File: kepubPath})

	webReaderAt := func(xp string, percentage float64) {
		t.Helper()
		place, err := book.FromXPointer(xp)
		require.NoError(t, err)
		require.True(t, place.Exact, xp)
		cfi := book.CFI(place)
		_, err = s.reading.UpdatePosition(s.ctx, reading.PositionUpdate{
			UserID: s.user.ID, FileID: epub.ID, Source: models.ReadingSourceWebReader,
			Locator: models.ReadingLocator{CFI: &cfi}, Percentage: percentage,
		})
		require.NoError(t, err)
	}
	webReaderPlace := func() string {
		t.Helper()
		position, err := s.reading.RetrieveWebReaderPosition(s.ctx, s.user.ID, epub.ID)
		require.NoError(t, err)
		require.NotNil(t, position.CFI)
		place, err := book.FromCFI(*position.CFI)
		require.NoError(t, err)
		require.True(t, place.Exact, *position.CFI)
		return book.XPointer(place)
	}

	for _, copy := range []struct {
		file, wrappers string
		book           *reflowable.Book
	}{{"long.epub", "", book}, {filepath.Base(kepubPath), "/div/div", kobo}} {
		// The web reader to KOReader: mid-paragraph in chapter three.
		webReaderAt("/body/DocFragment[4]/body/p[20]/text().250", 0.3)
		pulled := s.run(Args{Command: "pull", File: copy.file, Mode: "silent", View: "scroll"})
		assert.Equal(t, "/body/DocFragment[4]/body"+copy.wrappers+"/p[20]", paragraph(t, pulled.Progress), "%s: shown %v", copy.file, pulled.Shown)

		// KOReader to the web reader: a place mid-page in chapter five comes
		// back exactly.
		midPage, err := book.FromXPointer("/body/DocFragment[6]/body/p[12]/text().300")
		require.NoError(t, err)
		moved := s.run(Args{Command: "goto", File: copy.file, XPointer: copy.book.XPointer(midPage), View: "scroll"})
		require.Equal(t, "/body/DocFragment[6]/body"+copy.wrappers+"/p[12]", paragraph(t, moved.Progress))
		s.run(Args{Command: "push", File: copy.file, View: "scroll"})
		inEPUB := webReaderPlace()
		assert.Equal(t, "/body/DocFragment[6]/body/p[12]", paragraph(t, inEPUB), copy.file)
		if copy.wrappers == "" {
			assert.Equal(t, moved.Progress, inEPUB, "the EPUB's own XPointer comes back exactly")
		}
	}

	// Pushes queued offline drain in order when KOReader reconnects, and make
	// no session, even with a pull before them.
	s.downloadOPDS("Comic", "comic.cbz")
	s.run(Args{Command: "pull", File: "long.epub", Mode: "silent"})
	before := s.count("reading_sessions")
	sixty := 60.0
	atSixty := s.run(Args{Command: "goto", File: "long.epub", Percent: &sixty})
	assert.Equal(t, 1, s.run(Args{Command: "queue-push", File: "long.epub"}).Queued)
	s.run(Args{Command: "goto", File: "comic.cbz", Page: 7})
	assert.Equal(t, 2, s.run(Args{Command: "queue-push", File: "comic.cbz"}).Queued)
	drained := s.run(Args{Command: "drain", File: "comic.cbz"})
	assert.Equal(t, 2, drained.Drained)
	position, err := s.reading.RetrievePosition(s.ctx, s.user.ID, epub.ID)
	require.NoError(t, err)
	require.NotNil(t, position.XPointer)
	assert.Equal(t, atSixty.Progress, *position.XPointer)
	comic, err := s.reading.RetrievePosition(s.ctx, s.user.ID, cbz.ID)
	require.NoError(t, err)
	assert.Equal(t, 6, *comic.Page)
	assert.Equal(t, before, s.count("reading_sessions"), "a replay is no sitting")

	// A pull when opening and a push when closing, a while later, is a sitting.
	s.run(Args{Command: "pull", File: "long.epub", Mode: "silent"})
	_, err = s.db.NewUpdate().Model((*models.KOReaderPull)(nil)).Set("pulled_at = ?", time.Now().Add(-10*time.Minute)).
		Where("device_id = ?", s.device.ID).Exec(s.ctx)
	require.NoError(t, err)
	_, err = s.db.NewUpdate().Model((*models.ReadingPositionUpdate)(nil)).Set("created_at = ?", time.Now().Add(-10*time.Minute)).
		Where("user_id = ?", s.user.ID).Exec(s.ctx)
	require.NoError(t, err)
	seventy := 70.0
	s.run(Args{Command: "goto", File: "long.epub", Percent: &seventy})
	s.run(Args{Command: "push", File: "long.epub"})
	assert.Equal(t, before+1, s.count("reading_sessions"))
}

func (s *scenario) count(table string) int {
	s.t.Helper()
	n, err := s.db.NewSelect().Table(table).Where("user_id = ?", s.user.ID).Count(s.ctx)
	require.NoError(s.t, err)
	return int(n)
}
