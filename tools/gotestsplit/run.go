package main

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jstemmer/go-junit-report/v2/gtr"
	"github.com/jstemmer/go-junit-report/v2/junit"
	parser "github.com/jstemmer/go-junit-report/v2/parser/gotest"
)

var (
	errRunFlagsInvalid  = errors.New("run requires -junit-dir, -total>0, 0<=-index<-total")
	errRunNeedsPackages = errors.New("run requires at least one package pattern")
)

func cmdRun(ctx context.Context, argv []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	junitDir := fs.String("junit-dir", "", "directory to read prior JUnit XML from and write new XML to (required)")
	total := fs.Int("total", 0, "total number of shards (required)")
	index := fs.Int("index", 0, "zero-based shard index")
	if err := fs.Parse(argv); err != nil {
		return err
	}
	if *junitDir == "" || *total <= 0 || *index < 0 || *index >= *total {
		return errRunFlagsInvalid
	}

	args := fs.Args()
	var pkgs, testOpts []string
	for i, a := range args {
		if a == "--" {
			testOpts = args[i+1:]
			break
		}
		pkgs = append(pkgs, a)
	}
	if len(pkgs) == 0 {
		return errRunNeedsPackages
	}

	if err := os.MkdirAll(*junitDir, 0o755); err != nil {
		return err
	}
	hist, err := ReadHistory(*junitDir)
	if err != nil {
		return err
	}
	discovered, err := DiscoverPackages(ctx, pkgs, testOpts)
	if err != nil {
		return err
	}
	shards := Pack(hist, discovered, *total)
	mine := shards[*index]

	// `go test` invocations: one for whole-pkg items grouped together (cheap),
	// then one per chunk (each is an additional compile).
	var wholePkgs []string
	var chunkItems []Item
	for _, it := range mine {
		if len(it.Tests) == 0 {
			wholePkgs = append(wholePkgs, it.Pkg)
		} else {
			chunkItems = append(chunkItems, it)
		}
	}

	// Force -v so JUnit parser has per-test output.
	if !hasFlag(testOpts, "-v") {
		testOpts = append([]string{"-v"}, testOpts...)
	}

	seq := 0
	runOne := func(extraArgs []string) error {
		seq++
		goArgs := append([]string{"test"}, testOpts...)
		goArgs = append(goArgs, extraArgs...)
		fmt.Fprintf(stderr, "gotestsplit: shard %d/%d step %d: go %s\n",
			*index+1, *total, seq, strings.Join(goArgs, " "))
		report, runErr := goTest(ctx, goArgs, stdout, stderr)
		if report != nil {
			path := filepath.Join(*junitDir,
				fmt.Sprintf("junit-%d-%d.xml", *index, seq))
			if werr := writeJUnit(path, *report); werr != nil {
				fmt.Fprintf(stderr, "gotestsplit: failed to write %s: %v\n", path, werr)
			}
		}
		return runErr
	}

	var firstErr error
	if len(wholePkgs) > 0 {
		if err := runOne(wholePkgs); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	for _, it := range chunkItems {
		runRegex := "^(?:" + strings.Join(it.Tests, "|") + ")$"
		if err := runOne([]string{"-run", runRegex, it.Pkg}); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func hasFlag(opts []string, name string) bool {
	for _, o := range opts {
		if o == name {
			return true
		}
	}
	return false
}

// goTest runs `go <args>` and returns a parsed JUnit report alongside the run
// error. Stdout/stderr stream to the supplied writers and are also captured for
// the JUnit parser.
func goTest(ctx context.Context, args []string, stdout, stderr io.Writer) (*gtr.Report, error) {
	cmd := exec.CommandContext(ctx, "go", args...)
	var buf bytes.Buffer
	cmd.Stdout = io.MultiWriter(stdout, &buf)
	cmd.Stderr = io.MultiWriter(stderr, &buf)
	runErr := cmd.Run()
	report, parseErr := parser.NewParser().Parse(&buf)
	if parseErr != nil {
		return nil, errors.Join(runErr, parseErr)
	}
	return &report, runErr
}

func writeJUnit(path string, report gtr.Report) error {
	suites := junit.CreateFromReport(report, "")
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.WriteString(xml.Header); err != nil {
		return err
	}
	enc := xml.NewEncoder(f)
	enc.Indent("", "\t")
	if err := enc.Encode(suites); err != nil {
		return err
	}
	return enc.Flush()
}
