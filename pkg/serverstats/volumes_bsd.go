//go:build unix && !linux

package serverstats

import "syscall"

// blockUnit is the size of the blocks statfs counts. macOS and the BSDs
// report it as Bsize and have no separate fragment size.
func blockUnit(fs *syscall.Statfs_t) uint64 {
	return uint64(fs.Bsize) //nolint:gosec,unconvert // uint32 on macOS, never negative
}
