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
