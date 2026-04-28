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
	// Five packages, all 50s, into 5 shards → all loads = 50s.
	// Tests LPT only (no chunking): total=250, ideal=50, threshold=40; all pkgs fit.
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
	// Package "p" has no history but another package does. When history is
	// non-empty overall (allEmpty=false), estimateDuration falls back to
	// unknownPackageFallbackPerTest (30s) per test → 4 * 30 = 120.
	pkgs := []Package{
		{Path: "p", Tests: []string{"T1", "T2", "T3", "T4"}},
		{Path: "known", Tests: []string{"TK"}},
	}
	hist := History{"known": {"TK": 10}} // non-empty history, but "p" is absent
	shards := Pack(hist, pkgs, 1)
	var pDuration float64
	for _, it := range shards[0] {
		if it.Pkg == "p" {
			pDuration += it.Duration
		}
	}
	if pDuration != 120 {
		t.Errorf("got p duration %v, want 120", pDuration)
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

func TestPack_NoHistoryAtAll_FallsBackToCountBased(t *testing.T) {
	t.Parallel()
	// 6 tests across 2 packages, no history → count-based split into 3 shards = 2 each.
	pkgs := []Package{
		{Path: "a", Tests: []string{"T1", "T2", "T3"}},
		{Path: "b", Tests: []string{"T4", "T5", "T6"}},
	}
	shards := Pack(History{}, pkgs, 3)
	if len(shards) != 3 {
		t.Fatalf("got %d shards, want 3", len(shards))
	}
	// Exact count-based behavior: each shard gets exactly 2 tests.
	for i, s := range shards {
		count := 0
		for _, it := range s {
			count += len(it.Tests)
		}
		if count != 2 {
			t.Errorf("shard %d: got %d tests, want 2 (count-based split)", i, count)
		}
	}
	// All 6 tests accounted for. Whole-pkg items have Tests=nil, so look up
	// the test count from the original pkgs slice.
	totalTests := 0
	for _, s := range shards {
		for _, it := range s {
			if len(it.Tests) > 0 {
				totalTests += len(it.Tests)
			} else {
				for _, p := range pkgs {
					if p.Path == it.Pkg {
						totalTests += len(p.Tests)
						break
					}
				}
			}
		}
	}
	if totalTests != 6 {
		t.Errorf("count fallback dropped tests: got %d, want 6", totalTests)
	}
}

func TestPack_NoHistory_WholePackagesEmittedAsBatch(t *testing.T) {
	t.Parallel()
	// Single package, all tests on one shard → should emit a whole-pkg Item (Tests=nil).
	pkgs := []Package{{Path: "p", Tests: []string{"T1", "T2"}}}
	shards := Pack(History{}, pkgs, 1)
	if len(shards[0]) != 1 {
		t.Fatalf("got %d items, want 1", len(shards[0]))
	}
	if shards[0][0].Tests != nil {
		t.Errorf("expected Tests=nil for whole-pkg item, got %v", shards[0][0].Tests)
	}
}

func TestPack_NoHistory_FullPkgOnOneShard_EmitsWholePkgItem(t *testing.T) {
	t.Parallel()
	// 2 packages of 3 tests each, into 2 shards. Each shard gets 3 of the 6 tests.
	// Pkg "a"'s 3 tests all land on shard 0 (alphabetic sort). Pkg "b"'s 3 tests
	// all land on shard 1. So both shards should emit ONE whole-pkg item each.
	pkgs := []Package{
		{Path: "a", Tests: []string{"T1", "T2", "T3"}},
		{Path: "b", Tests: []string{"T4", "T5", "T6"}},
	}
	shards := Pack(History{}, pkgs, 2)
	for i, s := range shards {
		if len(s) != 1 {
			t.Errorf("shard %d: got %d items, want 1", i, len(s))
			continue
		}
		if s[0].Tests != nil {
			t.Errorf("shard %d: expected whole-pkg item, got Tests=%v", i, s[0].Tests)
		}
	}
}

func TestPack_NoHistory_PartialFragmentRetainsTests(t *testing.T) {
	t.Parallel()
	// 1 package of 4 tests, 2 shards. Each shard gets 2 of 4 tests → fragments.
	// Both shards should emit Items with Tests populated (NOT nil).
	pkgs := []Package{{Path: "p", Tests: []string{"T1", "T2", "T3", "T4"}}}
	shards := Pack(History{}, pkgs, 2)
	for i, s := range shards {
		if len(s) != 1 {
			t.Errorf("shard %d: got %d items, want 1", i, len(s))
			continue
		}
		if s[0].Tests == nil || len(s[0].Tests) != 2 {
			t.Errorf("shard %d: expected fragment with 2 tests, got Tests=%v", i, s[0].Tests)
		}
	}
}
