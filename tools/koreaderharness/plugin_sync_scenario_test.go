//go:build koreaderharness

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/koreaderplugin"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/reading"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// signInPlugin installs the plugin signed in to the scenario's server before
// any reading was recorded, and dates the Device from since: the server
// refuses reading from before its Device existed.
func (s *scenario) signInPlugin(since time.Time) {
	s.t.Helper()
	s.signInPluginWith(since, "")
}

// signInPluginWith is signInPlugin with extra fields, as Lua, on the server's
// entry in the plugin's state.
func (s *scenario) signInPluginWith(since time.Time, extra string) {
	s.t.Helper()
	bundle, err := koreaderplugin.DefaultBundle()
	require.NoError(s.t, err)
	s.installPlugin(bundle.Zip())
	state := filepath.Join(s.h.Dir, "home", ".config", "koreader", "settings", "shisho.lua")
	require.NoError(s.t, os.MkdirAll(filepath.Dir(state), 0o755))
	content := fmt.Sprintf(`return { servers = { { url = %q, username = %q, credential = %q, device_id = %q, device_name = %q,
		stats_cursor = { rowid = 0 }, %s } } }`,
		s.containerServer(), s.user.Username, s.device.Credential, s.device.ID, s.device.Name, extra)
	require.NoError(s.t, os.WriteFile(state, []byte(content), 0o644))
	_, err = s.db.NewUpdate().Model((*devices.Device)(nil)).Set("created_at = ?", since).Where("id = ?", s.device.ID).Exec(s.ctx)
	require.NoError(s.t, err)
}

// documentHash is a hash the server maps to file.
func (s *scenario) documentHash(file *models.File) string {
	s.t.Helper()
	var hash string
	err := s.db.NewSelect().Model((*models.DocumentHash)(nil)).Column("kdh.hash").
		Where("kdh.file_id = ?", file.ID).Limit(1).Scan(s.ctx, &hash)
	require.NoError(s.t, err)
	return hash
}

func (s *scenario) pluginSessions() []*models.ReadingSession {
	s.t.Helper()
	var sessions []*models.ReadingSession
	err := s.db.NewSelect().Model(&sessions).
		Where("rs.user_id = ? AND rs.source = ?", s.user.ID, models.ReadingSourceKOReader).
		OrderExpr("rs.started_at").Scan(s.ctx)
	require.NoError(s.t, err)
	return sessions
}

// Reading offline across two days reaches Shisho in one sync, as measured
// sessions on the days they happened, timed by actual reading.
func TestPluginSyncsReadingTimeAfterOfflineDays(t *testing.T) {
	s := newScenario(t)
	epub := s.file(models.FileTypeEPUB)
	document := s.documentHash(epub)
	dayOne := time.Now().UTC().Truncate(24 * time.Hour).Add(-48*time.Hour + 10*time.Hour)
	dayTwo := dayOne.Add(24 * time.Hour)
	s.signInPlugin(dayOne.Add(-24 * time.Hour))

	row := func(page int, at time.Time, seconds int) StatRow {
		return StatRow{Document: document, Page: page, TotalPages: 100, StartTime: at.Unix(), Duration: seconds}
	}
	stats := []StatRow{
		// Day one: a sitting, a break of half an hour, and another.
		row(1, dayOne, 60), row(2, dayOne.Add(time.Minute), 60),
		row(3, dayOne.Add(32*time.Minute), 60), row(4, dayOne.Add(33*time.Minute), 120),
		// Day two.
		row(5, dayTwo, 90), row(6, dayTwo.Add(90*time.Second), 90),
	}

	offline := s.run(Args{Command: "plugin-sync", Stats: stats})
	assert.Empty(t, s.pluginSessions(), "nothing goes without Wi-Fi")
	require.Len(t, offline.Plugin.Servers, 1)

	// Wi-Fi coming on syncs, with nothing tapped.
	synced := s.run(Args{Command: "plugin-sync", Online: true})
	sessions := s.pluginSessions()
	require.Len(t, sessions, 3, "shown: %v", synced.Shown)
	for i, want := range []struct {
		start    time.Time
		duration int
	}{{dayOne, 120}, {dayOne.Add(32 * time.Minute), 180}, {dayTwo, 180}} {
		assert.True(t, sessions[i].Measured, "session %d is measured", i)
		assert.Equal(t, want.start.Unix(), sessions[i].StartedAt.Unix(), "session %d starts when its first page did", i)
		assert.Equal(t, want.duration, sessions[i].Duration, "session %d is timed by its pages", i)
		require.NotNil(t, sessions[i].SourceDeviceID)
		assert.Equal(t, s.device.ID, *sessions[i].SourceDeviceID)
	}

	readThrough := &models.ReadThrough{}
	err := s.db.NewSelect().Model(readThrough).Where("rt.user_id = ? AND rt.book_id = ?", s.user.ID, epub.BookID).Scan(s.ctx)
	require.NoError(t, err)
	require.NotNil(t, readThrough.BeginDay)
	assert.Equal(t, dayOne.Format(time.DateOnly), *readThrough.BeginDay, "the Read-through began on the first day")

	server := synced.Plugin.Servers[0]
	require.NotNil(t, server.StatsCursor)
	assert.Equal(t, int64(len(stats)), server.StatsCursor.RowID, "the cursor is past every row")
	assert.NotZero(t, server.LastSynced)

	// Syncing again sends nothing new and changes nothing.
	s.run(Args{Command: "plugin-sync", Online: true, SyncNow: true})
	assert.Len(t, s.pluginSessions(), 3)
}

