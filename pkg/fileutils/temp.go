package fileutils

import (
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

// maxTempTries bounds the retries on a name collision, as os.CreateTemp does.
const maxTempTries = 10000

// CreateTemp is os.CreateTemp with a mode: it creates a new file in dir,
// named from pattern with the last "*" replaced by a random string (or the
// string appended when there is no "*"), and opens it for reading and writing.
//
// The file gets perm at creation, filtered by the umask and any ACL the
// directory passes on, and it is never chmodded afterwards. ACL-managed storage
// such as a ZFS dataset with aclmode=restricted refuses chmod outright, and a
// chmod would also override an operator's stricter umask. Create a file with
// the mode it should end up with instead of fixing the mode afterwards.
//
// The create is exclusive, so a name another writer already holds is skipped
// rather than opened.
func CreateTemp(dir, pattern string, perm os.FileMode) (*os.File, error) {
	return createTemp(dir, pattern, perm, randomTempName)
}

// MkdirTemp is os.MkdirTemp with a mode. See CreateTemp for why the mode is
// set at creation.
func MkdirTemp(dir, pattern string, perm os.FileMode) (string, error) {
	return mkdirTemp(dir, pattern, perm, randomTempName)
}

func createTemp(dir, pattern string, perm os.FileMode, next func() string) (*os.File, error) {
	var f *os.File
	_, err := tryTempNames(dir, pattern, next, func(name string) error {
		var err error
		f, err = os.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, perm)
		return err
	})
	if err != nil {
		return nil, err
	}
	return f, nil
}

func mkdirTemp(dir, pattern string, perm os.FileMode, next func() string) (string, error) {
	return tryTempNames(dir, pattern, next, func(name string) error {
		return os.Mkdir(name, perm)
	})
}

// tryTempNames calls create with fresh names from pattern until one does not
// already exist, and returns that name.
func tryTempNames(dir, pattern string, next func() string, create func(name string) error) (string, error) {
	if dir == "" {
		dir = os.TempDir()
	}
	if strings.ContainsRune(pattern, os.PathSeparator) {
		return "", errors.Errorf("pattern %q contains a path separator", pattern)
	}
	prefix, suffix := pattern, ""
	if i := strings.LastIndexByte(pattern, '*'); i >= 0 {
		prefix, suffix = pattern[:i], pattern[i+1:]
	}

	for range maxTempTries {
		name := filepath.Join(dir, prefix+next()+suffix)
		err := create(name)
		if err == nil {
			return name, nil
		}
		if !os.IsExist(err) {
			return "", errors.WithStack(err)
		}
	}
	return "", errors.WithStack(&os.PathError{
		Op:   "createtemp",
		Path: filepath.Join(dir, prefix+"*"+suffix),
		Err:  os.ErrExist,
	})
}

func randomTempName() string {
	return strconv.FormatUint(uint64(rand.Uint32()), 10)
}
