# gotestsplit Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the CI use of `gotesplit` with an in-tree, timing-aware Go test splitter (`tools/gotestsplit`) that bin-packs by measured per-test duration and exposes a `simulate` subcommand for choosing the shard count.

**Architecture:** Forked from gotesplit v0.4.0 (MIT, Songmu). A pure `pack(history, packages, N)` function drives three subcommands (`simulate`, `plan`, `run`). History is parsed from JUnit XML files cached between CI runs via `actions/cache`. Hot packages get smart-chunked across shards using `-run "^(?:T1|T2|...)$"` regexes. Cache miss → falls back to gotesplit's count-based split.

**Tech Stack:** Go (single module — `tools/gotestsplit/` lives in shisho's main `go.mod`), `github.com/jstemmer/go-junit-report/v2` (JUnit emission, already a gotesplit transitive dep), GitHub Actions (`actions/cache@v4`).

**Spec:** `docs/superpowers/specs/2026-04-27-gotestsplit-design.md`

## File Structure

All under `tools/gotestsplit/`:

| File | Responsibility |
|------|---------------|
| `main.go` | Entry point, top-level flag parsing, subcommand dispatch |
| `pack.go` | Pure bin-packing logic — `Pack(history, packages, N) → [N][]Item` |
| `pack_test.go` | Golden tests for `Pack` |
| `junit.go` | Read JUnit XML directory → `History` (`map[pkg]map[test]float64`) |
| `junit_test.go` | Tests for JUnit reader |
| `discover.go` | `go test -list` wrapper (lifted from gotesplit) |
| `simulate.go` | `simulate` subcommand — projects per-shard time across N range |
| `plan.go` | `plan` subcommand — print shard assignment for one (N, I) |
| `run.go` | `run` subcommand — plan + exec `go test` + write JUnit |
| `LICENSE` | MIT, with attribution to Songmu (gotesplit upstream) |
| `README.md` | Brief usage/CLI reference |
| `testdata/` | Sample JUnit XMLs and integration-test fixture module |

Same-module (no separate `go.mod`) so `go run ./tools/gotestsplit` works from repo root with no module juggling. Pull-out later means moving files + `go mod init`.

---

### Task 1: Tool skeleton + LICENSE

**Files:**
- Create: `tools/gotestsplit/main.go`
- Create: `tools/gotestsplit/LICENSE`
- Create: `tools/gotestsplit/README.md`

- [ ] **Step 1: Create directory and LICENSE**

```bash
mkdir -p tools/gotestsplit
```

`tools/gotestsplit/LICENSE`:
```
MIT License

Copyright (c) 2020 Songmu (original gotesplit)
Copyright (c) 2026 Shisho Books (timing-aware fork)

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

- [ ] **Step 2: Write main.go with subcommand dispatch (no-op handlers)**

`tools/gotestsplit/main.go`:
```go
// Package main is gotestsplit, a timing-aware fork of gotesplit
// (https://github.com/Songmu/gotesplit, MIT). It splits Go tests across CI
// shards based on measured per-test duration rather than test count.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if err != flag.ErrHelp {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}

func run(ctx context.Context, argv []string, stdout, stderr io.Writer) error {
	if len(argv) == 0 {
		printUsage(stderr)
		return flag.ErrHelp
	}
	switch argv[0] {
	case "simulate":
		return cmdSimulate(ctx, argv[1:], stdout, stderr)
	case "plan":
		return cmdPlan(ctx, argv[1:], stdout, stderr)
	case "run":
		return cmdRun(ctx, argv[1:], stdout, stderr)
	case "-h", "--help", "help":
		printUsage(stdout)
		return nil
	default:
		// Bare `gotestsplit -total=N -index=I ...` is a shortcut for `run`.
		return cmdRun(ctx, argv, stdout, stderr)
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, `gotestsplit — timing-aware Go test splitter

Usage:
  gotestsplit simulate -junit-dir=<dir> [-min=N] [-max=N]
  gotestsplit plan -junit-dir=<dir> -total=N -index=I [-detail] <pkg>...
  gotestsplit run -junit-dir=<dir> -total=N -index=I <pkg>... [-- go-test-args]
  gotestsplit -total=N -index=I <pkg>... [-- go-test-args]   # shortcut for "run"`)
}

// Stubs — implemented in simulate.go, plan.go, run.go.
func cmdSimulate(ctx context.Context, argv []string, stdout, stderr io.Writer) error {
	return fmt.Errorf("simulate: not implemented")
}
func cmdPlan(ctx context.Context, argv []string, stdout, stderr io.Writer) error {
	return fmt.Errorf("plan: not implemented")
}
func cmdRun(ctx context.Context, argv []string, stdout, stderr io.Writer) error {
	return fmt.Errorf("run: not implemented")
}
```

- [ ] **Step 3: Write README.md**

`tools/gotestsplit/README.md`:
```markdown
# gotestsplit

Timing-aware Go test splitter. Forked from [gotesplit](https://github.com/Songmu/gotesplit) (MIT, Songmu).

Where gotesplit splits by test count, gotestsplit reads prior JUnit XML output
to bin-pack by measured wallclock time, and chunks hot packages across shards
using `-run` regexes.

## Usage

```
gotestsplit simulate -junit-dir=.gotest-timings [-min=2] [-max=10]
gotestsplit plan -junit-dir=.gotest-timings -total=5 -index=0 ./pkg/...
gotestsplit run -junit-dir=.gotest-timings -total=5 -index=0 ./pkg/... -- -race
```

See `docs/superpowers/specs/2026-04-27-gotestsplit-design.md` for design rationale.
```

- [ ] **Step 4: Verify it builds and prints help**

```bash
go run ./tools/gotestsplit --help
```
Expected: usage block prints, exit 0.

```bash
go run ./tools/gotestsplit simulate
```
Expected: `simulate: not implemented`, exit 1.

- [ ] **Step 5: Commit**

```bash
git add tools/gotestsplit/
git commit -m "[CI] Scaffold gotestsplit tool"
```

---

### Task 2: JUnit history reader

**Files:**
- Create: `tools/gotestsplit/testdata/sample-1.xml`
- Create: `tools/gotestsplit/testdata/sample-2.xml`
- Create: `tools/gotestsplit/junit_test.go`
- Create: `tools/gotestsplit/junit.go`

- [ ] **Step 1: Add sample JUnit fixtures**

`tools/gotestsplit/testdata/sample-1.xml`:
```xml
<?xml version="1.0" encoding="UTF-8"?>
<testsuites>
  <testsuite name="github.com/example/foo" tests="2">
    <testcase classname="foo" name="TestAlpha" time="3.500"/>
    <testcase classname="foo" name="TestBeta" time="1.200"/>
    <testcase classname="foo" name="TestBeta/subcase" time="0.500"/>
  </testsuite>
  <testsuite name="github.com/example/bar" tests="1">
    <testcase classname="bar" name="TestGamma" time="0.100"/>
  </testsuite>
</testsuites>
```

`tools/gotestsplit/testdata/sample-2.xml`:
```xml
<?xml version="1.0" encoding="UTF-8"?>
<testsuites>
  <testsuite name="github.com/example/foo" tests="1">
    <testcase classname="foo" name="TestAlpha" time="4.000"/>
  </testsuite>
</testsuites>
```

- [ ] **Step 2: Write the failing test**

`tools/gotestsplit/junit_test.go`:
```go
package main

import (
	"reflect"
	"testing"
)

func TestReadHistory_ParsesAndMergesFiles(t *testing.T) {
	t.Parallel()
	got, err := ReadHistory("testdata")
	if err != nil {
		t.Fatalf("ReadHistory: %v", err)
	}
	// sample-2 overrides sample-1 for TestAlpha (later file wins).
	// Subtests (TestBeta/subcase) are skipped — top-level only.
	want := History{
		"github.com/example/foo": {
			"TestAlpha": 4.0,
			"TestBeta":  1.2,
		},
		"github.com/example/bar": {
			"TestGamma": 0.1,
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ReadHistory mismatch:\n got=%v\nwant=%v", got, want)
	}
}

func TestReadHistory_EmptyDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	got, err := ReadHistory(dir)
	if err != nil {
		t.Fatalf("ReadHistory: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty history, got %v", got)
	}
}

func TestReadHistory_MissingDir(t *testing.T) {
	t.Parallel()
	got, err := ReadHistory("/nonexistent/path/xyz")
	if err != nil {
		t.Fatalf("ReadHistory should not error on missing dir: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty history, got %v", got)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

```bash
go test ./tools/gotestsplit -run TestReadHistory -v
```
Expected: FAIL — `History` and `ReadHistory` are undefined.

- [ ] **Step 4: Implement junit.go**

`tools/gotestsplit/junit.go`:
```go
package main

import (
	"encoding/xml"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// History maps package import path → top-level test name → wallclock seconds.
type History map[string]map[string]float64

type junitTestSuites struct {
	XMLName    xml.Name         `xml:"testsuites"`
	TestSuites []junitTestSuite `xml:"testsuite"`
}

type junitTestSuite struct {
	XMLName   xml.Name         `xml:"testsuite"`
	Name      string           `xml:"name,attr"`
	TestCases []junitTestCase  `xml:"testcase"`
}

type junitTestCase struct {
	Name string  `xml:"name,attr"`
	Time float64 `xml:"time,attr"`
}

// ReadHistory walks dir for *.xml files, parses them as JUnit XML, and returns
// a merged History. Subtests (Test/Subtest) are skipped — only top-level tests
// can be targeted with `go test -run`.
//
// Files are processed in lexical order; later files override earlier values
// for the same (pkg, test). A missing dir returns an empty History (no error).
func ReadHistory(dir string) (History, error) {
	h := History{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return h, nil
		}
		return nil, err
	}
	var paths []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".xml") {
			continue
		}
		paths = append(paths, filepath.Join(dir, e.Name()))
	}
	sort.Strings(paths)
	for _, p := range paths {
		if err := mergeFile(h, p); err != nil {
			return nil, err
		}
	}
	return h, nil
}

func mergeFile(h History, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var ts junitTestSuites
	// Tolerate bare <testsuite> roots by wrapping if needed.
	if err := xml.Unmarshal(data, &ts); err != nil {
		var single junitTestSuite
		if err2 := xml.Unmarshal(data, &single); err2 == nil {
			ts.TestSuites = []junitTestSuite{single}
		} else {
			return err
		}
	}
	for _, suite := range ts.TestSuites {
		if suite.Name == "" {
			continue
		}
		pkg := h[suite.Name]
		if pkg == nil {
			pkg = map[string]float64{}
			h[suite.Name] = pkg
		}
		for _, tc := range suite.TestCases {
			if strings.Contains(tc.Name, "/") {
				continue // subtest, skip
			}
			pkg[tc.Name] = tc.Time
		}
	}
	return nil
}
```

- [ ] **Step 5: Run test to verify it passes**

```bash
go test ./tools/gotestsplit -run TestReadHistory -v
```
Expected: PASS for all three subtests.

- [ ] **Step 6: Commit**

```bash
git add tools/gotestsplit/
git commit -m "[CI] Add JUnit history reader for gotestsplit"
```

---

### Task 3: Pack — package-level LPT

**Files:**
- Create: `tools/gotestsplit/pack.go`
- Create: `tools/gotestsplit/pack_test.go`

- [ ] **Step 1: Write the failing test**

`tools/gotestsplit/pack_test.go`:
```go
package main

import (
	"sort"
	"testing"
)

func shardLoads(shards [][]Item) []float64 {
	out := make([]float64, len(shards))
	for i, s := range shards {
		var sum float64
		for _, it := range s {
			sum += it.Duration
		}
		out[i] = sum
	}
	sort.Float64s(out)
	return out
}

func TestPack_PackageOnly_LPT(t *testing.T) {
	t.Parallel()
	// Five packages, durations 100, 50, 40, 30, 10. Total=230, ideal=115.
	// Nothing exceeds ideal*0.8=92 except the 100s package — but with K=ceil(100/92)=2
	// it'd split. To keep this test about LPT only, set ideal*0.8 high enough by
	// using packages that all fit. We'll test chunking in the next task.
	pkgs := []Package{
		{Path: "pkg/a", Tests: []string{"TA"}},
		{Path: "pkg/b", Tests: []string{"TB"}},
		{Path: "pkg/c", Tests: []string{"TC"}},
		{Path: "pkg/d", Tests: []string{"TD"}},
		{Path: "pkg/e", Tests: []string{"TE"}},
	}
	hist := History{
		"pkg/a": {"TA": 50},
		"pkg/b": {"TB": 50},
		"pkg/c": {"TC": 50},
		"pkg/d": {"TD": 50},
		"pkg/e": {"TE": 50},
	}
	shards := Pack(hist, pkgs, 5)
	if len(shards) != 5 {
		t.Fatalf("got %d shards, want 5", len(shards))
	}
	loads := shardLoads(shards)
	for _, l := range loads {
		if l != 50 {
			t.Errorf("shard load = %v, want 50; loads=%v", l, loads)
		}
	}
}

func TestPack_PackageOnly_Skewed(t *testing.T) {
	t.Parallel()
	// 100, 60, 40, 20 across 2 shards. Ideal=110. ideal*0.8=88. 100 < 88? no, 100>88
	// → would chunk. To force whole-package behavior, give each pkg only one test
	// (can't chunk a single-test package below 1 chunk).
	pkgs := []Package{
		{Path: "big", Tests: []string{"T1"}},
		{Path: "med", Tests: []string{"T2"}},
		{Path: "small", Tests: []string{"T3"}},
		{Path: "tiny", Tests: []string{"T4"}},
	}
	hist := History{
		"big":   {"T1": 100},
		"med":   {"T2": 60},
		"small": {"T3": 40},
		"tiny":  {"T4": 20},
	}
	shards := Pack(hist, pkgs, 2)
	loads := shardLoads(shards)
	// LPT places: big(100)→s0, med(60)→s1, small(40)→s1, tiny(20)→s0
	// → s0=120, s1=100. Sorted: [100, 120].
	if loads[0] != 100 || loads[1] != 120 {
		t.Errorf("loads = %v, want [100, 120]", loads)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./tools/gotestsplit -run TestPack_PackageOnly -v
```
Expected: FAIL — `Pack`, `Item`, `Package` undefined.

- [ ] **Step 3: Implement pack.go (package-level only — chunking comes in Task 4)**

`tools/gotestsplit/pack.go`:
```go
package main

import (
	"container/heap"
	"sort"
)

// Package is a discovered Go package and its top-level tests.
type Package struct {
	Path  string
	Tests []string
}

// Item is one unit of work assigned to a shard. If Tests is nil/empty, the
// whole package runs (no -run regex). Otherwise only the listed tests run.
type Item struct {
	Pkg      string
	Tests    []string // nil = whole package
	Duration float64  // estimated wallclock seconds
}

// Pack distributes packages (and chunks of hot packages) across N shards using
// LPT bin-packing. Pure: same inputs produce identical outputs. Used by
// simulate, plan, and run so they always agree.
func Pack(hist History, pkgs []Package, n int) [][]Item {
	if n <= 0 {
		return nil
	}
	items := buildItems(hist, pkgs, n)
	return lpt(items, n)
}

// buildItems turns each Package into one or more Items. For now: one whole-pkg
// Item per package, with Duration = sum of known top-level test durations.
// Smart chunking is layered on in Task 4.
func buildItems(hist History, pkgs []Package, n int) []Item {
	items := make([]Item, 0, len(pkgs))
	for _, p := range pkgs {
		var sum float64
		for _, t := range p.Tests {
			sum += hist[p.Path][t]
		}
		items = append(items, Item{Pkg: p.Path, Duration: sum})
	}
	return items
}

// lpt is Longest-Processing-Time-first greedy: sort items desc by duration,
// place each into the currently-shortest shard.
func lpt(items []Item, n int) [][]Item {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Duration != items[j].Duration {
			return items[i].Duration > items[j].Duration
		}
		return items[i].Pkg < items[j].Pkg // tie-break for determinism
	})
	shards := make([][]Item, n)
	h := &shardHeap{}
	for i := 0; i < n; i++ {
		heap.Push(h, &shardEntry{idx: i})
	}
	for _, it := range items {
		top := heap.Pop(h).(*shardEntry)
		top.load += it.Duration
		shards[top.idx] = append(shards[top.idx], it)
		heap.Push(h, top)
	}
	return shards
}

type shardEntry struct {
	idx  int
	load float64
}

type shardHeap []*shardEntry

func (h shardHeap) Len() int { return len(h) }
func (h shardHeap) Less(i, j int) bool {
	if h[i].load != h[j].load {
		return h[i].load < h[j].load
	}
	return h[i].idx < h[j].idx // tie-break for determinism
}
func (h shardHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *shardHeap) Push(x interface{}) { *h = append(*h, x.(*shardEntry)) }
func (h *shardHeap) Pop() interface{} {
	old := *h
	x := old[len(old)-1]
	*h = old[:len(old)-1]
	return x
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./tools/gotestsplit -run TestPack_PackageOnly -v
```
Expected: PASS for both subtests.

- [ ] **Step 5: Commit**

```bash
git add tools/gotestsplit/
git commit -m "[CI] Add LPT bin-packer for gotestsplit"
```

---

### Task 4: Pack — smart chunking for hot packages

**Files:**
- Modify: `tools/gotestsplit/pack.go` (extend `buildItems`)
- Modify: `tools/gotestsplit/pack_test.go` (add chunking tests)

- [ ] **Step 1: Write the failing tests**

Append to `tools/gotestsplit/pack_test.go`:
```go
func TestPack_SmartChunking_HotPackage(t *testing.T) {
	t.Parallel()
	// One hot package: 10 tests × 50s = 500s wallclock.
	// Plus 4 small packages of 50s each. Total = 700, ideal = 140, threshold = 112.
	// Hot pkg 500 > 112 → K = ceil(500/112) = 5 chunks of 100s each.
	// 5 chunks of 100 + 4 small of 50, into 5 shards via LPT:
	//   each shard gets one 100-chunk (=500), some get a 50.
	// Expected slowest = 150s, fastest = 100s.
	hot := Package{Path: "hot", Tests: nil}
	hist := History{"hot": {}}
	for i := 0; i < 10; i++ {
		name := "TestHot" + string(rune('A'+i))
		hot.Tests = append(hot.Tests, name)
		hist["hot"][name] = 50
	}
	pkgs := []Package{hot}
	for i := 0; i < 4; i++ {
		name := "small" + string(rune('A'+i))
		pkgs = append(pkgs, Package{Path: name, Tests: []string{"T"}})
		hist[name] = map[string]float64{"T": 50}
	}
	shards := Pack(hist, pkgs, 5)
	loads := shardLoads(shards)
	// 5 shards, total=700, fairness check: max ≤ 1.2 * ideal
	if loads[len(loads)-1] > 168 { // 1.2 * 140
		t.Errorf("max shard load %v exceeds 1.2x ideal (168); loads=%v", loads[len(loads)-1], loads)
	}
	// Confirm hot pkg actually got chunked: count items mentioning hot
	hotItems := 0
	for _, s := range shards {
		for _, it := range s {
			if it.Pkg == "hot" {
				hotItems++
				if len(it.Tests) == 0 {
					t.Errorf("hot pkg item has no Tests — should be chunked, got whole-pkg item")
				}
			}
		}
	}
	if hotItems < 2 {
		t.Errorf("hot pkg should be chunked into ≥2 items, got %d", hotItems)
	}
}

func TestPack_SmartChunking_TestsAssignedDeterministically(t *testing.T) {
	t.Parallel()
	hot := Package{Path: "hot", Tests: []string{"TA", "TB", "TC", "TD"}}
	hist := History{"hot": {"TA": 40, "TB": 30, "TC": 20, "TD": 10}}
	pkgs := []Package{hot, {Path: "x", Tests: []string{"T"}}}
	hist["x"] = map[string]float64{"T": 10}

	a := Pack(hist, pkgs, 2)
	b := Pack(hist, pkgs, 2)
	// Same input → identical assignment ordering.
	if len(a) != len(b) {
		t.Fatalf("nondeterministic: %d vs %d shards", len(a), len(b))
	}
	for i := range a {
		if len(a[i]) != len(b[i]) {
			t.Errorf("shard %d sizes differ: %d vs %d", i, len(a[i]), len(b[i]))
		}
		for j := range a[i] {
			if a[i][j].Pkg != b[i][j].Pkg {
				t.Errorf("shard %d item %d: pkg differs", i, j)
			}
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./tools/gotestsplit -run TestPack_SmartChunking -v
```
Expected: FAIL — `TestPack_SmartChunking_HotPackage` because hot pkg won't be chunked.

- [ ] **Step 3: Replace buildItems in pack.go with chunking-aware version**

In `tools/gotestsplit/pack.go`, replace the existing `buildItems` with:

```go
import "math"

func buildItems(hist History, pkgs []Package, n int) []Item {
	// First pass: compute each package's known wallclock and the global total.
	pkgWall := make(map[string]float64, len(pkgs))
	var total float64
	for _, p := range pkgs {
		var sum float64
		for _, t := range p.Tests {
			sum += hist[p.Path][t]
		}
		pkgWall[p.Path] = sum
		total += sum
	}
	if total == 0 || n <= 0 {
		// No history to drive chunking. Return one whole-pkg Item each.
		out := make([]Item, 0, len(pkgs))
		for _, p := range pkgs {
			out = append(out, Item{Pkg: p.Path})
		}
		return out
	}
	ideal := total / float64(n)
	threshold := ideal * 0.8

	items := make([]Item, 0, len(pkgs))
	for _, p := range pkgs {
		w := pkgWall[p.Path]
		if w <= threshold || len(p.Tests) <= 1 {
			items = append(items, Item{Pkg: p.Path, Duration: w})
			continue
		}
		k := int(math.Ceil(w / threshold))
		if k > len(p.Tests) {
			k = len(p.Tests)
		}
		items = append(items, chunkPackage(p, hist[p.Path], k)...)
	}
	return items
}

// chunkPackage assigns p.Tests to k chunks via LPT, returning k Items, each
// with non-nil Tests targeted by `go test -run "^(?:T1|T2|...)$"`.
func chunkPackage(p Package, durations map[string]float64, k int) []Item {
	type tt struct {
		name     string
		duration float64
	}
	tests := make([]tt, len(p.Tests))
	for i, name := range p.Tests {
		tests[i] = tt{name, durations[name]}
	}
	sort.Slice(tests, func(i, j int) bool {
		if tests[i].duration != tests[j].duration {
			return tests[i].duration > tests[j].duration
		}
		return tests[i].name < tests[j].name
	})

	chunks := make([]Item, k)
	loads := make([]float64, k)
	for i := range chunks {
		chunks[i] = Item{Pkg: p.Path}
	}
	for _, t := range tests {
		// Find shortest chunk (small k → linear scan is fine).
		min := 0
		for i := 1; i < k; i++ {
			if loads[i] < loads[min] {
				min = i
			}
		}
		chunks[min].Tests = append(chunks[min].Tests, t.name)
		chunks[min].Duration += t.duration
		loads[min] += t.duration
	}
	return chunks
}
```

- [ ] **Step 4: Run all pack tests**

```bash
go test ./tools/gotestsplit -run TestPack -v
```
Expected: PASS for all subtests (including earlier package-only tests).

- [ ] **Step 5: Commit**

```bash
git add tools/gotestsplit/
git commit -m "[CI] Add smart chunking of hot packages in gotestsplit"
```

---

### Task 5: Pack — unknown test handling (median fallback)

**Files:**
- Modify: `tools/gotestsplit/pack.go`
- Modify: `tools/gotestsplit/pack_test.go`

- [ ] **Step 1: Write the failing test**

Append to `tools/gotestsplit/pack_test.go`:
```go
func TestPack_UnknownTest_UsesPackageMedian(t *testing.T) {
	t.Parallel()
	// Package has 3 tests with history (10, 20, 30 → median 20) and 1 unknown.
	// The unknown should be estimated at 20s.
	pkgs := []Package{{Path: "p", Tests: []string{"TA", "TB", "TC", "TUnknown"}}}
	hist := History{"p": {"TA": 10, "TB": 20, "TC": 30}}
	shards := Pack(hist, pkgs, 1)
	var sum float64
	for _, it := range shards[0] {
		sum += it.Duration
	}
	want := 10.0 + 20 + 30 + 20 // 80
	if sum != want {
		t.Errorf("got total %v, want %v", sum, want)
	}
}

func TestPack_UnknownPackage_Uses30sPerTest(t *testing.T) {
	t.Parallel()
	// Package has 4 tests, no history at all → 4 * 30 = 120.
	pkgs := []Package{{Path: "p", Tests: []string{"T1", "T2", "T3", "T4"}}}
	hist := History{} // empty
	shards := Pack(hist, pkgs, 1)
	var sum float64
	for _, it := range shards[0] {
		sum += it.Duration
	}
	if sum != 120 {
		t.Errorf("got total %v, want 120", sum)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./tools/gotestsplit -run TestPack_Unknown -v
```
Expected: FAIL — both tests, since unknown tests currently contribute 0.

- [ ] **Step 3: Add estimateDuration helper and use it everywhere**

In `tools/gotestsplit/pack.go`, add (above `buildItems`):

```go
const unknownPackageFallbackPerTest = 30.0

// estimateDuration returns the best guess for one test's wallclock:
//   - exact JUnit value if known
//   - else the median of other known tests in this package
//   - else unknownPackageFallbackPerTest seconds
func estimateDuration(hist History, pkg, test string) float64 {
	if d, ok := hist[pkg][test]; ok {
		return d
	}
	pkgHist := hist[pkg]
	if len(pkgHist) > 0 {
		ds := make([]float64, 0, len(pkgHist))
		for _, d := range pkgHist {
			ds = append(ds, d)
		}
		sort.Float64s(ds)
		return ds[len(ds)/2] // median (lower of two for even count — fine)
	}
	return unknownPackageFallbackPerTest
}
```

Then replace **both** places that read `hist[p.Path][t]` directly (in `buildItems` and `chunkPackage`) with `estimateDuration(hist, p.Path, t)` / `estimateDuration(hist, p.Path, name)`.

For `chunkPackage`, change its signature to accept the full `History`:
```go
func chunkPackage(p Package, hist History, k int) []Item {
```
and update its body to call `estimateDuration(hist, p.Path, name)`. Update the call site in `buildItems` to pass `hist` instead of `hist[p.Path]`.

- [ ] **Step 4: Run all pack tests**

```bash
go test ./tools/gotestsplit -run TestPack -v
```
Expected: PASS for all subtests.

- [ ] **Step 5: Commit**

```bash
git add tools/gotestsplit/
git commit -m "[CI] Estimate unknown test durations via package median"
```

---

### Task 6: Pack — count-based fallback when no history exists

**Files:**
- Modify: `tools/gotestsplit/pack.go`
- Modify: `tools/gotestsplit/pack_test.go`

- [ ] **Step 1: Write the failing test**

Append to `tools/gotestsplit/pack_test.go`:
```go
func TestPack_NoHistoryAtAll_FallsBackToCountBased(t *testing.T) {
	t.Parallel()
	// 6 tests across 2 packages, no history → equal-count split into 3 shards = 2 each.
	pkgs := []Package{
		{Path: "a", Tests: []string{"T1", "T2", "T3"}},
		{Path: "b", Tests: []string{"T4", "T5", "T6"}},
	}
	shards := Pack(History{}, pkgs, 3)
	if len(shards) != 3 {
		t.Fatalf("got %d shards, want 3", len(shards))
	}
	totalTests := 0
	for _, s := range shards {
		for _, it := range s {
			if len(it.Tests) > 0 {
				totalTests += len(it.Tests)
			} else {
				// Count discovery for whole-pkg items by looking up.
				for _, p := range pkgs {
					if p.Path == it.Pkg {
						totalTests += len(p.Tests)
					}
				}
			}
		}
	}
	if totalTests != 6 {
		t.Errorf("count fallback dropped tests: got %d, want 6", totalTests)
	}
}
```

- [ ] **Step 2: Run test to verify it passes or fails**

```bash
go test ./tools/gotestsplit -run TestPack_NoHistoryAtAll -v
```

If it already passes (because the median fallback assigns 30s/test uniformly and LPT produces a reasonable split), commit the test as documentation and skip Step 3. If it fails (e.g. all tests pile onto one shard because durations are all equal and tie-breaking sorts them together), continue.

- [ ] **Step 3: Only if Step 2 failed, add explicit count-based path**

In `tools/gotestsplit/pack.go`, at the top of `Pack`, before calling `buildItems`:

```go
// If we have zero historical data anywhere, fall back to count-based splitting
// (gotesplit's classic behavior). This is the first-run / fresh-cache case.
allEmpty := true
for _, m := range hist {
	if len(m) > 0 {
		allEmpty = false
		break
	}
}
if allEmpty {
	return countBasedSplit(pkgs, n)
}
```

Add `countBasedSplit`:
```go
func countBasedSplit(pkgs []Package, n int) [][]Item {
	// Flatten (pkg, test) pairs, sort, split into N equal-count chunks (matches
	// gotesplit's algorithm). Then re-group consecutive same-pkg tests into one
	// Item per (shard, pkg).
	type pair struct{ pkg, test string }
	var all []pair
	for _, p := range pkgs {
		tests := append([]string(nil), p.Tests...)
		sort.Strings(tests)
		for _, t := range tests {
			all = append(all, pair{p.Path, t})
		}
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].pkg != all[j].pkg {
			return all[i].pkg < all[j].pkg
		}
		return all[i].test < all[j].test
	})

	shards := make([][]Item, n)
	per := len(all) / n
	mod := len(all) % n
	offset := func(i int) int {
		o := per * i
		if i < mod {
			o += i
		} else {
			o += mod
		}
		return o
	}
	for i := 0; i < n; i++ {
		slice := all[offset(i):offset(i+1)]
		// Group consecutive same-pkg tests.
		var cur Item
		for _, pr := range slice {
			if cur.Pkg != pr.pkg {
				if cur.Pkg != "" {
					shards[i] = append(shards[i], cur)
				}
				cur = Item{Pkg: pr.pkg}
			}
			cur.Tests = append(cur.Tests, pr.test)
		}
		if cur.Pkg != "" {
			shards[i] = append(shards[i], cur)
		}
	}
	return shards
}
```

- [ ] **Step 4: Run all tests**

```bash
go test ./tools/gotestsplit -v
```
Expected: PASS for all tests.

- [ ] **Step 5: Commit**

```bash
git add tools/gotestsplit/
git commit -m "[CI] Add count-based fallback when no JUnit history exists"
```

---

### Task 7: simulate subcommand

**Files:**
- Create: `tools/gotestsplit/simulate.go`
- Create: `tools/gotestsplit/simulate_test.go`
- Modify: `tools/gotestsplit/main.go` (delete the stub)

- [ ] **Step 1: Write the failing test**

`tools/gotestsplit/simulate_test.go`:
```go
package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestSimulate_PrintsTableAcrossRange(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	err := cmdSimulate(context.Background(),
		[]string{"-junit-dir=testdata", "-min=2", "-max=3"},
		&stdout, &stderr)
	if err != nil {
		t.Fatalf("cmdSimulate: %v\nstderr: %s", err, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"N=2", "N=3", "slowest=", "cost="} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\nfull output:\n%s", want, out)
		}
	}
}

func TestSimulate_NoHistory_ErrorsClearly(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	err := cmdSimulate(context.Background(),
		[]string{"-junit-dir=" + t.TempDir()},
		&stdout, &stderr)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no JUnit") {
		t.Errorf("error should mention missing JUnit data, got: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./tools/gotestsplit -run TestSimulate -v
```
Expected: FAIL — `cmdSimulate` returns "not implemented".

- [ ] **Step 3: Implement simulate.go**

`tools/gotestsplit/simulate.go`:
```go
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"sort"
)

func cmdSimulate(ctx context.Context, argv []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("simulate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	junitDir := fs.String("junit-dir", "", "directory of JUnit XML files from prior runs (required)")
	min := fs.Int("min", 2, "minimum N to project")
	max := fs.Int("max", 10, "maximum N to project")
	if err := fs.Parse(argv); err != nil {
		return err
	}
	if *junitDir == "" {
		return fmt.Errorf("-junit-dir is required")
	}
	if *min < 1 || *max < *min {
		return fmt.Errorf("invalid range: min=%d max=%d", *min, *max)
	}
	hist, err := ReadHistory(*junitDir)
	if err != nil {
		return err
	}
	if len(hist) == 0 {
		return fmt.Errorf("no JUnit data found in %s", *junitDir)
	}
	pkgs := packagesFromHistory(hist)
	fmt.Fprintf(stdout, "%-5s  %-13s  %s\n", "N", "slowest", "cost")
	for n := *min; n <= *max; n++ {
		shards := Pack(hist, pkgs, n)
		var slowest float64
		for _, s := range shards {
			var load float64
			for _, it := range s {
				load += it.Duration
			}
			if load > slowest {
				slowest = load
			}
		}
		cost := slowest * float64(n)
		fmt.Fprintf(stdout, "N=%-3d  slowest=%5.1fm  cost=%5.1f min\n",
			n, slowest/60, cost/60)
	}
	return nil
}

// packagesFromHistory builds Package list using only what's in JUnit history
// (simulate does not invoke `go test -list`).
func packagesFromHistory(hist History) []Package {
	pkgs := make([]Package, 0, len(hist))
	for path, tests := range hist {
		p := Package{Path: path}
		for name := range tests {
			p.Tests = append(p.Tests, name)
		}
		sort.Strings(p.Tests)
		pkgs = append(pkgs, p)
	}
	sort.Slice(pkgs, func(i, j int) bool { return pkgs[i].Path < pkgs[j].Path })
	return pkgs
}
```

In `tools/gotestsplit/main.go`, **delete** the `cmdSimulate` stub (the real one is now in `simulate.go`).

- [ ] **Step 4: Run all tests**

```bash
go test ./tools/gotestsplit -v
```
Expected: PASS.

- [ ] **Step 5: Manual smoke test**

```bash
go run ./tools/gotestsplit simulate -junit-dir=tools/gotestsplit/testdata
```
Expected: a few rows of `N=… slowest=… cost=…`.

- [ ] **Step 6: Commit**

```bash
git add tools/gotestsplit/
git commit -m "[CI] Add simulate subcommand to gotestsplit"
```

---

### Task 8: discover.go — `go test -list` wrapper

**Files:**
- Create: `tools/gotestsplit/discover.go`

- [ ] **Step 1: Implement discover.go**

Lifted in spirit from gotesplit's `getTestListsFromPkgs` / `detectTags` / `detectRace`. We don't need a unit test here — it shells out, and it'll be exercised by the integration test in Task 11.

`tools/gotestsplit/discover.go`:
```go
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
```

- [ ] **Step 2: Verify it builds**

```bash
go build ./tools/gotestsplit
```
Expected: builds clean (no test required at this stage).

- [ ] **Step 3: Commit**

```bash
git add tools/gotestsplit/
git commit -m "[CI] Add go test -list discovery wrapper"
```

---

### Task 9: plan subcommand

**Files:**
- Create: `tools/gotestsplit/plan.go`
- Create: `tools/gotestsplit/plan_test.go`
- Modify: `tools/gotestsplit/main.go` (delete the `cmdPlan` stub)

- [ ] **Step 1: Write the failing test**

`tools/gotestsplit/plan_test.go`:
```go
package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestPlan_PrintsAssignmentForShard(t *testing.T) {
	t.Parallel()
	// Use packagesFromHistory under the hood by passing -no-discover so we don't
	// shell out to `go test -list` in unit tests.
	var stdout, stderr bytes.Buffer
	err := cmdPlan(context.Background(),
		[]string{
			"-junit-dir=testdata",
			"-total=2", "-index=0",
			"-no-discover",
		},
		&stdout, &stderr)
	if err != nil {
		t.Fatalf("cmdPlan: %v\nstderr: %s", err, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "shard 1/2") {
		t.Errorf("missing 'shard 1/2' header; got:\n%s", out)
	}
}

func TestPlan_DetailMode_ShowsAllShards(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	err := cmdPlan(context.Background(),
		[]string{
			"-junit-dir=testdata",
			"-total=2", "-index=0",
			"-detail", "-no-discover",
		},
		&stdout, &stderr)
	if err != nil {
		t.Fatalf("cmdPlan: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "shard 1/2") || !strings.Contains(out, "shard 2/2") {
		t.Errorf("detail mode should show both shards; got:\n%s", out)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./tools/gotestsplit -run TestPlan -v
```
Expected: FAIL — `cmdPlan` returns "not implemented".

- [ ] **Step 3: Implement plan.go**

`tools/gotestsplit/plan.go`:
```go
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

func cmdPlan(ctx context.Context, argv []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("plan", flag.ContinueOnError)
	fs.SetOutput(stderr)
	junitDir := fs.String("junit-dir", "", "directory of JUnit XML files (required)")
	total := fs.Int("total", 0, "total number of shards (required)")
	index := fs.Int("index", 0, "zero-based shard index")
	detail := fs.Bool("detail", false, "show all shards' assignments, not just -index")
	noDiscover := fs.Bool("no-discover", false, "(test-only) build packages from JUnit history instead of running `go test -list`")
	if err := fs.Parse(argv); err != nil {
		return err
	}
	if *junitDir == "" || *total <= 0 || *index < 0 || *index >= *total {
		return fmt.Errorf("plan requires -junit-dir, -total>0, 0<=-index<-total")
	}
	pkgPatterns := fs.Args()

	hist, err := ReadHistory(*junitDir)
	if err != nil {
		return err
	}
	var pkgs []Package
	if *noDiscover {
		pkgs = packagesFromHistory(hist)
	} else {
		if len(pkgPatterns) == 0 {
			return fmt.Errorf("plan requires at least one package pattern")
		}
		pkgs, err = DiscoverPackages(ctx, pkgPatterns, nil)
		if err != nil {
			return err
		}
	}
	shards := Pack(hist, pkgs, *total)
	if *detail {
		for i, s := range shards {
			printShard(stdout, i, *total, s)
		}
		return nil
	}
	printShard(stdout, *index, *total, shards[*index])
	return nil
}

func printShard(w io.Writer, i, total int, items []Item) {
	var load float64
	for _, it := range items {
		load += it.Duration
	}
	fmt.Fprintf(w, "shard %d/%d  load=%.1fs (%.1fm)  items=%d\n",
		i+1, total, load, load/60, len(items))
	for _, it := range items {
		if len(it.Tests) == 0 {
			fmt.Fprintf(w, "  %-60s  whole pkg  (%.1fs)\n", it.Pkg, it.Duration)
		} else {
			fmt.Fprintf(w, "  %-60s  %d tests   (%.1fs)\n", it.Pkg, len(it.Tests), it.Duration)
		}
	}
}
```

In `tools/gotestsplit/main.go`, **delete** the `cmdPlan` stub.

- [ ] **Step 4: Run all tests**

```bash
go test ./tools/gotestsplit -v
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add tools/gotestsplit/
git commit -m "[CI] Add plan subcommand to gotestsplit"
```

---

### Task 10: run subcommand (exec + JUnit emit)

**Files:**
- Create: `tools/gotestsplit/run.go`
- Modify: `tools/gotestsplit/main.go` (delete the `cmdRun` stub)
- Modify: `go.mod`, `go.sum` (will pick up `go-junit-report/v2`)

- [ ] **Step 1: Implement run.go**

`tools/gotestsplit/run.go` — combines a per-shard plan with `go test` execution. Mirrors gotesplit's `goTest` and JUnit writer (MIT, attribution in `LICENSE`):

```go
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
		return fmt.Errorf("run requires -junit-dir, -total>0, 0<=-index<-total")
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
		return fmt.Errorf("run requires at least one package pattern")
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
	runOne := func(extraArgs []string, displayLabel string) error {
		seq++
		args := append([]string{"test"}, testOpts...)
		args = append(args, extraArgs...)
		fmt.Fprintf(stderr, "gotestsplit: shard %d/%d step %d: go %s\n",
			*index+1, *total, seq, strings.Join(args, " "))
		report, runErr := goTest(ctx, args, stdout, stderr)
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
		if err := runOne(wholePkgs, "whole-pkg group"); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	for _, it := range chunkItems {
		runRegex := "^(?:" + strings.Join(it.Tests, "|") + ")$"
		if err := runOne([]string{"-run", runRegex, it.Pkg}, it.Pkg); err != nil && firstErr == nil {
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
```

In `tools/gotestsplit/main.go`, **delete** the `cmdRun` stub.

- [ ] **Step 2: Pull in the new dep**

```bash
go mod tidy
```
Expected: `go.mod` and `go.sum` gain `github.com/jstemmer/go-junit-report/v2`.

- [ ] **Step 3: Build verification**

```bash
go build ./tools/gotestsplit
```
Expected: builds clean.

- [ ] **Step 4: Run existing unit tests to confirm nothing regressed**

```bash
go test ./tools/gotestsplit -v
```
Expected: all pre-existing tests still PASS.

- [ ] **Step 5: Commit**

```bash
git add tools/gotestsplit/ go.mod go.sum
git commit -m "[CI] Add run subcommand: exec go test + emit JUnit per shard"
```

> **Coverprofile note:** gotesplit also supports `-coverprofile` flag rewriting and merging multiple coverprofiles into one. Shisho's CI does not currently use `-coverprofile` (the test job runs `... -- -race` only), so this code path is intentionally **omitted** from the fork. If a future workflow needs it, port `mergeCoverprofiles` and the `-coverprofile-dir` flag from `/tmp/gotesplit/run.go` (lines 117–135 and 199–222 of upstream).

---

### Task 11: Integration test against a fixture module

**Files:**
- Create: `tools/gotestsplit/testdata/fixturemod/go.mod`
- Create: `tools/gotestsplit/testdata/fixturemod/a_test.go`
- Create: `tools/gotestsplit/testdata/fixturemod/b_test.go`
- Create: `tools/gotestsplit/integration_test.go`

- [ ] **Step 1: Build the fixture module**

`tools/gotestsplit/testdata/fixturemod/go.mod`:
```
module example.com/fixturemod

go 1.22
```

`tools/gotestsplit/testdata/fixturemod/a_test.go`:
```go
package fixturemod

import (
	"testing"
	"time"
)

func TestSlow(t *testing.T)   { time.Sleep(150 * time.Millisecond) }
func TestMedium(t *testing.T) { time.Sleep(80 * time.Millisecond) }
func TestFast(t *testing.T)   { time.Sleep(20 * time.Millisecond) }
```

`tools/gotestsplit/testdata/fixturemod/b_test.go`:
```go
package fixturemod

import (
	"testing"
	"time"
)

func TestExtra(t *testing.T) { time.Sleep(40 * time.Millisecond) }
```

- [ ] **Step 2: Write the integration test**

`tools/gotestsplit/integration_test.go`:
```go
package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRun_ProducesJUnit_AndShardsAreRunnable verifies end-to-end:
//   1. `run` succeeds against a fresh fixture module with no history → uses count fallback.
//   2. JUnit XML is written into the cache dir.
//   3. A second `run` reads that history without error.
func TestRun_ProducesJUnit_AndShardsAreRunnable(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	// Cannot t.Parallel() — uses t.Chdir.

	cacheDir := t.TempDir()
	fixture, err := filepath.Abs("testdata/fixturemod")
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(fixture) // Go 1.24+, repo go.mod is 1.25.

	for shardIdx := 0; shardIdx < 2; shardIdx++ {
		var stdout, stderr bytes.Buffer
		err := cmdRun(context.Background(), []string{
			"-junit-dir=" + cacheDir,
			"-total=2", "-index=" + strconv.Itoa(shardIdx),
			"./...",
		}, &stdout, &stderr)
		if err != nil {
			t.Fatalf("first pass shard %d: %v\nstderr:\n%s", shardIdx, err, stderr.String())
		}
		if !strings.Contains(stdout.String(), "PASS") {
			t.Errorf("shard %d stdout missing PASS:\n%s", shardIdx, stdout.String())
		}
	}

	// JUnit files should exist now.
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	junitCount := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".xml") {
			junitCount++
		}
	}
	if junitCount == 0 {
		t.Fatalf("no JUnit files written to %s", cacheDir)
	}

	// Second pass uses the history.
	var stdout, stderr bytes.Buffer
	err = cmdRun(context.Background(), []string{
		"-junit-dir=" + cacheDir,
		"-total=2", "-index=0",
		"./...",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("second pass: %v\nstderr:\n%s", err, stderr.String())
	}
}
```

(Remember to add `"strconv"` to the import block.)

- [ ] **Step 3: Run the integration test**

```bash
go test ./tools/gotestsplit -run TestRun_ProducesJUnit -v
```
Expected: PASS. The fixture's tests take <500ms total, so the test runs in a few seconds.

- [ ] **Step 4: Commit**

```bash
git add tools/gotestsplit/
git commit -m "[CI] Add integration test for gotestsplit run command"
```

---

### Task 12: Wire into CI workflow

**Files:**
- Modify: `.github/workflows/ci.yml` (the `test` job)

- [ ] **Step 1: Replace the gotesplit install + invocation**

In `.github/workflows/ci.yml`, modify the `test` job (currently lines 23–41 per the spec). Replace:

```yaml
      - name: Install gotesplit
        run: curl -sfL https://raw.githubusercontent.com/Songmu/gotesplit/main/install.sh | sh -s -- -b /usr/local/bin v0.4.0
      - name: Run tests
        env:
          SHARD: ${{ matrix.shard }}
        run: gotesplit -total=5 -index=$((SHARD - 1)) ./pkg/... -- -race
```

with:

```yaml
      - name: Restore test timing cache
        uses: actions/cache@v4
        with:
          path: .gotest-timings
          key: gotest-timings-${{ github.ref }}-${{ github.sha }}-${{ matrix.shard }}
          restore-keys: |
            gotest-timings-${{ github.ref }}-
            gotest-timings-refs/heads/master-
      - name: Run tests
        env:
          SHARD: ${{ matrix.shard }}
        run: |
          go run ./tools/gotestsplit run \
            -junit-dir=.gotest-timings \
            -total=5 -index=$((SHARD - 1)) \
            ./pkg/... -- -race
```

The cache `key` includes `matrix.shard` so each shard saves its own JUnit files independently (otherwise five concurrent saves would race and only one would win). `restore-keys` is shard-agnostic so any shard's previous output is usable as history.

- [ ] **Step 2: Push and observe**

Push the branch. The first CI run will:
1. Cache miss for both keys (new branch, never run with this tool).
2. `Pack` finds empty history → falls back to count-based split → behavior identical to today.
3. JUnit XMLs written to `.gotest-timings/` and saved into per-shard caches.
4. The next run on the same branch (or any PR off master after master next merges) will hit the cache and use timing-aware splitting.

Watch the test job logs to confirm `gotestsplit:` log lines appear and JUnit files are saved (the cache action prints sizes).

- [ ] **Step 3: Commit and push**

```bash
git add .github/workflows/ci.yml
git commit -m "[CI] Use timing-aware gotestsplit for Go test sharding"
git push
```

---

### Task 13: Choose new shard count after one CI run completes

**Files:**
- Modify: `.github/workflows/ci.yml` (matrix only — no other code changes)

This task happens *after* CI has run at least once on master with the new tool, so timing data exists.

- [ ] **Step 1: Pull master's JUnit cache locally**

Easiest path: download the cache for the most recent successful master run.

```bash
gh run list --branch master --workflow CI --limit 5
# Pick a green one, then:
gh run download <RUN_ID> -n <cache-name-or-shard-output>
```

Alternatively, just run `mise test:race` once locally with `-junit-dir` flag set on the new tool — but that won't be representative of GHA hardware speed.

- [ ] **Step 2: Run simulate**

```bash
go run ./tools/gotestsplit simulate -junit-dir=.gotest-timings -min=2 -max=10
```

Expected output: a table where `slowest` decreases as N increases up to a knee, then flatlines. Pick the smallest N at the knee.

- [ ] **Step 3: Update the matrix**

In `.github/workflows/ci.yml`:
```yaml
    strategy:
      fail-fast: false
      matrix:
        shard: [1, 2, 3, 4, 5]   # ← change this
```
Update the list to the chosen N. Update the `-total=5` argument in the run command to match. Update the job name `Go Test (${{ matrix.shard }}/5)` so the displayed denominator stays accurate.

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "[CI] Tune Go test shard count to N based on timing simulation"
```

---

## Verification checklist

After all tasks:

- [ ] `mise check:quiet` passes locally.
- [ ] `go run ./tools/gotestsplit simulate -junit-dir=...` against real timing data prints a sensible projection.
- [ ] CI's slowest test shard time is meaningfully below the previous ~10 minute baseline.
- [ ] No code in `tools/gotestsplit/` imports anything under `github.com/shishobooks/shisho/pkg/` (so the tool stays trivially extractable).