// Sync on sleep turns Wi-Fi on to sync and off again, since a Kobo can't
// sleep with Wi-Fi on, and waits an hour after a sync before doing it again.
func TestPluginSyncsOnSleepAtMostHourly(t *testing.T) {
	s := newScenario(t)
	epub := s.file(models.FileTypeEPUB)
	document := s.documentHash(epub)
	start := time.Now().UTC().Add(-3 * time.Hour)
	s.signInPlugin(start.Add(-24 * time.Hour))
	row := func(page int, at time.Time) StatRow {
		return StatRow{Document: document, Page: page, TotalPages: 100, StartTime: at.Unix(), Duration: 60}
	}

	slept := s.run(Args{Command: "plugin-sleep", Stats: []StatRow{row(2, start), row(3, start.Add(time.Minute))}})
	assert.True(t, slept.Plugin.WifiTurnedOn)
	assert.False(t, slept.Plugin.WifiLeftOn, "Wi-Fi is off again before the device sleeps")
	require.Len(t, s.pluginSessions(), 1)

	// Sleeping again within the hour leaves Wi-Fi off and sends nothing.
	again := s.run(Args{Command: "plugin-sleep", Stats: []StatRow{row(4, start.Add(2*time.Hour))}})
	assert.False(t, again.Plugin.WifiTurnedOn)
	assert.False(t, again.Plugin.WifiLeftOn)
	assert.Len(t, s.pluginSessions(), 1)
}

// Daily sync wakes a sleeping device at its time, 4:00 by default, syncs
// with Wi-Fi it turns off again, and sets the next day's alarm.
func TestPluginDailySyncWakesTheDevice(t *testing.T) {
	s := newScenario(t)
	epub := s.file(models.FileTypeEPUB)
	document := s.documentHash(epub)
	start := time.Now().UTC().Add(-3 * time.Hour)
	s.signInPlugin(start.Add(-24 * time.Hour))
	stats := []StatRow{
		{Document: document, Page: 2, TotalPages: 100, StartTime: start.Unix(), Duration: 60},
		{Document: document, Page: 3, TotalPages: 100, StartTime: start.Add(time.Minute).Unix(), Duration: 60},
	}

	asleep := s.run(Args{Command: "plugin-daily", Stats: stats})
	require.Len(t, asleep.Plugin.AlarmsArmed, 1, "going to sleep sets the alarm")
	alarm := time.Unix(asleep.Plugin.AlarmsArmed[0], 0).UTC()
	// The container's KOReader runs on UTC.
	assert.Equal(t, 4, alarm.Hour())
	assert.Equal(t, 0, alarm.Minute())
	assert.True(t, alarm.After(time.Now()) && alarm.Before(time.Now().Add(24*time.Hour)), "the next 4:00: %s", alarm)
	assert.Empty(t, s.pluginSessions(), "nothing until the alarm")

	woken := s.run(Args{Command: "plugin-daily", Fire: true})
	assert.True(t, woken.Plugin.WifiTurnedOn)
	assert.False(t, woken.Plugin.WifiLeftOn, "Wi-Fi is off again before the device goes back to sleep")
	require.Len(t, s.pluginSessions(), 1)
	require.Len(t, woken.Plugin.AlarmsAfter, 1, "the next day's alarm is set")
	assert.Equal(t, int64(24*60*60), woken.Plugin.AlarmsAfter[0]-woken.Plugin.AlarmsArmed[0])
}

