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
	assert.Len(t, entries, 0)
}

func TestRingBuffer_EmptyBufferQuery(t *testing.T) {
	t.Parallel()

	rb := NewRingBuffer(100, nil)

	entries := rb.Query("", "", 100, 0)
	assert.Len(t, entries, 0)
}
