package main

import (
	"container/heap"
	"math"
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
	items := buildItems(hist, pkgs, n)
	return lpt(items, n)
}

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
		return ds[len(ds)/2] // median (upper of two for even count — fine)
	}
	return unknownPackageFallbackPerTest
}

// buildItems turns each Package into one or more Items. Hot packages (wallclock
// > ideal*0.8) are split into K chunks via test-level LPT so that no single
// item dominates a shard. Packages with ≤1 test are always kept whole.
func buildItems(hist History, pkgs []Package, n int) []Item {
	// First pass: compute each package's estimated wallclock and the global total.
	pkgWall := make(map[string]float64, len(pkgs))
	var total float64
	for _, p := range pkgs {
		var sum float64
		for _, t := range p.Tests {
			sum += estimateDuration(hist, p.Path, t)
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
		items = append(items, chunkPackage(p, hist, k)...)
	}
	return items
}

// chunkPackage assigns p.Tests to k chunks via LPT, returning k Items each
// with a non-nil Tests slice for targeted `go test -run` execution.
func chunkPackage(p Package, hist History, k int) []Item {
	type tt struct {
		name     string
		duration float64
	}
	tests := make([]tt, len(p.Tests))
	for i, name := range p.Tests {
		tests[i] = tt{name, estimateDuration(hist, p.Path, name)}
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
		minIdx := 0
		for i := 1; i < k; i++ {
			if loads[i] < loads[minIdx] {
				minIdx = i
			}
		}
		chunks[minIdx].Tests = append(chunks[minIdx].Tests, t.name)
		chunks[minIdx].Duration += t.duration
		loads[minIdx] += t.duration
	}
	return chunks
}

// lpt is Longest-Processing-Time-first greedy: sort items desc by duration,
// place each into the currently-shortest shard.
func lpt(items []Item, n int) [][]Item {
	sort.SliceStable(items, func(i, j int) bool {
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

// countBasedSplit divides tests into N equal-count shards (gotesplit's classic
// algorithm). Tests are sorted by (pkg, name) for determinism, then split into
// N consecutive chunks. Consecutive same-pkg tests within a chunk are grouped
// into one Item. If an Item covers all tests in a package, Tests is set to nil
// so run.go batches it as a whole-package invocation instead of a -run regex.
// Used when hist is entirely empty (first run / fresh cache).
func countBasedSplit(pkgs []Package, n int) [][]Item {
	// Build total-test-count map so we can detect whole-package items below.
	pkgTestCount := make(map[string]int, len(pkgs))
	for _, p := range pkgs {
		pkgTestCount[p.Path] = len(p.Tests)
	}

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
		// Group consecutive same-pkg tests into one Item per (shard, pkg).
		var cur Item
		for _, pr := range slice {
			if cur.Pkg != pr.pkg {
				if cur.Pkg != "" {
					// Emit whole-pkg item when this shard owns every test in the pkg.
					if len(cur.Tests) == pkgTestCount[cur.Pkg] {
						cur.Tests = nil
					}
					shards[i] = append(shards[i], cur)
				}
				cur = Item{Pkg: pr.pkg}
			}
			cur.Tests = append(cur.Tests, pr.test)
		}
		if cur.Pkg != "" {
			// Emit whole-pkg item when this shard owns every test in the pkg.
			if len(cur.Tests) == pkgTestCount[cur.Pkg] {
				cur.Tests = nil
			}
			shards[i] = append(shards[i], cur)
		}
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
func (h shardHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *shardHeap) Push(x any)   { *h = append(*h, x.(*shardEntry)) }
func (h *shardHeap) Pop() any {
	old := *h
	x := old[len(old)-1]
	*h = old[:len(old)-1]
	return x
}
