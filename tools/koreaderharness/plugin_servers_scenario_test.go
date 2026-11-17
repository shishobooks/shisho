//go:build koreaderharness

package main

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/koreaderplugin"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// oldCopy is a copy downloaded before Shisho recorded hashes, which no
// server recognizes.
const oldCopy = "0ddc0b7a11c0b7a11c0b7a11c0b7a11c"

// signInPluginToBoth installs the plugin in s's KOReader signed in to s's
// server and other's, each as its own Device, dated from since.
func (s *scenario) signInPluginToBoth(other *scenario, since time.Time) {
	s.t.Helper()
	bundle, err := koreaderplugin.DefaultBundle()
	require.NoError(s.t, err)
	s.installPlugin(bundle.Zip())
	state := filepath.Join(s.h.Dir, "home", ".config", "koreader", "settings", "shisho.lua")
	require.NoError(s.t, os.MkdirAll(filepath.Dir(state), 0o755))
	entry := func(sc *scenario) string {
		return fmt.Sprintf(`{ url = %q, username = %q, credential = %q, device_id = %q, device_name = %q, stats_cursor = { rowid = 0 } }`,
			sc.containerServer(), sc.user.Username, sc.device.Credential, sc.device.ID, sc.device.Name)
	}
	content := fmt.Sprintf(`return { servers = { %s, %s } }`, entry(s), entry(other))
	require.NoError(s.t, os.WriteFile(state, []byte(content), 0o644))
	for _, sc := range []*scenario{s, other} {
		_, err = sc.db.NewUpdate().Model(sc.device).Set("created_at = ?", since).WherePK().Exec(sc.ctx)
		require.NoError(s.t, err)
	}
}

// unrecognized is the documents s's server lists as unrecognized on its
// Device.
func (s *scenario) unrecognized() []string {
	s.t.Helper()
	documents := []string{}
	err := s.db.NewSelect().Model((*models.KOReaderUnrecognizedBook)(nil)).Column("kub.document").
		Where("kub.device_id = ? AND kub.linked_at IS NULL AND kub.dismissed_at IS NULL", s.device.ID).
		OrderExpr("kub.document").Scan(s.ctx, &documents)
	require.NoError(s.t, err)
	return documents
}

// held is how many page rows s's server holds for document.
func (s *scenario) held(document string) int {
	s.t.Helper()
	n, err := s.db.NewSelect().Model((*models.KOReaderPageRow)(nil)).
		Where("document = ? AND file_id IS NULL AND NOT grouped", document).Count(s.ctx)
	require.NoError(s.t, err)
	return int(n)
}

// web signs in to s's server as its user, as the browser does, and returns
// the session's cookies.
func (s *scenario) web() []*http.Cookie {
	s.t.Helper()
	hash, err := auth.HashPassword("web-password")
	require.NoError(s.t, err)
	_, err = s.db.NewUpdate().Model(s.user).Set("password_hash = ?", hash).WherePK().Exec(s.ctx)
	require.NoError(s.t, err)
	resp, err := http.Post(s.url+"/api/auth/login", "application/json", strings.NewReader(`{"username":"reader","password":"web-password"}`))
	require.NoError(s.t, err)
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	require.Equal(s.t, http.StatusOK, resp.StatusCode)
	return resp.Cookies()
}

