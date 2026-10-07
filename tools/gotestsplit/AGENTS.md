# gotestsplit

Timing-aware Go test splitter used by the `test` job in `.github/workflows/ci.yml`, forked from gotesplit (MIT). It bin-packs tests across N shards by measured per-test wallclock read from JUnit history, and splits hot packages (`pkg/worker`, `pkg/books`, `pkg/plugins`) across shards with `-run "^(?:T1|T2|...)$"` regexes. Subcommands (`simulate`, `plan`, `run`, `prune`) are in `main.go` and the README.

## Design rules

- **`Pack` (`pack.go`) is the single planner** used by `simulate`, `plan`, and `run`, so their outputs agree. It chunks any package whose estimate exceeds `0.8 * total / N` and distributes that package's tests longest-first. With no history it falls back to equal-count splitting.
- History is top-level tests only (`junit.go`); subtests are skipped because `-run` targets parents.
- The tool imports nothing from `pkg/`, only the standard library and `go-junit-report/v2`, so it can be extracted into its own repo. Keep it that way.

## CI cache

1. **Every shard restores the same cache** (`actions/cache/restore`, key `gotest-timings-${{ github.ref }}-${{ github.sha }}`). Each shard runs `Pack` independently; different histories mean divergent plans and missed or duplicated tests.
2. **Read and write directories are separate**: `-junit-dir` (`.gotest-timings`, restored history) and `-junit-out` (`.gotest-timings-fresh`, this run only), so each shard uploads only its fresh files.
3. **`include-hidden-files: true` on `upload-artifact`**, or the dot-directory uploads zero files without error.
4. **Only `consolidate-test-timings` saves**, using the split `cache/restore` and `cache/save` actions. It restores the previous cache, overlays every shard artifact (so a shard that failed before writing JUnit keeps its old history), runs `prune -total=$SHARD_TOTAL` to delete files for shards `>= N`, and saves. Do not switch to the unified `actions/cache`: concurrent saves race and one shard wins.
5. **Re-running CI on the same SHA fails the consolidator's save step.** Cache keys are immutable and include `github.sha`; the first attempt's cache is still there and later commits restore it through the prefix fallback. Do not add `github.run_attempt` to the key, which fragments the cache.

## Changing the shard count

Change `env.SHARD_TOTAL`, the `matrix.shard` list, and the job `name` (env is not available there) together. The first run at a new N plans from a cache shaped by the old N, so its timing is contaminated.

To pick N:

1. Confirm the latest master run's "Consolidate test timings" job found every shard artifact and saved the cache, then download any shard's artifact (it contains the full restored cache): `gh run download $RUN -n gotest-timings-shard-1 -D /tmp/ci-junit`.
2. `go run ./tools/gotestsplit simulate -junit-dir=/tmp/ci-junit -min=2 -max=14`. Simulated `slowest` overstates wallclock (parallel tests within a package overlap; actual ran at roughly 0.6 to 0.7 of simulated), so read only the relative shape: find the knee where `slowest` flattens and `cost` (slowest x N) starts climbing.
3. Validate two or three candidates in real CI. Each needs a first push at that N plus an `--allow-empty` second push; measure only the second. Runner variance on the slowest shard is about 2 minutes, so take three or more samples and use the median.

The current N is 12, chosen for headroom against the 10-minute shard target.

Scope CI monitors to a run id (`gh run view $RUN_ID --json jobs`), not a PR: `gh pr checks` follows the latest commit, and `gh run view` reports a run as in progress until the whole workflow finishes.
