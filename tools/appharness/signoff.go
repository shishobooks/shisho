package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
)

// The signoff is how a pull request from this repository passes the iOS
// unit tests and flows: they run here, on a Mac of the maintainer's, and a
// commit status on the tested commit says so. `CI OK`
// (.github/workflows/ci.yml) waits for that status instead of running them
// on GitHub's runners, which take several times longer. Nothing reaches
// this Mac from GitHub: only accounts with write access can post a status,
// and a fork's pull request still runs them on GitHub's runners.
const (
	signoffContext = "iOS signoff"
	ciWorkflow     = "CI"
	ciOKJob        = "CI OK"
)

// signoffPassesKept is how long a recorded pass is kept, so the signoff can
// be posted after the commit is pushed without running the checks again.
const signoffPassesKept = 30 * 24 * time.Hour

// runSignoff runs the unit tests and every flow on HEAD, which must have
// nothing uncommitted, records the pass, and posts the signoff once the
// commit is on GitHub. A commit that already passed here only posts.
// Posting also reruns a `CI OK` that failed only for want of the signoff.
func runSignoff(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("signoff", flag.ExitOnError)
	keep := flags.Bool("keep", false, "keep each shard's folder (server log, database, Maestro output) for debugging")
	parallel := flags.Int("parallel", maxParallelShards, "how many shards of the flows run at once; 1 on a busy Mac")
	if err := flags.Parse(args); err != nil {
		return err
	}
	root, err := os.Getwd()
	if err != nil {
		return errors.WithStack(err)
	}
	sha, err := cleanHead(ctx, root)
	if err != nil {
		return err
	}
	// The signoff tests the commit, not its merge with master, which GitHub's
	// runners did, so say when master has moved on.
	if behind, err := behindBase(ctx, root, "origin/master"); err == nil && behind {
		fmt.Fprintln(os.Stderr, "appharness: origin/master has commits this branch doesn't, and the signoff tests this commit, not its merge with them; merge master first if they touch the app")
	}
	passes, err := signoffPassDir()
	if err != nil {
		return err
	}
	if hasSignoffPass(passes, sha) {
		fmt.Fprintf(os.Stderr, "appharness: the checks already passed on %s here\n", short(sha))
	} else {
		// Built only now that the commit is known to be clean, so the app
		// the checks test is the commit's. ios:test builds it (the flows use
		// the same build) and runs the unit tests first, as they fail fastest.
		if err := runCommand(ctx, root, "mise", "run", "ios:test"); err != nil {
			return errors.Wrap(err, "the unit tests")
		}
		// Its simulator stays booted for later runs, but the flows boot two
		// more, and three at once is the memory pressure that brings stalls.
		shutdownWorktreeSimulator(ctx, root, "tests")
		if err := runIOSWith(ctx, iosOptions{
			app: defaultAppPath, flows: defaultFlowsDir, keep: *keep, parallel: *parallel,
		}); err != nil {
			return err
		}
		// The run tested the checkout, so it only vouches for the commit if
		// nothing changed while it built and ran.
		after, err := cleanHead(ctx, root)
		if err != nil {
			return errors.Wrap(err, "the checkout changed during the run")
		}
		if after != sha {
			return errors.Errorf("HEAD moved from %s to %s during the run", short(sha), short(after))
		}
		if err := recordSignoffPass(passes, sha, time.Now()); err != nil {
			return err
		}
	}

	pushed, err := commitOnGitHub(ctx, root, sha)
	if err != nil {
		return err
	}
	if !pushed {
		fmt.Fprintf(os.Stderr, "appharness: the checks passed on %s, which isn't on GitHub yet; push it, then run `mise ios:signoff` again to post the signoff without running them again\n", short(sha))
		return nil
	}
	if _, err := gh(ctx, "api", "--method", "POST", "repos/{owner}/{repo}/statuses/"+sha,
		"-f", "state=success",
		"-f", "context="+signoffContext,
		"-f", "description=Every iOS unit test and Maestro flow passed on a maintainer's Mac (mise ios:signoff)",
	); err != nil {
		return errors.Wrap(err, "post the signoff")
	}
	fmt.Fprintf(os.Stderr, "appharness: posted the signoff on %s\n", short(sha))
	if err := rerunCIOK(ctx, sha); err != nil {
		// The signoff is posted; rerunning the check by hand still works.
		fmt.Fprintf(os.Stderr, "appharness: couldn't rerun %s: %v\n", ciOKJob, err)
	}
	return nil
}

// cleanHead returns HEAD's commit, or an error when the checkout has changes
// or untracked files: the build picks up untracked files too, so a run with
// either wouldn't be testing the commit the signoff names.
func cleanHead(ctx context.Context, root string) (string, error) {
	status, err := git(ctx, root, "status", "--porcelain", "--untracked-files=normal")
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(status) != "" {
		return "", errors.Errorf("the signoff vouches for a commit, so the checkout must hold nothing else. Commit these, or set them aside with a named `git stash push -u -m`; never delete files you didn't make:\n%s", status)
	}
	sha, err := git(ctx, root, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(sha), nil
}

func git(ctx context.Context, root string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = root
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", errors.Wrapf(err, "git %s: %s", strings.Join(args, " "), strings.TrimSpace(stderr.String()))
	}
	return string(out), nil
}

