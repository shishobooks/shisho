//go:build linux

package serverstats

import "syscall"

// blockUnit is the size of the blocks statfs counts. Linux counts them in
// the fragment size; Bsize is only the preferred I/O size, which network
// filesystems set far larger (virtiofs reports 256 times the fragment size).
func blockUnit(fs *syscall.Statfs_t) uint64 {
	if fs.Frsize > 0 {
		return uint64(fs.Frsize) //nolint:gosec // never negative
	}
	return uint64(fs.Bsize) //nolint:gosec // never negative
}
