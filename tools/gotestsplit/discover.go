package main

import (
	"bytes"
	"context"
	"os/exec"
	"regexp"
	"sort"
	"strings"
)

// DiscoverPackages runs `go test -list . <pkgs...>` and returns the discovered
// top-level Test/Example functions per resolved package import path.
//
// goArgs are extra flags to forward to `go test` (e.g. -race, -tags=foo). They
// affect compilation and therefore which tests are listed; we forward them so
// build-tagged tests aren't silently skipped.
func DiscoverPackages(ctx context.Context, pkgPatterns, goArgs []string) ([]Package, error) {
	args := []string{"test", "-list", "."}
	if tags := detectTags(goArgs); tags != "" {
		args = append(args, tags)
	}
	if detectRace(goArgs) {
		args = append(args, "-race")
	}
	args = append(args, pkgPatterns...)

	var buf bytes.Buffer
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		return nil, &goListError{err: err, output: buf.String()}
	}
	return parseListOutput(buf.String()), nil
}

type goListError struct {
	err    error
	output string
}

func (e *goListError) Error() string { return e.err.Error() + ": " + e.output }

var tagsRE = regexp.MustCompile(`^--?tags(=.*)?$`)

func detectTags(argv []string) string {
	l := len(argv)
	for i := 0; i < l; i++ {
		t := argv[i]
		m := tagsRE.FindStringSubmatch(t)
		if len(m) < 2 {
			continue
		}
		if m[1] == "" && i+1 < l {
			t += "=" + argv[i+1]
		}
		return t
	}
	return ""
}

func detectRace(argv []string) bool {
	for _, a := range argv {
		if a == "-race" || a == "--race" {
			return true
		}
	}
	return false
}

// parseListOutput parses interleaved `go test -list` output blocks of the form:
//
//	TestFoo
//	TestBar
//	ok   github.com/example/pkg  0.123s
func parseListOutput(out string) []Package {
	var (
		pkgs []Package
		cur  []string
	)
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "Test") || strings.HasPrefix(line, "Example") {
			cur = append(cur, line)
			continue
		}
		if strings.HasPrefix(line, "ok ") {
			f := strings.Fields(line)
			if len(f) < 2 {
				continue
			}
			sort.Strings(cur)
			pkgs = append(pkgs, Package{Path: f[1], Tests: append([]string(nil), cur...)})
			cur = nil
		}
	}
	sort.Slice(pkgs, func(i, j int) bool { return pkgs[i].Path < pkgs[j].Path })
	return pkgs
}
