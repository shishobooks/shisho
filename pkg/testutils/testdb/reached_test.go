package testdb

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A probe fires while its statement runs, so canceling then interrupts the
// statement, and an unused probe never fires.
func TestReached_SignalsWhileTheStatementRuns(t *testing.T) {
	t.Parallel()
	db := New(t)
	expr, reached := Reached(t)
	_, unused := Reached(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		// This query never finishes on its own.
		_, err := db.ExecContext(ctx, `WITH RECURSIVE c(x) AS (SELECT `+expr+` UNION ALL SELECT x + 1 FROM c) SELECT count(*) FROM c`)
		result <- err
	}()
	select {
	case <-reached:
	case <-time.After(5 * time.Second):
		t.Fatal("the statement never evaluated the probe")
	}
	cancel()
	select {
	case err := <-result:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("canceling did not interrupt the statement")
	}

	select {
	case <-unused:
		assert.Fail(t, "a probe no statement evaluated fired")
	default:
	}
}
