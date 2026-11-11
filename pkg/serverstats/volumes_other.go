//go:build !unix

package serverstats

import "github.com/pkg/errors"

func statFilesystem(string) (volumeStat, error) {
	return volumeStat{}, errors.New("volume space is only reported on Unix")
}
