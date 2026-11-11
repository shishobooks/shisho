package serverstats

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/errors"
)

// volumeStat is one filesystem's space. id is the same for every path on
// the filesystem, so paths sharing a mount are reported once.
type volumeStat struct {
	id    uint64
	total uint64
	free  uint64
	used  uint64
}

// statVolume reports the filesystem holding path. A path Shisho creates on
// first use (the cache directory) is reported, while it doesn't exist yet,
// on the filesystem of its nearest existing parent, where it will be
// created. With mustExist, a missing path is an error instead: a library
// folder on a drive that isn't mounted leaves its empty mount point on
// another filesystem, whose space isn't the library's.
func statVolume(path string, mustExist bool) (volumeStat, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return volumeStat{}, errors.WithStack(err)
	}
	for {
		_, err := os.Stat(path)
		if err == nil {
			break
		}
		parent := filepath.Dir(path)
		if mustExist || !os.IsNotExist(err) || parent == path {
			return volumeStat{}, errors.WithStack(err)
		}
		path = parent
	}
	return statFilesystem(path)
}

// statVolumeWithin runs stat in the background and gives up after timeout
// or when ctx ends. A stat on a hung network mount blocks without limit, so
// it is left running rather than holding up the page.
func statVolumeWithin(
	ctx context.Context,
	timeout time.Duration,
	stat func(path string, mustExist bool) (volumeStat, error),
	path string,
	mustExist bool,
) (volumeStat, error) {
	type result struct {
		stat volumeStat
		err  error
	}
	done := make(chan result, 1)
	go func() {
		s, err := stat(path, mustExist)
		done <- result{s, err}
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case r := <-done:
		return r.stat, r.err
	case <-timer.C:
		return volumeStat{}, errors.Errorf("no answer from %s after %s", path, timeout)
	case <-ctx.Done():
		return volumeStat{}, errors.WithStack(ctx.Err())
	}
}