// KOReader asks before turning Wi-Fi on unless told to turn it on by itself,
// and a question on a sleeping screen helps no one, so syncing on sleep and
// daily sync need that setting and otherwise leave Wi-Fi alone.
func TestPluginSyncsOnSleepOnlyWhenKOReaderTurnsWifiOnByItself(t *testing.T) {
	s := newScenario(t)
	epub := s.file(models.FileTypeEPUB)
	document := s.documentHash(epub)
	start := time.Now().UTC().Add(-3 * time.Hour)
	s.signInPlugin(start.Add(-24 * time.Hour))
	stats := []StatRow{{Document: document, Page: 2, TotalPages: 100, StartTime: start.Unix(), Duration: 60}}

	slept := s.run(Args{Command: "plugin-sleep", WifiPrompt: true, Stats: stats})
	assert.False(t, slept.Plugin.WifiTurnedOn)
	assert.Empty(t, slept.Prompted, "nothing asks on the sleep screen")
	assert.Empty(t, s.pluginSessions())

	woken := s.run(Args{Command: "plugin-daily", WifiPrompt: true, Fire: true})
	assert.False(t, woken.Plugin.WifiTurnedOn)
	assert.Empty(t, s.pluginSessions())
}

// A server turned off in the plugin gets nothing and is never contacted, so
// one that's out of reach costs nothing; turned on again, it gets the
// reading from where it left off.
func TestPluginLeavesADisabledServerAlone(t *testing.T) {
	s := newScenario(t)
	epub := s.file(models.FileTypeEPUB)
	document := s.documentHash(epub)
	start := time.Now().UTC().Add(-3 * time.Hour)
	s.signInPluginWith(start.Add(-24*time.Hour), "disabled = true")
	stats := []StatRow{
		{Document: document, Page: 2, TotalPages: 100, StartTime: start.Unix(), Duration: 60},
		{Document: document, Page: 3, TotalPages: 100, StartTime: start.Add(time.Minute).Unix(), Duration: 60},
	}

	off := s.run(Args{Command: "plugin-sync", Online: true, SyncNow: true, Stats: stats})
	assert.Empty(t, s.pluginSessions())
	require.Len(t, off.Plugin.Servers, 1)
	assert.Equal(t, int64(0), off.Plugin.Servers[0].StatsCursor.RowID)
	assert.Zero(t, off.Plugin.Journal, "nothing waits for it")
	device, err := s.devices.Retrieve(s.ctx, s.user.ID, s.device.ID)
	require.NoError(t, err)
	assert.Nil(t, device.PluginLastConnectedAt, "not even checked in with")
	assert.Contains(t, strings.Join(off.Shown, " "), "No Shisho server is enabled")

	menu := s.run(Args{Command: "plugin-menu"})
	assert.Contains(t, menu.Plugin.Menu, s.containerLabel()+" (Disabled)")
	assert.Contains(t, menu.Plugin.Menu, s.containerLabel()+" (Disabled) > Enabled")

	// Turned on again, it gets what it missed from statistics.
	state := filepath.Join(s.h.Dir, "home", ".config", "koreader", "settings", "shisho.lua")
	data, err := os.ReadFile(state)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(state, []byte(strings.Replace(string(data), "disabled = true", "disabled = nil", 1)), 0o644))
	s.run(Args{Command: "plugin-sync", Online: true, SyncNow: true})
	require.Len(t, s.pluginSessions(), 1)
	assert.Equal(t, 120, s.pluginSessions()[0].Duration)
}

