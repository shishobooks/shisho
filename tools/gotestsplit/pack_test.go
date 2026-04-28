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
