//go:build koreaderharness

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/koreaderplugin"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pastYear is a year of KOReader reading from before the plugin: a sitting
// of twelve pages each week, the first half-year in the comic and the rest
// in the long book, with the comic finished and rated and the long book put
// On hold. It's recorded on the device with the plugin installed and no
// server signed in.
type pastYear struct {
	cbz, epub *models.File
	// sittings is when each sitting began, in order.
	sittings []time.Time
	// finished and onHold are the days KOReader recorded the statuses.
	finished, onHold string
}

func (s *scenario) recordPastYear() pastYear {
	s.t.Helper()
	s.downloadOPDS("Comic", "comic.cbz")
	s.downloadOPDS("Long Book", "long.epub")
	y := pastYear{cbz: s.file(models.FileTypeCBZ), epub: s.file(models.FileTypeEPUB)}
	cbzHash, epubHash := s.documentHash(y.cbz), s.documentHash(y.epub)
	start := time.Now().UTC().Truncate(24*time.Hour).AddDate(-1, 0, 0).Add(20 * time.Hour)

	var stats []StatRow
	for week := 0; week < 52; week++ {
		at := start.AddDate(0, 0, 7*week)
		y.sittings = append(y.sittings, at)
		document := cbzHash
		if week >= 26 {
			document = epubHash
		}
		for page := 1; page <= 12; page++ {
			stats = append(stats, StatRow{
				Document: document, Page: page + 12*(week%26), TotalPages: 400,
				StartTime: at.Add(time.Duration(page-1) * time.Minute).Unix(), Duration: 60,
			})
		}
	}
	y.finished = start.AddDate(0, 0, 7*25+1).Format(time.DateOnly)
	y.onHold = start.AddDate(0, 0, 7*51+1).Format(time.DateOnly)

	bundle, err := koreaderplugin.DefaultBundle()
	require.NoError(s.t, err)
	s.installPlugin(bundle.Zip())
	s.run(Args{Command: "plugin-sync", Stats: stats, Summaries: map[string]SidecarSummary{
		"comic.cbz": {Status: "complete", Rating: 4, Modified: y.finished},
		"long.epub": {Status: "abandoned", Modified: y.onHold},
	}})
	return y
}

// signInFresh signs the plugin in with a pairing code, as a person does on
// first sign-in, answering whether to send history.
func (s *scenario) signInFresh(history string) *Result {
	s.t.Helper()
	short, err := s.devices.GenerateShortURL(s.ctx, s.user.ID, s.device.ID)
	require.NoError(s.t, err)
	result := s.run(Args{Command: "plugin-signin", Server: s.url, Code: strings.ToUpper(short.ShortCode), Online: true, History: history})
	require.True(s.t, result.Plugin.SignedIn, result.Plugin.Message)
	require.Contains(s.t, result.Plugin.Asked, "reading history", "first sign-in asks")
	return result
}