// A sync refused for a regenerated credential keeps everything queued, and
// signing in again delivers it with nothing lost.
func TestPluginKeepsReadingQueuedUntilSignedInAgain(t *testing.T) {
	s := newScenario(t)
	epub := s.file(models.FileTypeEPUB)
	document := s.documentHash(epub)
	since := time.Now().Add(-3 * time.Hour).Truncate(time.Second)
	s.signInPlugin(since)
	read := since.Add(time.Hour)
	stats := []StatRow{
		{Document: document, Page: 1, TotalPages: 100, StartTime: read.Unix(), Duration: 60},
		{Document: document, Page: 2, TotalPages: 100, StartTime: read.Add(time.Minute).Unix(), Duration: 60},
	}
	regenerated, err := s.devices.RegenerateCredential(s.ctx, s.user.ID, s.device.ID)
	require.NoError(t, err)

	refused := s.run(Args{Command: "plugin-sync", Stats: stats, Online: true})
	require.Len(t, refused.Plugin.Servers, 1)
	assert.True(t, refused.Plugin.Servers[0].NeedsLogin)
	require.NotNil(t, refused.Plugin.Servers[0].StatsCursor)
	assert.Zero(t, refused.Plugin.Servers[0].StatsCursor.RowID, "nothing is counted as sent")
	assert.Empty(t, s.pluginSessions())

	nowShown := s.run(Args{Command: "plugin-sync", Online: true, SyncNow: true})
	assert.Contains(t, strings.Join(nowShown.Plugin.Problems, " "), "needs a new sign-in")

	signedIn := s.run(Args{Command: "plugin-signin", Server: s.url, Username: "reader", Password: regenerated.Credential, Online: true})
	require.True(t, signedIn.Plugin.SignedIn, signedIn.Plugin.Message)

	delivered := s.run(Args{Command: "plugin-sync", Online: true, SyncNow: true})
	assert.Empty(t, delivered.Plugin.Problems)
	sessions := s.pluginSessions()
	require.Len(t, sessions, 1, "shown: %v", delivered.Shown)
	assert.Equal(t, 120, sessions[0].Duration)
	assert.False(t, delivered.Plugin.Servers[0].NeedsLogin)
	assert.Equal(t, int64(2), delivered.Plugin.Servers[0].StatsCursor.RowID)

	// With statistics off, Sync now says reading time won't sync.
	off := s.run(Args{Command: "plugin-sync", Online: true, SyncNow: true, StatisticsOff: true})
	assert.Contains(t, strings.Join(off.Shown, " "), "reading time won't sync")
	menu := s.run(Args{Command: "plugin-menu"})
	require.NotEmpty(t, menu.Plugin.Menu)
	// Short enough for one line of the menu; tapping it says the rest.
	assert.Equal(t, "Reading time won't sync: statistics off", menu.Plugin.Menu[0])
}

// The plugin sends this Device's place, and opening a book online offers a
// newer place from the web reader, naming it.
func TestPluginPushesAndPullsPositions(t *testing.T) {
	s := newScenario(t)
	s.downloadOPDS("Comic", "comic.cbz")
	cbz := s.file(models.FileTypeCBZ)
	s.signInPlugin(time.Now().Add(-time.Hour))

	pushed := s.run(Args{Command: "plugin-open", File: "comic.cbz", Page: 20, Online: true, SyncNow: true})
	assert.Equal(t, 0, pushed.Plugin.Journal, "the server took the place: %v", pushed.Shown)
	position, err := s.reading.RetrievePosition(s.ctx, s.user.ID, cbz.ID)
	require.NoError(t, err)
	require.NotNil(t, position)
	require.NotNil(t, position.Page)
	assert.Equal(t, 19, *position.Page, "a stored page counts from 0")
	require.NotNil(t, position.SourceDeviceID)
	assert.Equal(t, s.device.ID, *position.SourceDeviceID)

	// Its own place isn't offered back.
	reopened := s.run(Args{Command: "plugin-open", File: "comic.cbz", Online: true})
	assert.Empty(t, reopened.Prompted)

	time.Sleep(1100 * time.Millisecond)
	webPage := 24
	_, err = s.reading.UpdatePosition(s.ctx, reading.PositionUpdate{
		UserID: s.user.ID, FileID: cbz.ID, Source: models.ReadingSourceWebReader,
		Locator: models.ReadingLocator{Page: &webPage}, Percentage: 25.0 / 30,
	})
	require.NoError(t, err)

	offline := s.run(Args{Command: "plugin-open", File: "comic.cbz"})
	assert.Empty(t, offline.Prompted, "opening offline doesn't wait for Wi-Fi")

	prompted := s.run(Args{Command: "plugin-open", File: "comic.cbz", Online: true})
	assert.Contains(t, prompted.Prompted, "Shisho web reader", "shown: %v", prompted.Shown)
	assert.Equal(t, "20", prompted.Progress, "the jump waits for an answer")
	confirmed := s.run(Args{Command: "plugin-open", File: "comic.cbz", Online: true, Confirm: true})
	assert.Equal(t, "25", confirmed.Progress)
}

// containerLabel is how the plugin's menu names the scenario's server: its
// URL without the scheme.
func (s *scenario) containerLabel() string {
	label := s.containerServer()
	if i := strings.Index(label, "://"); i >= 0 {
		label = label[i+3:]
	}
	return label
}
