//go:build unix

// Package testumask sets the process umask for the length of a test.
//
// The umask is process-wide, so a test that calls Set must not call
// t.Parallel(). Go pauses every parallel test until the sequential top-level
// tests have finished, so a sequential top-level test sees no other test
// creating files while it runs.
package testumask

import (
	"syscall"
	"testing"
)

// Set changes the process umask to mask and restores the previous value when
// the test ends.
func Set(t testing.TB, mask int) {
	t.Helper()
	old := syscall.Umask(mask)
	t.Cleanup(func() { syscall.Umask(old) })
}
