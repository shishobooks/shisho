//go:build unix

package serverstats

import (
	"syscall"

	"github.com/pkg/errors"
)

func statFilesystem(path string) (volumeStat, error) {
	var st syscall.Stat_t
	if err := syscall.Stat(path, &st); err != nil {
		return volumeStat{}, errors.Wrapf(err, "stat %s", path)
	}
	var fs syscall.Statfs_t
	if err := syscall.Statfs(path, &fs); err != nil {
		return volumeStat{}, errors.Wrapf(err, "statfs %s", path)
	}
	blockSize := blockUnit(&fs)
	return volumeStat{
		id:    uint64(st.Dev), //nolint:gosec,unconvert // Dev is uint64 on Linux and int32 on macOS
		total: fs.Blocks * blockSize,
		// Free is what an unprivileged process can still write, which is
		// what Shisho can use. Blocks reserved for root are in neither.
		free: fs.Bavail * blockSize,
		used: (fs.Blocks - fs.Bfree) * blockSize,
	}, nil
}
