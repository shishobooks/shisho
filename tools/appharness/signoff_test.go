package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSignoffPassesAreKeptPerCommit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	assert.False(t, hasSignoffPass(dir, "aaa"))

	require.NoError(t, recordSignoffPass(dir, "aaa", time.Now()))
	assert.True(t, hasSignoffPass(dir, "aaa"))
	assert.False(t, hasSignoffPass(dir, "bbb"))
}

func TestRecordingASignoffPassPrunesOldOnes(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, recordSignoffPass(dir, "old", time.Now()))
	old := time.Now().Add(-signoffPassesKept - time.Hour)
	require.NoError(t, os.Chtimes(filepath.Join(dir, "old"), old, old))

	require.NoError(t, recordSignoffPass(dir, "new", time.Now()))
	assert.False(t, hasSignoffPass(dir, "old"))
	assert.True(t, hasSignoffPass(dir, "new"))
}

func TestCIOKIsRerunOnlyWhenItAloneFailed(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		jobs []ciJob
		want bool
	}{
		{
			name: "only CI OK failed",
			jobs: []ciJob{
				{Name: "Go Lint", Conclusion: "success"},
				{Name: "iOS flows", Conclusion: "skipped"},
				{Name: ciOKJob, Conclusion: "failure"},
			},
			want: true,
		},
		{
			name: "another job failed too",
			jobs: []ciJob{
				{Name: "Go Test (1/2)", Conclusion: "failure"},
				{Name: ciOKJob, Conclusion: "failure"},
			},
		},
		{
			name: "another job was cancelled",
			jobs: []ciJob{
				{Name: "iOS", Conclusion: "cancelled"},
				{Name: ciOKJob, Conclusion: "failure"},
			},
		},
		{
			name: "CI OK passed",
			jobs: []ciJob{{Name: ciOKJob, Conclusion: "success"}},
		},
		{
			name: "no CI OK",
			jobs: []ciJob{{Name: "Go Lint", Conclusion: "failure"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, onlyCIOKFailed(tc.jobs))
		})
	}
}

// CI checks for the status the signoff posts, under the name the harness
// posts it as, in the workflow that has the job it reruns.
func TestCIChecksTheStatusTheSignoffPosts(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("../../.github/workflows/ci.yml")
	require.NoError(t, err)
	workflow := string(data)
	assert.True(t, strings.HasPrefix(workflow, "name: "+ciWorkflow+"\n"), "the workflow is named %q", ciWorkflow)
	for _, want := range []string{"name: " + ciOKJob + "\n", `select(.context == "` + signoffContext + `")`} {
		assert.True(t, strings.Contains(workflow, want), "ci.yml has %q", want)
	}
}