// link links document on s's Device page to file, as its owner does.
func (s *scenario) link(document string, file *models.File) {
	s.t.Helper()
	req, err := http.NewRequestWithContext(s.ctx, http.MethodPost,
		s.url+"/api/user/devices/"+s.device.ID+"/koreader-unrecognized/"+document+"/link",
		strings.NewReader(fmt.Sprintf(`{"file_id":%d}`, file.ID)))
	require.NoError(s.t, err)
	req.Header.Set("Content-Type", "application/json")
	for _, cookie := range s.web() {
		req.AddCookie(cookie)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(s.t, err)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.Equal(s.t, http.StatusNoContent, resp.StatusCode, string(body))
}

// A server that's down doesn't make the books only it recognizes look
// unrecognized on another, and a copy no server recognizes waits for it.
// Once it's back, each server gets its own books, and both list the copy.
func TestPluginSeveralServersWithOneDown(t *testing.T) {
	a := newScenario(t)
	b := newScenario(t)
	start := time.Now().UTC().Add(-3 * time.Hour)
	a.signInPluginToBoth(b, start.Add(-24*time.Hour))
	// A copy only A produced, and one only B did. The scenarios' own files
	// are the same bytes, which both recognize.
	aBook := "a000a000a000a000a000a000a000a000"
	require.NoError(t, a.fingerprints.RecordDocumentHash(a.ctx, a.file(models.FileTypeEPUB).ID, models.FileTypeEPUB, aBook))
	bBook := "b000b000b000b000b000b000b000b000"
	require.NoError(t, b.fingerprints.RecordDocumentHash(b.ctx, b.file(models.FileTypeEPUB).ID, models.FileTypeEPUB, bBook))
	row := func(document string, page int, at time.Time) StatRow {
		return StatRow{Document: document, Page: page, TotalPages: 100, StartTime: at.Unix(), Duration: 60}
	}

	b.down()
	synced := a.run(Args{Command: "plugin-sync", Online: true, SyncNow: true, Stats: []StatRow{
		row(aBook, 2, start), row(aBook, 3, start.Add(time.Minute)),
		row(bBook, 2, start.Add(time.Hour)), row(bBook, 3, start.Add(time.Hour+time.Minute)),
		row(oldCopy, 2, start.Add(2*time.Hour)), row(oldCopy, 3, start.Add(2*time.Hour+time.Minute)),
	}})
	assert.Len(t, a.pluginSessions(), 1, "A gets its own book, shown: %v", synced.Shown)
	assert.Empty(t, a.unrecognized(), "nothing from the server that's down is listed")
	assert.Zero(t, a.held(bBook))
	assert.Zero(t, a.held(oldCopy), "the copy no one recognizes waits for B")

	b.up()
	a.run(Args{Command: "plugin-sync", Online: true, SyncNow: true})
	assert.Len(t, a.pluginSessions(), 1)
	assert.Len(t, b.pluginSessions(), 1, "B gets its own book")
	assert.Equal(t, []string{oldCopy}, a.unrecognized(), "never B's book")
	assert.Equal(t, []string{oldCopy}, b.unrecognized(), "never A's book")
	assert.Equal(t, 2, a.held(oldCopy))
	assert.Equal(t, 2, b.held(oldCopy))
}

// Linking a copy on one server applies its reading there at once, and the
// next sync has the other server drop it.
func TestPluginLinkOnOneServerIsPassedToTheOther(t *testing.T) {
	a := newScenario(t)
	b := newScenario(t)
	start := time.Now().UTC().Add(-3 * time.Hour)
	a.signInPluginToBoth(b, start.Add(-24*time.Hour))
	a.run(Args{Command: "plugin-sync", Online: true, SyncNow: true, Stats: []StatRow{
		{Document: oldCopy, Page: 2, TotalPages: 100, StartTime: start.Unix(), Duration: 60},
		{Document: oldCopy, Page: 3, TotalPages: 100, StartTime: start.Add(time.Minute).Unix(), Duration: 60},
	}})
	require.Equal(t, []string{oldCopy}, a.unrecognized())
	require.Equal(t, []string{oldCopy}, b.unrecognized())
	// Each lists it with what KOReader's statistics know of it.
	listed := &models.KOReaderUnrecognizedBook{}
	require.NoError(t, b.db.NewSelect().Model(listed).Where("kub.document = ?", oldCopy).Scan(b.ctx))
	require.NotNil(t, listed.Title)
	assert.Equal(t, "Book "+oldCopy, *listed.Title)
	assert.Equal(t, []string{"Harness"}, listed.Authors)
	assert.Equal(t, 120, listed.ReadingSeconds)

	file := a.file(models.FileTypeEPUB)
	a.link(oldCopy, file)
	sessions := a.pluginSessions()
	require.Len(t, sessions, 1, "its reading appears without a sync")
	require.NotNil(t, sessions[0].FileID)
	assert.Equal(t, file.ID, *sessions[0].FileID)

	a.run(Args{Command: "plugin-sync", Online: true, SyncNow: true})
	assert.Empty(t, b.unrecognized(), "B drops the copy A linked")
	assert.Zero(t, b.held(oldCopy))
	assert.Empty(t, b.pluginSessions())
}

// A copy the server holds with nothing KOReader knows of it, as a plugin too
// old to send that leaves one, is described at the next sync, though nothing
// more was read in it.
func TestPluginDescribesACopyHeldWithoutDetails(t *testing.T) {
	s := newScenario(t)
	s.signInPlugin(time.Now().Add(-time.Hour))
	require.NoError(t, os.MkdirAll(s.h.booksDir(), 0o755))
	writeLongEPUB(t, filepath.Join(s.h.booksDir(), "odd.epub"), "Odd Copy")
	s.run(Args{Command: "plugin-read", File: "odd.epub", Turns: 2, Close: true})
	s.run(Args{Command: "plugin-sync", Online: true, SyncNow: true})
	document := s.run(Args{Command: "digest", File: "odd.epub"}).Hash
	// The copy is held, but with none of what KOReader knows of it.
	res, err := s.db.NewUpdate().Model((*models.KOReaderUnrecognizedBook)(nil)).
		Set("title = NULL").Set("file_type = NULL").Set("authors = '[]'").
		Where("document = ?", document).Exec(s.ctx)
	require.NoError(t, err)
	n, err := res.RowsAffected()
	require.NoError(t, err)
	require.EqualValues(t, 1, n, "the copy is held")

	s.run(Args{Command: "plugin-sync", Online: true, SyncNow: true})

	listed := &models.KOReaderUnrecognizedBook{}
	require.NoError(t, s.db.NewSelect().Model(listed).Where("kub.document = ?", document).Scan(s.ctx))
	require.NotNil(t, listed.Title)
	assert.Equal(t, "Odd Copy", *listed.Title)
	require.NotNil(t, listed.FileType)
	assert.Equal(t, "epub", *listed.FileType)
}

// serveBuild puts a proxy in front of s's server that serves the build it
// holds in place of the server's own plugin, as a server running another
// build of the same version does: every plugin answer names that build's
// hash, and the plugin's download is its zip. Holding nil, the server's own
// plugin is served. s's URL becomes the proxy's.
func (s *scenario) serveBuild(build *koreaderplugin.Bundle) *atomic.Pointer[koreaderplugin.Bundle] {
	s.t.Helper()
	own, err := koreaderplugin.DefaultBundle()
	require.NoError(s.t, err)
	served := &atomic.Pointer[koreaderplugin.Bundle]{}
	served.Store(build)
	s.proxy(func(resp *http.Response) error {
		build := served.Load()
		if build == nil || !strings.HasPrefix(resp.Request.URL.Path, "/koreader/plugin/") {
			return nil
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return err
		}
		if resp.Request.URL.Path == "/koreader/plugin/download" && resp.StatusCode == http.StatusOK {
			body = build.Zip()
		} else {
			body = bytes.ReplaceAll(body, []byte(own.Hash), []byte(build.Hash))
		}
		resp.Body = io.NopCloser(bytes.NewReader(body))
		resp.ContentLength = int64(len(body))
		resp.Header.Set("Content-Length", strconv.Itoa(len(body)))
		return nil
	})
	return served
}

// proxy puts a reverse proxy in front of s's server that passes each
// response through modify, and makes s's URL the proxy's.
func (s *scenario) proxy(modify func(*http.Response) error) {
	s.t.Helper()
	target, err := url.Parse(s.url)
	require.NoError(s.t, err)
	proxy := httputil.NewSingleHostReverseProxy(target)
	direct := proxy.Director
	proxy.Director = func(r *http.Request) {
		direct(r)
		// Left to the transport, which then hands back the body unzipped.
		r.Header.Del("Accept-Encoding")
	}
	proxy.ModifyResponse = modify
	listener, err := (&net.ListenConfig{}).Listen(s.ctx, "tcp", "0.0.0.0:0")
	require.NoError(s.t, err)
	srv := &http.Server{Handler: proxy, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(listener) }()
	s.t.Cleanup(func() { _ = srv.Close() })
	s.url = fmt.Sprintf("http://localhost:%d", listener.Addr().(*net.TCPAddr).Port)
}

// setInstalledFrom records in s's KOReader that the plugin was last
// installed from serverURL.
func (s *scenario) setInstalledFrom(serverURL string) {
	s.t.Helper()
	path := filepath.Join(s.h.Dir, "home", ".config", "koreader", "settings", "shisho.lua")
	data, err := os.ReadFile(path)
	require.NoError(s.t, err)
	require.True(s.t, bytes.HasPrefix(data, []byte("return { ")), "%s", data)
	data = append([]byte(fmt.Sprintf("return { installed_from = %q, ", serverURL)), data[len("return { "):]...)
	require.NoError(s.t, os.WriteFile(path, data, 0o644))
}

// Two dev servers serving their own builds of one version: only the server
// the plugin was last installed from offers its build, so the plugin never
// swaps between them each time Wi-Fi comes on.
func TestPluginOffersASameVersionBuildOnlyFromItsServer(t *testing.T) {
	a := newScenario(t)
	b := newScenario(t)
	own, err := koreaderplugin.DefaultBundle()
	require.NoError(t, err)
	bBuild := anotherBuild(t)
	served := b.serveBuild(bBuild)
	a.signInPluginToBoth(b, time.Now().Add(-time.Hour))
	a.setInstalledFrom(a.containerServer())

	// Installed from A, which serves the build installed: B's build isn't
	// offered.
	launched := a.run(Args{Command: "plugin-launch", Online: true})
	require.Len(t, launched.Plugin.Servers, 2, "shown: %v", launched.Shown)
	require.NotNil(t, launched.Plugin.Servers[1].Info, "B answered")
	require.Equal(t, bBuild.Hash, launched.Plugin.Servers[1].Info.PluginHash, "B serves its own build")
	assert.Equal(t, own.Hash, launched.Plugin.Hash)
	assert.Nil(t, launched.Plugin.Offer, "shown: %v", launched.Shown)

	// Reinstalled from B, A's build isn't offered either.
	reinstalled := a.run(Args{Command: "plugin-update", Online: true, From: b.url})
	require.True(t, reinstalled.Plugin.Installed, reinstalled.Plugin.Message)
	require.Equal(t, bBuild.Hash, reinstalled.Plugin.Hash)
	require.Equal(t, b.containerServer(), reinstalled.Plugin.InstalledFrom)
	launched = a.run(Args{Command: "plugin-launch", Online: true})
	require.NotNil(t, launched.Plugin.Servers[0].Info, "A answered")
	require.Equal(t, own.Hash, launched.Plugin.Servers[0].Info.PluginHash, "A serves its own build")
	assert.Nil(t, launched.Plugin.Offer, "shown: %v", launched.Shown)

	// B rebuilt: B offers its new build, though A serves the same one.
	served.Store(nil)
	launched = a.run(Args{Command: "plugin-launch", Online: true})
	require.NotNil(t, launched.Plugin.Offer, "shown: %v", launched.Shown)
	assert.Equal(t, b.containerServer(), launched.Plugin.Offer.Server)
	assert.Equal(t, own.Hash, launched.Plugin.Offer.Hash)
}

// onCopy posts action for document on s's Device page, as its owner does:
// dismiss, restore, or delete-reading.
func (s *scenario) onCopy(document, action string) {
	s.t.Helper()
	req, err := http.NewRequestWithContext(s.ctx, http.MethodPost,
		s.url+"/api/user/devices/"+s.device.ID+"/koreader-unrecognized/"+document+"/"+action, nil)
	require.NoError(s.t, err)
	for _, cookie := range s.web() {
		req.AddCookie(cookie)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(s.t, err)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.Equal(s.t, http.StatusNoContent, resp.StatusCode, string(body))
}

// Sync now says how many books wait to be linked. A copy whose reading was
// deleted on the Device's page, then restored, gets it back when the plugin
// sends its history again, and sending it again once linked adds nothing.
func TestPluginSendsHistoryAgainForARestoredCopy(t *testing.T) {
	s := newScenario(t)
	start := time.Now().UTC().Add(-3 * time.Hour)
	s.signInPluginWith(start.Add(-24*time.Hour), "history = { through = 0 }")
	synced := s.run(Args{Command: "plugin-sync", Online: true, SyncNow: true, Stats: []StatRow{
		{Document: oldCopy, Page: 2, TotalPages: 100, StartTime: start.Unix(), Duration: 60},
		{Document: oldCopy, Page: 3, TotalPages: 100, StartTime: start.Add(time.Minute).Unix(), Duration: 60},
	}})
	require.Equal(t, []string{oldCopy}, s.unrecognized())
	require.Equal(t, 2, s.held(oldCopy))
	assert.Contains(t, strings.Join(synced.Shown, "\n"),
		"1 book on "+s.containerLabel()+" is waiting to be linked on the Device's page there.")

	s.onCopy(oldCopy, "dismiss")
	s.onCopy(oldCopy, "delete-reading")
	require.Zero(t, s.held(oldCopy))
	quiet := s.run(Args{Command: "plugin-sync", Online: true, SyncNow: true})
	assert.NotContains(t, strings.Join(quiet.Shown, "\n"), "waiting to be linked")
	require.Zero(t, s.held(oldCopy), "its reading stays dropped")

	s.onCopy(oldCopy, "restore")
	menu := s.run(Args{Command: "plugin-menu"})
	require.Contains(t, menu.Plugin.Menu, s.containerLabel()+" > Send reading history again")
	sent := s.run(Args{Command: "plugin-menu", Online: true, SendHistoryAgain: true})
	assert.Equal(t, 2, s.held(oldCopy), "its reading is back: %v", sent.Shown)

	file := s.file(models.FileTypeEPUB)
	s.link(oldCopy, file)
	require.Len(t, s.pluginSessions(), 1)
	s.run(Args{Command: "plugin-menu", Online: true, SendHistoryAgain: true})
	assert.Len(t, s.pluginSessions(), 1, "nothing is counted twice")
	assert.Zero(t, s.held(oldCopy))
}

// asOfV080 puts a proxy in front of s's server that, while it holds true,
// answers the plugin as Shisho v0.8.0 did, from before capabilities: no
// response lists any. s's URL becomes the proxy's.
func (s *scenario) asOfV080() *atomic.Bool {
	s.t.Helper()
	old := &atomic.Bool{}
	s.proxy(func(resp *http.Response) error {
		if old.Load() {
			resp.Header.Del(koreaderplugin.HeaderCapabilities)
		}
		return nil
	})
	return old
}

// Send reading history again needs the server to keep a held copy's own
// status, which v0.8.0 doesn't, so the plugin offers it only where the server
// lists the capability, and notices an upgrade or a rollback at the next sync.
func TestPluginOffersSendHistoryAgainOnlyWhereTheServerTakesIt(t *testing.T) {
	s := newScenario(t)
	old := s.asOfV080()
	old.Store(true)
	s.signInPluginWith(time.Now().UTC().Add(-24*time.Hour), "history = { through = 0 }")
	again := s.containerLabel() + " > Send reading history again"
	menuAfterSync := func() []string {
		s.run(Args{Command: "plugin-sync", Online: true, SyncNow: true})
		return s.run(Args{Command: "plugin-menu"}).Plugin.Menu
	}

	assert.NotContains(t, menuAfterSync(), again, "v0.8.0 would start a held copy's Read-through on the wrong day")

	old.Store(false)
	assert.Contains(t, menuAfterSync(), again, "the upgraded server lists it")

	old.Store(true)
	assert.NotContains(t, menuAfterSync(), again, "a rollback takes it away at the next sync")
}