// Saying yes on first sign-in sends a year of history: every sitting on the
// day it happened, and a Read-through for each book KOReader shows finished
// or On hold, dated by its sessions and its status.
func TestPluginSendsHistoryOnFirstSignIn(t *testing.T) {
	s := newScenario(t)
	y := s.recordPastYear()

	signedIn := s.signInFresh("send")
	shownProgress := false
	for _, text := range signedIn.Shown {
		if strings.Contains(text, "Sending your reading history") {
			shownProgress = true
		}
	}
	assert.True(t, shownProgress, "a long history shows how far it got: %v", signedIn.Shown)

	sessions := s.pluginSessions()
	require.Len(t, sessions, len(y.sittings), "shown: %v", signedIn.Shown)
	for i, session := range sessions {
		assert.True(t, session.Measured)
		assert.Equal(t, y.sittings[i].Unix(), session.StartedAt.Unix(), "sitting %d is on the day it happened", i)
		assert.Equal(t, 12*60, session.Duration)
	}

	comic := s.readThroughs(y.cbz)
	require.Len(t, comic, 1)
	assert.Equal(t, models.ReadThroughOutcomeFinished, comic[0].Outcome)
	require.NotNil(t, comic[0].BeginDay)
	assert.Equal(t, y.sittings[0].Format(time.DateOnly), *comic[0].BeginDay)
	require.NotNil(t, comic[0].EndDay)
	assert.Equal(t, y.finished, *comic[0].EndDay)
	require.NotNil(t, s.ratingUnits(y.cbz))
	assert.Equal(t, 16, *s.ratingUnits(y.cbz), "its stars come along")

	long := s.readThroughs(y.epub)
	require.Len(t, long, 1)
	assert.Equal(t, models.ReadThroughOutcomeUnderway, long[0].Outcome)
	assert.True(t, long[0].Paused)
	require.NotNil(t, long[0].BeginDay)
	assert.Equal(t, y.sittings[26].Format(time.DateOnly), *long[0].BeginDay)

	// Syncing again duplicates nothing, and KOReader keeps its statuses.
	again := s.run(Args{Command: "plugin-sync", Online: true, SyncNow: true})
	assert.Empty(t, again.Plugin.Problems)
	assert.Len(t, s.pluginSessions(), len(y.sittings))
	assert.Len(t, s.readThroughs(y.cbz), 1)
	assert.Len(t, s.readThroughs(y.epub), 1)
	assert.Equal(t, "complete", again.Plugin.Sidecars["comic.cbz"].Status)
	assert.Equal(t, "abandoned", again.Plugin.Sidecars["long.epub"].Status)
}

// Saying no sends only reading from sign-in on.
func TestPluginDecliningHistorySendsOnlyNewReading(t *testing.T) {
	s := newScenario(t)
	y := s.recordPastYear()

	signedIn := s.signInFresh("skip")
	require.Len(t, signedIn.Plugin.Servers, 1)
	assert.Nil(t, signedIn.Plugin.Servers[0].History)
	assert.Empty(t, s.pluginSessions(), "nothing old arrives")
	assert.Empty(t, s.readThroughs(y.cbz))
	assert.Empty(t, s.readThroughs(y.epub))
	assert.Nil(t, s.ratingUnits(y.cbz))

	// Reading after sign-in, which the Device predates.
	_, err := s.db.NewUpdate().Model((*devices.Device)(nil)).Set("created_at = ?", time.Now().Add(-24*time.Hour)).
		Where("id = ?", s.device.ID).Exec(s.ctx)
	require.NoError(t, err)
	now := time.Now().Add(-time.Hour).Truncate(time.Second)
	read := s.run(Args{Command: "plugin-sync", Online: true, Stats: []StatRow{
		{Document: s.documentHash(y.epub), Page: 320, TotalPages: 400, StartTime: now.Unix(), Duration: 60},
	}})
	sessions := s.pluginSessions()
	require.Len(t, sessions, 1, "shown: %v", read.Shown)
	assert.Equal(t, now.Unix(), sessions[0].StartedAt.Unix())
	assert.Equal(t, "complete", read.Plugin.Sidecars["comic.cbz"].Status, "KOReader keeps its own record")
}

// With the setting to send history, a first sign-in asks nothing and sends
// it at once, as answering the question does.
func TestPluginSettingSendsHistoryAtSignIn(t *testing.T) {
	s := newScenario(t)
	y := s.recordPastYear()
	state := filepath.Join(s.h.Dir, "home", ".config", "koreader", "settings", "shisho.lua")
	require.NoError(t, os.WriteFile(state, []byte(`return { settings = { send_history = "send" } }`), 0o644))

	short, err := s.devices.GenerateShortURL(s.ctx, s.user.ID, s.device.ID)
	require.NoError(t, err)
	signedIn := s.run(Args{Command: "plugin-signin", Server: s.url, Code: strings.ToUpper(short.ShortCode), Online: true})
	require.True(t, signedIn.Plugin.SignedIn, signedIn.Plugin.Message)
	assert.NotContains(t, strings.Join(signedIn.Shown, " "), "Send your existing", "nothing asks")
	assert.Len(t, s.pluginSessions(), len(y.sittings), "shown: %v", signedIn.Shown)
	assert.Len(t, s.readThroughs(y.cbz), 1)
}
