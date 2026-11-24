package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
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

func TestOnlyThePullRequestsNewestCIRunIsRerun(t *testing.T) {
	t.Parallel()
	failed := ciRun{ID: 10, Name: ciWorkflow, Event: "pull_request", Status: "completed", Conclusion: "failure"}
	for _, tc := range []struct {
		name string
		runs []ciRun
		want int64
	}{
		{name: "the newest failed", runs: []ciRun{failed}, want: 10},
		{
			name: "a newer run is still going",
			runs: []ciRun{failed, {ID: 11, Name: ciWorkflow, Event: "pull_request", Status: "in_progress"}},
		},
		{
			name: "a newer run passed",
			runs: []ciRun{failed, {ID: 11, Name: ciWorkflow, Event: "pull_request", Status: "completed", Conclusion: "success"}},
		},
		{
			name: "other workflows and events don't count",
			runs: []ciRun{
				failed,
				{ID: 12, Name: "Docs", Event: "pull_request", Status: "completed", Conclusion: "success"},
				{ID: 13, Name: ciWorkflow, Event: "push", Status: "completed", Conclusion: "success"},
			},
			want: 10,
		},
		{name: "no runs"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			run, ok := latestFailedCIRun(tc.runs)
			assert.Equal(t, tc.want != 0, ok)
			if ok {
				assert.Equal(t, tc.want, run.ID)
			}
		})
	}
}

// The flows run on GitHub's runners exactly when CI OK doesn't wait for a
// signoff, so every pull request gets one or the other.
func TestCIRunsTheFlowsOrWaitsForTheSignoffNeverNeither(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("../../.github/workflows/ci.yml")
	require.NoError(t, err)
	var workflow struct {
		Jobs map[string]struct {
			If    string `yaml:"if"`
			Steps []struct {
				Name string `yaml:"name"`
				If   string `yaml:"if"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	require.NoError(t, yaml.Unmarshal(data, &workflow))
	const changed = "needs.app-changes.outputs.changed == 'true'"
	assert.Equal(t, changed+" && needs.app-changes.outputs.signoff != 'true'", workflow.Jobs["app-ios-flows"].If)
	var wait string
	for _, step := range workflow.Jobs["ci-ok"].Steps {
		if step.Name == "Wait for the iOS flows signoff" {
			wait = step.If
		}
	}
	assert.Equal(t, changed+" && needs.app-changes.outputs.signoff == 'true'", wait)
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
		assert.Contains(t, workflow, want)
	}
}
