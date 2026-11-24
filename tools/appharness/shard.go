package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/pkg/errors"
)

// flowSeconds is about how long each flow takes on a CI runner, to balance
// the flows when they're split into shards (`ios -shard` on GitHub's runners,
// `ios -parallel` in the signoff). A new flow gets an entry in the same
// change, which TestFlowSecondsNameEveryFlow checks. It starts as an
// estimate: scale its local time by how a flow already here compares with
// its local time, since that ratio moves with the runners. Estimates, and
// flows that changed since CI last timed them, last until CI times them (the
// `[Passed]` lines in the job's log).
var flowSeconds = map[string]float64{
	"access_change.yaml":              92,
	"browse_offline.yaml":             85,
	"download_and_play_offline.yaml":  188,
	"listen_offline.yaml":             280,
	"listen_to_the_end.yaml":          98,
	"read_and_reopen.yaml":            160,
	"read_fixed_layout.yaml":          76,
	"reader_footnote.yaml":            185,
	"reader_rapid_turns.yaml":         235,
	"reader_settings.yaml":            300,
	"server_set_up_again.yaml":        117,
	"sign_out_sends_changes.yaml":     270,
	"sleep_timer_end_of_chapter.yaml": 180,
	"switch_accounts.yaml":            94,
}

// topLevelFlows lists the flows in dir that Maestro runs: its .yaml and
// .yml files other than its config, not the subflows folder's.
func topLevelFlows(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var flows []string
	for _, entry := range entries {
		name := entry.Name()
		ext := filepath.Ext(name)
		if !entry.IsDir() && (ext == ".yaml" || ext == ".yml") && name != "config.yaml" && name != "config.yml" {
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
