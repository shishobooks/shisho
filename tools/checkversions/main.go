// Command checkversions fails when a tool pinned in mise.toml is pinned to a
// different version in the Dockerfile (golang and node base images, the tygo
// `go install`) or package.json (packageManager pnpm, the @types/node major).
// Docker does not use mise, so these pins are copies that must move together.
//
// Run it with `mise lint:versions`.
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	problems, err := run()
	if err != nil {
		fmt.Fprintln(os.Stderr, "check-versions:", err)
		os.Exit(2)
	}
	for _, p := range problems {
		fmt.Println(p)
	}
	if len(problems) > 0 {
		fmt.Fprintf(os.Stderr, "check-versions: %d tool pin(s) disagree with mise.toml. mise.toml is the source of truth; update the other pin to match (see \"Tool versions\" in AGENTS.md).\n", len(problems))
		os.Exit(1)
	}
}

func run() ([]string, error) {
	top, err := exec.CommandContext(context.Background(), "git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return nil, fmt.Errorf("git rev-parse: %w", err)
	}
	root := strings.TrimSpace(string(top))
	var files [3][]byte
	for i, name := range []string{"mise.toml", "Dockerfile", "package.json"} {
		files[i], err = os.ReadFile(filepath.Join(root, name))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
	}
	return Check(files[0], files[1], files[2]), nil
}
