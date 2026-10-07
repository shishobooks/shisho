package devtool

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

// Environment locates the worktree a command runs in and the main worktree
// that every worktree of the repository shares.
type Environment struct {
	MainRoot    string
	CurrentRoot string
}

// PortLockRoot is where worktrees record the ports they hold. It lives in the
// main worktree so that every linked worktree sees the same locks.
func (e Environment) PortLockRoot() string {
	return filepath.Join(e.MainRoot, "tmp", "ports")
}

// CookieNamespace names this worktree's session cookie. The main worktree
// keeps the default cookie name. A linked worktree gets its directory name
// plus a hash of its path, so two worktrees with the same directory name in
// different places still differ.
func (e Environment) CookieNamespace() string {
	if e.MainRoot == e.CurrentRoot {
		return ""
	}
	sum := sha256.Sum256([]byte(e.CurrentRoot))
	return slug(filepath.Base(e.CurrentRoot)) + "_" + hex.EncodeToString(sum[:4])
}

// slug lowercases name and replaces each run of characters a cookie name
// can't hold with one underscore.
func slug(name string) string {
	var builder strings.Builder
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
		} else if builder.Len() > 0 && !strings.HasSuffix(builder.String(), "_") {
			builder.WriteByte('_')
		}
	}
	result := strings.TrimSuffix(builder.String(), "_")
	if result == "" {
		return "worktree"
	}
	return result
}

// ResolveEnvironment describes the worktree containing the working directory.
func ResolveEnvironment(ctx context.Context) (Environment, error) {
	dir, err := os.Getwd()
	if err != nil {
		return Environment{}, errors.WithStack(err)
	}
	return resolveEnvironmentAt(ctx, dir)
}

func resolveEnvironmentAt(ctx context.Context, dir string) (Environment, error) {
	currentRoot, err := gitOutput(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return Environment{}, err
	}
	commonDir, err := gitOutput(ctx, dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return Environment{}, err
	}
	mainRoot, err := filepath.EvalSymlinks(filepath.Dir(commonDir))
	if err != nil {
		return Environment{}, errors.Wrap(err, "resolve main worktree path")
	}
	currentRoot, err = filepath.EvalSymlinks(currentRoot)
	if err != nil {
		return Environment{}, errors.Wrap(err, "resolve current worktree path")
	}
	return Environment{MainRoot: mainRoot, CurrentRoot: currentRoot}, nil
}

func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, "git", args...)
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil {
		return "", errors.Wrapf(err, "git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}
