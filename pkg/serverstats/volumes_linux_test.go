//go:build linux

package serverstats

import (
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Linux counts statfs blocks in fragment-size units. Network filesystems
// (NFS, SMB, virtiofs) report a much larger preferred I/O size in Bsize, so
// using it would multiply every size.
func TestBlockUnit_UsesFragmentSizeOnLinux(t *testing.T) {
	t.Parallel()
	assert.Equal(t, uint64(4096), blockUnit(&syscall.Statfs_t{Bsize: 1 << 20, Frsize: 4096}))
	// A filesystem that leaves the fragment size unset falls back to Bsize.
	assert.Equal(t, uint64(4096), blockUnit(&syscall.Statfs_t{Bsize: 4096}))
}
