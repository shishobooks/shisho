package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShardFlowsRunsEveryFlowOnce(t *testing.T) {
	t.Parallel()
	flows := []string{"a.yaml", "b.yaml", "c.yaml", "d.yaml", "e.yaml", "new.yaml"}
	seconds := map[string]float64{"a.yaml": 300, "b.yaml": 200, "c.yaml": 100, "d.yaml": 100, "e.yaml": 50}
	seen := map[string]int{}
	for i := 1; i <= 3; i++ {
		shard, err := shardFlows(flows, i, 3, seconds)
		require.NoError(t, err)
		assert.NotEmpty(t, shard)
		for _, f := range shard {
			seen[f]++
		}
	}
	for _, f := range flows {
		assert.Equal(t, 1, seen[f], f)
	}
}

func TestShardFlowsBalancesByTime(t *testing.T) {
	t.Parallel()
	flows := []string{"a.yaml", "b.yaml", "c.yaml", "d.yaml"}
	seconds := map[string]float64{"a.yaml": 300, "b.yaml": 200, "c.yaml": 150, "d.yaml": 100}
	first, err := shardFlows(flows, 1, 2, seconds)
	require.NoError(t, err)
	second, err := shardFlows(flows, 2, 2, seconds)
	require.NoError(t, err)
	// 300 + 100 against 200 + 150.
	assert.ElementsMatch(t, []string{"a.yaml", "d.yaml"}, first)
	assert.ElementsMatch(t, []string{"b.yaml", "c.yaml"}, second)
}

func TestShardFlowsRefusesABadShard(t *testing.T) {
	t.Parallel()
	for _, shard := range [][2]int{{0, 3}, {4, 3}, {1, 0}} {
		_, err := shardFlows([]string{"a.yaml"}, shard[0], shard[1], nil)
		assert.Error(t, err, shard)
	}
}

// Each shard runs its shortest flow first, so the slow first flow on a cold
// runner is a short one.
func TestShardFlowsRunShortestFirst(t *testing.T) {
	t.Parallel()
	flows := []string{"a.yaml", "b.yaml", "c.yaml"}
	seconds := map[string]float64{"a.yaml": 300, "b.yaml": 50, "c.yaml": 100}
	shard, err := shardFlows(flows, 1, 1, seconds)
	require.NoError(t, err)
	assert.Equal(t, []string{"b.yaml", "c.yaml", "a.yaml"}, shard)
}
