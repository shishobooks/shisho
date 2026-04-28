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
	items := buildItems(hist, pkgs, n)
	return lpt(items, n)
}

// buildItems turns each Package into one or more Items. Hot packages (wallclock
// > ideal*0.8) are split into K chunks via test-level LPT so that no single
// item dominates a shard. Packages with ≤1 test are always kept whole.
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

// chunkPackage assigns p.Tests to k chunks via LPT, returning k Items each
// with a non-nil Tests slice for targeted `go test -run` execution.
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
