package database

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsBusyError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "database is locked",
			err:      errors.New("database is locked"),
			expected: true,
		},
		{
			name:     "database table is locked",
			err:      errors.New("database table is locked"),
			expected: true,
		},
		{
			name:     "SQLITE_BUSY",
			err:      errors.New("SQLITE_BUSY"),
			expected: true,
		},
		{
			name:     "SQLITE_LOCKED",
			err:      errors.New("SQLITE_LOCKED"),
			expected: true,
		},
		{
			name:     "error code 5",
			err:      errors.New("error (5): database busy"),
			expected: true,
		},
		{
			name:     "error code 6",
			err:      errors.New("error (6): database locked"),
			expected: true,
		},
		{
			name:     "unrelated error",
			err:      errors.New("connection refused"),
			expected: false,
		},
		{
			name:     "constraint violation",
			err:      errors.New("UNIQUE constraint failed"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isBusyError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRetryWithBackoff(t *testing.T) {
	t.Run("succeeds on first attempt", func(t *testing.T) {
		attempts := 0
		err := retryWithBackoff(context.Background(), 5, func() error {
			attempts++
			return nil
		})
		require.NoError(t, err)
		assert.Equal(t, 1, attempts)
	})

	t.Run("retries on busy error and succeeds", func(t *testing.T) {
		attempts := 0
		err := retryWithBackoff(context.Background(), 5, func() error {
			attempts++
			if attempts < 3 {
				return errors.New("database is locked")
			}
			return nil
		})
		require.NoError(t, err)
		assert.Equal(t, 3, attempts)
	})

	t.Run("fails immediately on non-busy error", func(t *testing.T) {
		attempts := 0
		err := retryWithBackoff(context.Background(), 5, func() error {
			attempts++
			return errors.New("connection refused")
		})
		require.Error(t, err)
		assert.Equal(t, 1, attempts)
		assert.Contains(t, err.Error(), "connection refused")
	})

	t.Run("exhausts all retries on persistent busy error", func(t *testing.T) {
		attempts := 0
		err := retryWithBackoff(context.Background(), 3, func() error {
			attempts++
			return errors.New("database is locked")
		})
		require.Error(t, err)
		assert.Equal(t, 4, attempts) // 1 initial + 3 retries
		assert.Contains(t, err.Error(), "database is locked")
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		attempts := 0

		go func() {
			time.Sleep(10 * time.Millisecond)
			cancel()
		}()

		err := retryWithBackoff(ctx, 10, func() error {
			attempts++
			return errors.New("database is locked")
		})

		require.Error(t, err)
		require.ErrorIs(t, err, context.Canceled)
		// Should have made at least 1 attempt but not all 10
		assert.GreaterOrEqual(t, attempts, 1)
		assert.Less(t, attempts, 10)
	})

	t.Run("zero retries means one attempt only", func(t *testing.T) {
		attempts := 0
		err := retryWithBackoff(context.Background(), 0, func() error {
			attempts++
			return errors.New("database is locked")
		})
		require.Error(t, err)
		assert.Equal(t, 1, attempts)
	})
}