// gh runs the GitHub CLI, whose `{owner}/{repo}` placeholders name the
// repository the checkout's remote points at.
func gh(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return out, errors.Wrapf(err, "gh %s: %s", strings.Join(args, " "), strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// runCommand runs a command in root with its output on this one's.
func runCommand(ctx context.Context, root, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = root
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return errors.Wrapf(cmd.Run(), "%s %s", name, strings.Join(args, " "))
}

// behindBase reports whether base has commits HEAD doesn't. A checkout
// without base isn't behind it.
func behindBase(ctx context.Context, root, base string) (bool, error) {
	if _, err := git(ctx, root, "rev-parse", "--verify", "--quiet", base+"^{commit}"); err != nil {
		return false, nil //nolint:nilerr // No base to be behind.
	}
	cmd := exec.CommandContext(ctx, "git", "merge-base", "--is-ancestor", base, "HEAD")
	cmd.Dir = root
	err := cmd.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return true, nil
	}
	return false, errors.Wrap(err, "git merge-base")
}

// commitOnGitHub reports whether the commit is on a branch pushed to the
// checkout's remote, as a status needs it on GitHub.
func commitOnGitHub(ctx context.Context, root, sha string) (bool, error) {
	branches, err := git(ctx, root, "branch", "--remotes", "--contains", sha)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(branches) != "", nil
}

type ciRun struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Event        string `json:"event"`
	Status       string `json:"status"`
	Conclusion   string `json:"conclusion"`
	PullRequests []struct {
		Number int `json:"number"`
	} `json:"pull_requests"`
}

type ciJob struct {
	Name       string `json:"name"`
	Conclusion string `json:"conclusion"`
}

// latestFailedCIRun returns the newest CI run among runs for one of pulls
// (the open pull requests this commit still heads), when it has finished
// and failed. A run for a pull request that has moved on to another commit
// is never rerun: it would join that pull request's concurrency group and
// cancel its newer run. One still going finds the signoff itself.
func latestFailedCIRun(runs []ciRun, pulls map[int]bool) (ciRun, bool) {
	var latest ciRun
	for _, r := range runs {
		if r.Name != ciWorkflow || r.Event != "pull_request" || r.ID <= latest.ID {
			continue
		}
		for _, pull := range r.PullRequests {
			if pulls[pull.Number] {
				latest = r
				break
			}
		}
	}
	return latest, latest.ID != 0 && latest.Status == "completed" && latest.Conclusion == "failure"
}

// onlyCIOKFailed reports whether a finished run failed only because `CI OK`
// found no signoff, so rerunning that one job passes it now.
func onlyCIOKFailed(jobs []ciJob) bool {
	failed := false
	for _, job := range jobs {
		switch {
		case job.Name == ciOKJob && job.Conclusion == "failure":
			failed = true
		case job.Name == ciOKJob:
			return false
		case job.Conclusion != "success" && job.Conclusion != "skipped":
			return false
		}
	}
	return failed
}

// rerunCIOK reruns `CI OK` when sha heads an open pull request whose newest
// CI run, on that commit, failed only there: it gave up waiting before the
// signoff came.
func rerunCIOK(ctx context.Context, sha string) error {
	out, err := gh(ctx, "api", "repos/{owner}/{repo}/commits/"+sha+"/pulls")
	if err != nil {
		return err
	}
	var pulls []struct {
		Number int    `json:"number"`
		State  string `json:"state"`
		Head   struct {
			SHA string `json:"sha"`
		} `json:"head"`
	}
	if err := json.Unmarshal(out, &pulls); err != nil {
		return errors.WithStack(err)
	}
	heads := map[int]bool{}
	for _, pull := range pulls {
		if pull.State == "open" && pull.Head.SHA == sha {
			heads[pull.Number] = true
		}
	}
	if len(heads) == 0 {
		return nil
	}
	out, err = gh(ctx, "api", "repos/{owner}/{repo}/actions/runs?event=pull_request&head_sha="+sha+"&per_page=100")
	if err != nil {
		return err
	}
	var runs struct {
		WorkflowRuns []ciRun `json:"workflow_runs"`
	}
	if err := json.Unmarshal(out, &runs); err != nil {
		return errors.WithStack(err)
	}
	latest, failed := latestFailedCIRun(runs.WorkflowRuns, heads)
	if !failed {
		return nil
	}
	out, err = gh(ctx, "api", fmt.Sprintf("repos/{owner}/{repo}/actions/runs/%d/jobs?filter=latest&per_page=100", latest.ID))
	if err != nil {
		return err
	}
	var jobs struct {
		Jobs []ciJob `json:"jobs"`
	}
	if err := json.Unmarshal(out, &jobs); err != nil {
		return errors.WithStack(err)
	}
	if !onlyCIOKFailed(jobs.Jobs) {
		return nil
	}
	if _, err := gh(ctx, "api", "--method", "POST", fmt.Sprintf("repos/{owner}/{repo}/actions/runs/%d/rerun-failed-jobs", latest.ID)); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "appharness: reran %s in CI run %d\n", ciOKJob, latest.ID)
	return nil
}

// signoffPassDir is the folder of recorded passes, one empty file per
// commit.
func signoffPassDir() (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", errors.WithStack(err)
	}
	return filepath.Join(cache, "shisho-appharness", "signoffs"), nil
}

func hasSignoffPass(dir, sha string) bool {
	_, err := os.Stat(filepath.Join(dir, sha))
	return err == nil
}

// recordSignoffPass records that the checks passed on sha, and forgets
// passes older than signoffPassesKept.
func recordSignoffPass(dir, sha string, now time.Time) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return errors.WithStack(err)
	}
	if entries, err := os.ReadDir(dir); err == nil {
		for _, entry := range entries {
			info, err := entry.Info()
			if err == nil && now.Sub(info.ModTime()) > signoffPassesKept {
				_ = os.Remove(filepath.Join(dir, entry.Name()))
			}
		}
	}
	return errors.WithStack(os.WriteFile(filepath.Join(dir, sha), nil, 0o600))
}

func short(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}
