package database

import (
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/shishobooks/shisho/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestConfig creates a config with a temp file database.
// Using a file instead of :memory: ensures multiple connections share
// the same database, which is required to test lock contention.
func newTestConfig(t *testing.T) *config.Config {
	t.Helper()
	tmpDir := t.TempDir()
	cfg := config.NewForTest()
	cfg.DatabaseFilePath = filepath.Join(tmpDir, "test.db")
	// Reduce retry safety nets to make lock errors surface faster.
	// Without MaxOpenConns(1), this should reliably produce SQLITE_BUSY errors.
	cfg.DatabaseMaxRetries = 0          // No retries
	cfg.DatabaseBusyTimeout = 1_000_000 // 1ms busy timeout (in nanoseconds via time.Duration)
	return cfg
}

// TestConcurrentWrites verifies that concurrent database writes complete
// successfully without "database is locked" errors. This tests the fix
// where MaxOpenConns(1) serializes all operations through a single connection.
func TestConcurrentWrites(t *testing.T) {
	t.Parallel()

	cfg := newTestConfig(t)
	db, err := New(cfg)
	require.NoError(t, err)
	defer db.Close()

	// Create a simple test table
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS concurrency_test (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		value TEXT NOT NULL,
		worker_id INTEGER NOT NULL
	)`)
	require.NoError(t, err)

	const numWorkers = 20
	const writesPerWorker = 50

	var wg sync.WaitGroup
	var errorCount atomic.Int32
	var successCount atomic.Int32
	errors := make(chan error, numWorkers*writesPerWorker)

	// Spawn workers to do concurrent writes
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < writesPerWorker; i++ {
				_, err := db.Exec(
					"INSERT INTO concurrency_test (value, worker_id) VALUES (?, ?)",
					fmt.Sprintf("worker-%d-write-%d", workerID, i),
					workerID,
				)
				if err != nil {
					errorCount.Add(1)
					errors <- fmt.Errorf("worker %d write %d: %w", workerID, i, err)
				} else {
					successCount.Add(1)
				}
			}
		}(w)
	}

	wg.Wait()
	close(errors)

	// Collect any errors for reporting
	var allErrors []error
	for err := range errors {
		allErrors = append(allErrors, err)
	}

	// Assert no errors occurred
	assert.Empty(t, allErrors, "concurrent writes should not produce errors")
	assert.Equal(t, int32(0), errorCount.Load(), "error count should be 0")
	assert.Equal(t, int32(numWorkers*writesPerWorker), successCount.Load(),
		"all writes should succeed")

	// Verify all rows were written
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM concurrency_test").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, numWorkers*writesPerWorker, count,
		"all rows should be present in database")
}

// TestConcurrentMixedOperations verifies that concurrent reads and writes
// complete successfully. This is a more realistic workload.
func TestConcurrentMixedOperations(t *testing.T) {
	t.Parallel()

	cfg := newTestConfig(t)
	db, err := New(cfg)
	require.NoError(t, err)
	defer db.Close()

	// Create and seed test table
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS mixed_test (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		value INTEGER NOT NULL
	)`)
	require.NoError(t, err)

	// Insert some initial data
	for i := 0; i < 100; i++ {
		_, err = db.Exec("INSERT INTO mixed_test (value) VALUES (?)", i)
		require.NoError(t, err)
	}

	const numWorkers = 8
	const opsPerWorker = 100

	var wg sync.WaitGroup
	var writeErrors atomic.Int32
	var readErrors atomic.Int32
	var writes atomic.Int32
	var reads atomic.Int32

	// Half workers do writes, half do reads
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		if w%2 == 0 {
			// Writer
			go func(workerID int) {
				defer wg.Done()
				for i := 0; i < opsPerWorker; i++ {
					_, err := db.Exec("INSERT INTO mixed_test (value) VALUES (?)", workerID*1000+i)
					if err != nil {
						writeErrors.Add(1)
					} else {
						writes.Add(1)
					}
				}
			}(w)
		} else {
			// Reader
			go func(workerID int) {
				defer wg.Done()
				for i := 0; i < opsPerWorker; i++ {
					var sum int
					err := db.QueryRow("SELECT SUM(value) FROM mixed_test").Scan(&sum)
					if err != nil {
						readErrors.Add(1)
					} else {
						reads.Add(1)
					}
				}
			}(w)
		}
	}

	wg.Wait()

	// Assert no errors
	assert.Equal(t, int32(0), writeErrors.Load(), "no write errors should occur")
	assert.Equal(t, int32(0), readErrors.Load(), "no read errors should occur")

	// Verify expected operation counts
	expectedWrites := int32((numWorkers / 2) * opsPerWorker)
	expectedReads := int32((numWorkers / 2) * opsPerWorker)
	assert.Equal(t, expectedWrites, writes.Load(), "all writes should complete")
	assert.Equal(t, expectedReads, reads.Load(), "all reads should complete")
}
