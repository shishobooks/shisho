// Command checkdocrefs fails when an agent instruction file (every AGENTS.md,
// CODING_STANDARDS.md, and docs/agents/**/*.md) names a repo path, code
// identifier, or quoted section that no longer exists.
//
// Run it through scripts/check-docrefs.sh (or `mise lint:docrefs`).
// Intentional mentions of things that do not exist go in allowlist.txt.
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"sort"
	"strings"
)

func main() {
	allowPath := flag.String("allowlist", "tools/checkdocrefs/allowlist.txt", "file of tokens to accept, relative to the repo root")
	flag.Parse()
	n, err := run(*allowPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "check-docrefs:", err)
		os.Exit(2)
	}
	if n > 0 {
		fmt.Fprintf(os.Stderr, "check-docrefs: %d stale reference(s) in agent instruction files or the allowlist. Fix or delete the line, or add an intentional mention to tools/checkdocrefs/allowlist.txt.\n", n)
		os.Exit(1)
	}
}

func run(allowPath string) (int, error) {
	ctx := context.Background()
	top, err := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return 0, fmt.Errorf("git rev-parse: %w", err)
	}
	root := strings.TrimSpace(string(top))
	fsys := os.DirFS(root)
	// Untracked files count, so the check passes before a new file is added.
	out, err := exec.CommandContext(ctx, "git", "-C", root,
		"ls-files", "-z", "--cached", "--others", "--exclude-standard").Output()
	if err != nil {
		return 0, fmt.Errorf("git ls-files: %w", err)
	}
	var paths []string
	for p := range bytes.SplitSeq(out, []byte{0}) {
		if len(p) > 0 {
			paths = append(paths, string(p))
		}
	}
	sort.Strings(paths)
	repo, err := NewRepo(fsys, paths)
	if err != nil {
		return 0, err
	}
	repo.ignored = func(p string) bool {
		// Exit status 0 means ignored. The path may be absent (a CI checkout
		// directory), so also try it as a directory to match `dir/` patterns.
		for _, q := range []string{p, p + "/"} {
			if exec.CommandContext(ctx, "git", "-C", root, "check-ignore", "-q", "--no-index", "--", q).Run() == nil {
				return true
			}
		}
		return false
	}
	allowData, err := fs.ReadFile(fsys, allowPath)
	if err != nil {
		return 0, fmt.Errorf("read allowlist: %w", err)
	}
	allow := ParseAllowlist(allowData)
	found := 0
	for _, p := range paths {
		if !isInstructionDoc(p) || !repo.files[p] {
			continue
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return 0, fmt.Errorf("read %s: %w", p, err)
		}
		for _, f := range CheckDoc(repo, p, data, allow) {
			fmt.Println(f)
			found++
		}
	}
	for _, t := range allow.Unused() {
		fmt.Printf("%s: %s: no instruction doc uses this allowlist entry; delete it\n", allowPath, t)
		found++
	}
	return found, nil
}
