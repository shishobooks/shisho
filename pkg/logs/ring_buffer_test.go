package logs

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRingBuffer_WriteAndQuery(t *testing.T) {
	t.Parallel()

	rb := NewRingBuffer(100, nil)

	line := `{"level":"info","timestamp":"2026-04-17T10:30:00Z","message":"starting shisho","data":{"version":"0.0.31"}}` + "\n"
	n, err := rb.Write([]byte(line))
	require.NoError(t, err)
	assert.Equal(t, len(line), n)

	entries := rb.Query("", "", 100, 0)
	require.Len(t, entries, 1)
	assert.Equal(t, "info", entries[0].Level)
	assert.Equal(t, "starting shisho", entries[0].Message)
	assert.Equal(t, uint64(1), entries[0].ID)
	assert.Equal(t, "0.0.31", entries[0].Data["version"])
	assert.Nil(t, entries[0].Error)
}

func TestRingBuffer_ErrorField(t *testing.T) {
	t.Parallel()

	rb := NewRingBuffer(100, nil)

	line := `{"level":"error","timestamp":"2026-04-17T10:30:00Z","message":"db failed","error":"connection refused"}` + "\n"
	_, err := rb.Write([]byte(line))
	require.NoError(t, err)

	entries := rb.Query("", "", 100, 0)
	require.Len(t, entries, 1)
	assert.Equal(t, "error", entries[0].Level)
	require.NotNil(t, entries[0].Error)
	assert.Equal(t, "connection refused", *entries[0].Error)
}

func TestRingBuffer_Wrapping(t *testing.T) {
	t.Parallel()

	rb := NewRingBuffer(3, nil)

	for i := 1; i <= 5; i++ {
		line := fmt.Sprintf(`{"level":"info","timestamp":"2026-04-17T10:30:00Z","message":"msg %d"}`, i) + "\n"
		_, err := rb.Write([]byte(line))
		require.NoError(t, err)
	}

	entries := rb.Query("", "", 100, 0)
	require.Len(t, entries, 3)
	assert.Equal(t, "msg 3", entries[0].Message)
	assert.Equal(t, "msg 4", entries[1].Message)
	assert.Equal(t, "msg 5", entries[2].Message)
	assert.Equal(t, uint64(3), entries[0].ID)
	assert.Equal(t, uint64(5), entries[2].ID)
}

func TestRingBuffer_QueryAfterID(t *testing.T) {
	t.Parallel()

	rb := NewRingBuffer(100, nil)

	for i := 1; i <= 5; i++ {
		line := fmt.Sprintf(`{"level":"info","timestamp":"2026-04-17T10:30:00Z","message":"msg %d"}`, i) + "\n"
		_, err := rb.Write([]byte(line))
		require.NoError(t, err)
	}

	entries := rb.Query("", "", 100, 3)
	require.Len(t, entries, 2)
	assert.Equal(t, "msg 4", entries[0].Message)
	assert.Equal(t, "msg 5", entries[1].Message)
}

func TestRingBuffer_QueryLevelFilter(t *testing.T) {
	t.Parallel()

	rb := NewRingBuffer(100, nil)

	lines := []string{
		`{"level":"debug","timestamp":"2026-04-17T10:30:00Z","message":"debug msg"}` + "\n",
		`{"level":"info","timestamp":"2026-04-17T10:30:01Z","message":"info msg"}` + "\n",
		`{"level":"warn","timestamp":"2026-04-17T10:30:02Z","message":"warn msg"}` + "\n",
		`{"level":"error","timestamp":"2026-04-17T10:30:03Z","message":"error msg"}` + "\n",
	}
	for _, line := range lines {
		_, err := rb.Write([]byte(line))
		require.NoError(t, err)
	}

	entries := rb.Query("warn", "", 100, 0)
	require.Len(t, entries, 2)
	assert.Equal(t, "warn msg", entries[0].Message)
	assert.Equal(t, "error msg", entries[1].Message)
}

func TestRingBuffer_QuerySearchFilter(t *testing.T) {
	t.Parallel()

	rb := NewRingBuffer(100, nil)

	lines := []string{
		`{"level":"info","timestamp":"2026-04-17T10:30:00Z","message":"starting shisho"}` + "\n",
		`{"level":"info","timestamp":"2026-04-17T10:30:01Z","message":"database connected"}` + "\n",
		`{"level":"info","timestamp":"2026-04-17T10:30:02Z","message":"server started"}` + "\n",
	}
	for _, line := range lines {
		_, err := rb.Write([]byte(line))
		require.NoError(t, err)
	}

	entries := rb.Query("", "DATABASE", 100, 0)
	require.Len(t, entries, 1)
	assert.Equal(t, "database connected", entries[0].Message)
}

func TestRingBuffer_QueryLimit(t *testing.T) {
	t.Parallel()

	rb := NewRingBuffer(100, nil)

	for i := 1; i <= 10; i++ {
		line := fmt.Sprintf(`{"level":"info","timestamp":"2026-04-17T10:30:00Z","message":"msg %d"}`, i) + "\n"
		_, err := rb.Write([]byte(line))
		require.NoError(t, err)
	}

	entries := rb.Query("", "", 3, 0)
	require.Len(t, entries, 3)
	assert.Equal(t, "msg 8", entries[0].Message)
	assert.Equal(t, "msg 10", entries[2].Message)
}

