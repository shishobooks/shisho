package main

import (
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEachParallelShardHasItsOwnWorktreeSimulator(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "flows", flowsPurpose(1))
	assert.Equal(t, "flows-2", flowsPurpose(2))
	for shard := 1; shard <= maxParallelShards; shard++ {
		// So `mise cleanup` deletes it too.
		assert.Contains(t, worktreePurposes, flowsPurpose(shard))
	}
}

func TestParallelShardsLabelEachWholeLine(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	var mu sync.Mutex
	one := &labelWriter{label: "[1/2] ", out: &out, mu: &mu}
	two := &labelWriter{label: "[2/2] ", out: &out, mu: &mu}

	_, err := one.Write([]byte("[Passed] Sign in"))
	require.NoError(t, err)
	_, err = two.Write([]byte("[Passed] Listen\n[Failed] Re"))
	require.NoError(t, err)
	_, err = one.Write([]byte(" (12s)\n"))
	require.NoError(t, err)
	_, err = two.Write([]byte("ad (3s)\n"))
	require.NoError(t, err)
	one.Flush()
	two.Flush()

	assert.Equal(t,
		"[2/2] [Passed] Listen\n[1/2] [Passed] Sign in (12s)\n[2/2] [Failed] Read (3s)\n",
		out.String())
}

func TestALabelWriterFlushesAnUnfinishedLastLine(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	w := &labelWriter{label: "[1/2] ", out: &out, mu: &sync.Mutex{}}
	_, err := w.Write([]byte("no newline"))
	require.NoError(t, err)
	assert.Empty(t, out.String())
	w.Flush()
	assert.Equal(t, "[1/2] no newline\n", out.String())
}

func TestParallelRunsRefuseAShard(t *testing.T) {
	t.Parallel()
	_, err := parallelShards(2, "1/2")
	require.Error(t, err)
	_, err = parallelShards(maxParallelShards+1, "")
	require.Error(t, err)
	shards, err := parallelShards(2, "")
	require.NoError(t, err)
	assert.Equal(t, []string{"1/2", "2/2"}, shards)
	shards, err = parallelShards(1, "1/3")
	require.NoError(t, err)
	assert.Equal(t, []string{"1/3"}, shards)
	shards, err = parallelShards(1, "")
	require.NoError(t, err)
	assert.Equal(t, []string{""}, shards)
}
