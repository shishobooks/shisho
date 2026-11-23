package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pkg/errors"
)

// flowSeconds is about how long each flow takes on a CI runner, to balance
// the flows across CI's parallel jobs (`ios -shard`). A flow missing here
// counts as the median, so a new one still runs; add it once CI has timed it
// (the `[Passed]` lines in the job's log).
var flowSeconds = map[string]float64{
	"access_change.yaml":              66,
	"bookmark_offline.yaml":           104,
	"chapter_navigation.yaml":         102,
	"listen_and_resume.yaml":          180,
	"listen_offline.yaml":             159,
	"listen_to_the_end.yaml":          58,
	"search_offline.yaml":             62,
	"sign_in_and_browse_offline.yaml": 144,
	"sleep_timer_end_of_chapter.yaml": 178,
	"speed_per_file.yaml":             265,
	"switch_accounts.yaml":            75,
}

// topLevelFlows lists the flows in dir that Maestro runs: its YAML files
// other than config.yaml, not the subflows folder's.
func topLevelFlows(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var flows []string
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() && strings.HasSuffix(name, ".yaml") && name != "config.yaml" {
			flows = append(flows, name)
		}
	}
	return flows, nil
}

// shardFlows returns the flows shard index of total runs (1-based), packing
// the longest first onto the shard with the least time so far, so the
// shards finish at about the same time, and orders them shortest first.
// Every flow lands on exactly one shard.
func shardFlows(flows []string, index, total int, seconds map[string]float64) ([]string, error) {
	if total < 1 || index < 1 || index > total {
		return nil, errors.Errorf("shard %d of %d: want 1 to %d", index, total, total)
	}
	known := []float64{}
	for _, s := range seconds {
		known = append(known, s)
	}
	sort.Float64s(known)
	median := 60.0
	if len(known) > 0 {
		median = known[len(known)/2]
	}
	cost := func(flow string) float64 {
		if s, ok := seconds[flow]; ok {
			return s
		}
		return median
	}
	sorted := append([]string(nil), flows...)
	sort.Slice(sorted, func(i, j int) bool {
		if cost(sorted[i]) != cost(sorted[j]) {
			return cost(sorted[i]) > cost(sorted[j])
		}
		return sorted[i] < sorted[j]
	})
	load := make([]float64, total)
	var mine []string
	for _, flow := range sorted {
		least := 0
		for i := range load {
			if load[i] < load[least] {
				least = i
			}
		}
		load[least] += cost(flow)
		if least == index-1 {
			mine = append(mine, flow)
		}
	}
	// The shortest first: a cold runner's first flow is its slowest.
	for i, j := 0, len(mine)-1; i < j; i, j = i+1, j-1 {
		mine[i], mine[j] = mine[j], mine[i]
	}
	return mine, nil
}

// shardFlowPaths returns the paths of the flows in dir that shard ("i/n")
// runs.
func shardFlowPaths(dir, shard string) ([]string, error) {
	var index, total int
	if n, err := fmt.Sscanf(shard, "%d/%d", &index, &total); err != nil || n != 2 {
		return nil, errors.Errorf("-shard %q: want i/n, such as 1/3", shard)
	}
	flows, err := topLevelFlows(dir)
	if err != nil {
		return nil, err
	}
	mine, err := shardFlows(flows, index, total, flowSeconds)
	if err != nil {
		return nil, err
	}
	paths := make([]string, len(mine))
	for i, flow := range mine {
		paths[i] = filepath.Join(dir, flow)
	}
	return paths, nil
}