func TestRingBuffer_MultiLineWrite(t *testing.T) {
	t.Parallel()

	rb := NewRingBuffer(100, nil)

	batch := `{"level":"info","timestamp":"2026-04-17T10:30:00Z","message":"first"}` + "\n" +
		`{"level":"info","timestamp":"2026-04-17T10:30:01Z","message":"second"}` + "\n"
	_, err := rb.Write([]byte(batch))
	require.NoError(t, err)

	entries := rb.Query("", "", 100, 0)
	require.Len(t, entries, 2)
	assert.Equal(t, "first", entries[0].Message)
	assert.Equal(t, "second", entries[1].Message)
}

func TestRingBuffer_InvalidJSON(t *testing.T) {
	t.Parallel()

	rb := NewRingBuffer(100, nil)

	_, err := rb.Write([]byte("not json\n"))
	require.NoError(t, err)

	entries := rb.Query("", "", 100, 0)
	assert.Empty(t, entries)
}

func TestRingBuffer_EmptyBufferQuery(t *testing.T) {
	t.Parallel()

	rb := NewRingBuffer(100, nil)

	entries := rb.Query("", "", 100, 0)
	assert.Empty(t, entries)
}

func TestRingBuffer_RootLevelFields(t *testing.T) {
	t.Parallel()

	rb := NewRingBuffer(100, nil)

	// Zerolog puts Root() fields at the JSON root level, not under "data"
	line := `{"level":"info","timestamp":"2026-04-17T10:30:00Z","message":"request handled","method":"GET","path":"/books","route":"/books","status_code":200,"duration":"1.234","hostname":"myhost"}` + "\n"
	_, err := rb.Write([]byte(line))
	require.NoError(t, err)

	entries := rb.Query("", "", 100, 0)
	require.Len(t, entries, 1)
	assert.Equal(t, "request handled", entries[0].Message)
	// Root-level fields should appear in Data (except known fields like hostname)
	assert.Equal(t, "GET", entries[0].Data["method"])
	assert.Equal(t, "/books", entries[0].Data["path"])
	assert.Equal(t, "/books", entries[0].Data["route"])
	assert.EqualValues(t, 200, entries[0].Data["status_code"])
	assert.Equal(t, "1.234", entries[0].Data["duration"])
	// hostname is excluded
	assert.Nil(t, entries[0].Data["hostname"])
}

func TestRingBuffer_RootAndNestedDataMerged(t *testing.T) {
	t.Parallel()

	rb := NewRingBuffer(100, nil)

	// Both nested "data" and root-level fields should appear in Data
	line := `{"level":"info","timestamp":"2026-04-17T10:30:00Z","message":"starting","data":{"version":"0.0.31"},"id":"abc-123"}` + "\n"
	_, err := rb.Write([]byte(line))
	require.NoError(t, err)

	entries := rb.Query("", "", 100, 0)
	require.Len(t, entries, 1)
	// Nested data
	assert.Equal(t, "0.0.31", entries[0].Data["version"])
	// Root-level field
	assert.Equal(t, "abc-123", entries[0].Data["id"])
}

func TestRingBuffer_SkipLogsRoute(t *testing.T) {
	t.Parallel()

	rb := NewRingBuffer(100, nil)

	// Requests to /logs should be skipped to prevent feedback loops
	line := `{"level":"info","timestamp":"2026-04-17T10:30:00Z","message":"request handled","route":"/logs","method":"GET","status_code":200}` + "\n"
	_, err := rb.Write([]byte(line))
	require.NoError(t, err)

	entries := rb.Query("", "", 100, 0)
	assert.Empty(t, entries)
}

func TestRingBuffer_SkipEventsRoute(t *testing.T) {
	t.Parallel()

	rb := NewRingBuffer(100, nil)

	// SSE endpoint should also be skipped
	line := `{"level":"info","timestamp":"2026-04-17T10:30:00Z","message":"request handled","route":"/events","method":"GET"}` + "\n"
	_, err := rb.Write([]byte(line))
	require.NoError(t, err)

	entries := rb.Query("", "", 100, 0)
	assert.Empty(t, entries)
}

func TestRingBuffer_SkipHealthRoute(t *testing.T) {
	t.Parallel()

	rb := NewRingBuffer(100, nil)

	line := `{"level":"info","timestamp":"2026-04-17T10:30:00Z","message":"request handled","route":"/health","method":"GET","status_code":200}` + "\n"
	_, err := rb.Write([]byte(line))
	require.NoError(t, err)

	entries := rb.Query("", "", 100, 0)
	assert.Empty(t, entries)
}

func TestRingBuffer_SearchMatchesDataFields(t *testing.T) {
	t.Parallel()

	rb := NewRingBuffer(100, nil)

	lines := []string{
		`{"level":"info","timestamp":"2026-04-17T10:30:00Z","message":"request handled","method":"GET","path":"/books","duration":"1.234"}` + "\n",
		`{"level":"info","timestamp":"2026-04-17T10:30:01Z","message":"request handled","method":"POST","path":"/jobs","duration":"5.678"}` + "\n",
		`{"level":"info","timestamp":"2026-04-17T10:30:02Z","message":"starting shisho","data":{"version":"0.0.31"}}` + "\n",
	}
	for _, line := range lines {
		_, err := rb.Write([]byte(line))
		require.NoError(t, err)
	}

	// Search by data field value
	entries := rb.Query("", "/books", 100, 0)
	require.Len(t, entries, 1)
	assert.Equal(t, "/books", entries[0].Data["path"])

	// Search by data field key
	entries = rb.Query("", "duration", 100, 0)
	require.Len(t, entries, 2)

	// Search by nested data value
	entries = rb.Query("", "0.0.31", 100, 0)
	require.Len(t, entries, 1)
}
