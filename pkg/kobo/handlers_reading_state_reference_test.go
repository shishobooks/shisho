package kobo

import (
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A Reference Book goes to the Kobo as ready to read, with its place, since
// a Kobo has no status for a Book dipped into (ADR 0028).
func TestKoboReadingState_ReferenceGoesAsReadyToReadWithItsBookmark(t *testing.T) {
	t.Parallel()
	f := newReadingStateFixture(t)
	f.put(koboState{status: "Reading", at: time.Now().Add(-time.Hour), source: "OEBPS/text/one.xhtml", span: "kobo.2.1", progress: 8})
	require.NoError(t, f.reading.MarkReference(f.ctx, f.user.ID, f.bookA.ID))

	states := f.changedStates()

	require.Len(t, states, 1)
	require.NotNil(t, states[0].StatusInfo)
	assert.Equal(t, koboStatusReadyToRead, states[0].StatusInfo.Status)
	require.NotNil(t, states[0].CurrentBookmark)
	require.NotNil(t, states[0].CurrentBookmark.Location)
	assert.Equal(t, "kobo.2.1", states[0].CurrentBookmark.Location.Value)
}

// Reading and Finished from the Kobo move the place and add time to a
// Reference Book, but leave its status alone.
func TestKoboReadingState_ReferenceIgnoresReportedStatuses(t *testing.T) {
	t.Parallel()
	f := newReadingStateFixture(t)
	require.NoError(t, f.reading.MarkReference(f.ctx, f.user.ID, f.bookA.ID))
	start := time.Now().Add(-time.Hour).Truncate(time.Second)

	f.put(koboState{status: "Reading", at: start, minutes: minutes(5), source: "OEBPS/text/one.xhtml", span: "kobo.2.1", progress: 10})
	f.put(koboState{status: "Finished", at: start.Add(30 * time.Minute), minutes: minutes(20), source: "OEBPS/text/two.xhtml", span: "kobo.40.1", progress: 100, chapter: 100})

	assert.Equal(t, models.ReadingStatusReference, f.status())
	assert.Empty(t, f.bookReading().ReadThroughs)
	position, err := f.reading.RetrievePosition(f.ctx, f.user.ID, f.fileA.ID)
	require.NoError(t, err)
	require.NotNil(t, position)
	assert.InDelta(t, 1.0, position.Percentage, 0.0001, "the place still moves")
	assert.NotEmpty(t, f.sessions(), "the time still comes through")
}
