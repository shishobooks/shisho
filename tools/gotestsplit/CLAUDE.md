# gotestsplit

In-tree, timing-aware Go test splitter. Forked from [gotesplit](https://github.com/Songmu/gotesplit) (MIT, Songmu). Used by `.github/workflows/ci.yml`'s `test` job.

## What it does

Bin-packs Go tests across N CI shards by **measured per-test wallclock**, not test count. The 3 hot packages today (`pkg/worker`, `pkg/books`, `pkg/plugins` — together 56% of total test time) get split across shards via `-run "^(?:T1|T2|...)$"` regexes so no single shard inherits a 7-minute package alone.

Subcommands:
- `simulate -junit-dir=DIR [-min=N] [-max=N]` — read JUnit history, project per-shard wallclock and CI cost across a range of N. Used for picking shard count.
- `plan -junit-dir=DIR -total=N -index=I [-detail] PKG...` — print the assignment for one shard. Useful for debugging "why is shard 3 slow?".
- `run -junit-dir=DIR [-junit-out=DIR] -total=N -index=I PKG... [-- go-test-args]` — plan + exec `go test` per chunk + emit JUnit XML. `-junit-dir` is the read-only history input; `-junit-out` (defaults to `-junit-dir`) is where fresh XML is written.
- `prune -junit-dir=DIR -total=N` — delete orphan `junit-{shard}-{seq}.xml` files where `shard >= N`. Run by the CI consolidator to keep the cache tidy after shard count changes.

## Architecture cheatsheet

- `pack.go` — pure `Pack(history, packages, N) [][]Item` function. Used by all three subcommands so simulate/plan/run agree. Smart-chunks any package whose wallclock estimate exceeds `ideal * 0.8` where `ideal = total / N`. Within a chunked package, tests are LPT-distributed by per-test duration.
- `junit.go` — reads a directory of JUnit XML files into `History` (`map[pkg]map[test]float64`). Top-level tests only; subtests skipped (`-run` regex targets parents).
- `discover.go` — wraps `go test -list .` to enumerate top-level tests per package.
- `run.go` — execs `go test` per Item: whole-package items batched into one invocation, chunks each get their own `-run` regex invocation. Streams output and parses it via `go-junit-report/v2` to write `junit-{shard}-{seq}.xml` into `-junit-out` (separate from the `-junit-dir` read path).

## Cache strategy in CI

`.github/workflows/ci.yml`'s `test` job uses **restore-only** cache (`actions/cache/restore@v5`) with a key shared across all shards (`gotest-timings-${{ github.ref }}-${{ github.sha }}`).

After tests, each shard uploads the contents of its `-junit-out` directory (`.gotest-timings-fresh/`) as a per-shard artifact (`gotest-timings-shard-N`). Because `-junit-out` is separate from `-junit-dir`, only fresh files exist there — no filtering needed. A separate `consolidate-test-timings` job (`needs: test`, `if: always()`) restores the previous cache, downloads all shard artifacts on top of it, and saves a single `actions/cache/save@v5` entry.

Three non-obvious requirements:

1. **`include-hidden-files: true` on `actions/upload-artifact`** — `.gotest-timings-fresh/` starts with a dot; without this flag the action silently uploads zero files.
2. **All shards must restore from the same cache** — each shard runs `Pack` independently. If shards see different histories, their plans diverge and tests can be missed or duplicated. The shared cache key + post-job consolidator together guarantee a single coherent input on the next run.
3. **Read and write directories must be separate** — `-junit-dir` (read) is the restored cache containing history from all shards; `-junit-out` (write) is a clean directory that only contains this run's fresh files. This separation means the upload step can grab the entire output directory without risk of re-uploading stale data from other shards. The consolidator additionally restores the previous cache before downloading artifacts, so a shard that fails before writing any junit doesn't get its history dropped from the cache permanently.

If the cache is empty (cache miss), `Pack` falls back to count-based equal-count splitting (gotesplit's classic behavior) so first runs on a fresh branch behave like upstream gotesplit.

## Recalculating the optimal shard count — playbook

Hardware, dependencies, and test mix all shift over time. When CI shards start drifting toward the wallclock target, redo this measurement. The simulator's accuracy varies by N (factor 0.59–0.72 of simulated → actual on shisho's CI in 2026-04), so **simulator output alone is insufficient** — you need actual CI runs to validate.

### Step 1: Get a clean cache (consolidated, all shards' data)

The simulator and Pack both depend on having full per-test timing data. Verify the latest CI run's consolidator job succeeded:

```bash
LATEST_RUN=$(gh run list --workflow CI --branch master --limit 1 --json databaseId --jq '.[0].databaseId')
gh run view $LATEST_RUN --log --job $(gh run view $LATEST_RUN --json jobs --jq '.jobs[] | select(.name == "Consolidate test timings") | .databaseId') 2>&1 | grep -E "Found .* artifact|Cache saved with key"
```

You want to see `Found 8 artifact(s)` (or whatever N is) and `Cache saved with key: gotest-timings-...`.

### Step 2: Download the consolidated history locally

```bash
mkdir -p /tmp/ci-junit && rm -f /tmp/ci-junit/*.xml
# Any shard's artifact contains the restored full cache (since restore happens
# before tests). Just pull shard 1.
gh run download $LATEST_RUN -n gotest-timings-shard-1 -D /tmp/ci-junit
ls /tmp/ci-junit/ | wc -l   # should be a healthy double-digit number
```

If you can't get artifacts (older runs may have artifacts retention-expired), regenerate by running `go test -race -count=1 -json ./pkg/...` locally and converting to JUnit (see `scripts/`-equivalent or write a small ad-hoc converter).

### Step 3: Run the simulator across a range of N

```bash
go run ./tools/gotestsplit simulate -junit-dir=/tmp/ci-junit -min=2 -max=14
```

Output looks like:
```
N      slowest        cost
N=8    slowest=14.2m  cost=113.5 min
N=10   slowest=11.4m  cost=114.1 min
N=12   slowest= 9.9m  cost=118.8 min
```

**Important caveat**: the projection sums per-test durations from JUnit. Tests within a package run with `t.Parallel()`, so actual wallclock is substantially less than the simulated value. The `slowest=` numbers will be too high; relative ordering is what matters.

### Step 4: Identify candidate Ns

Look for the knee where `slowest` flattens out — adding more shards stops helping. Then look at `cost` (slowest × N, rough billable CI-minutes) — when cost climbs steeply, you've over-sharded. Pick 2-3 candidate Ns spanning the knee (e.g., one below, one at, one above).

### Step 5: Validate each candidate with a CLEAN CI measurement

Per-test wallclocks in the cache are mostly stable across runs, but Pack's chunking decisions depend on the totals, and a cache that was populated under N=X may yield non-optimal chunking on the first run at N=Y. **Each candidate needs two consecutive CI runs:**

1. **First pass** at N=Y: pushes the matrix change, populates the cache for that N's chunking shape.
2. **Second pass** at N=Y (empty `--allow-empty` commit): restores N=Y's own consolidated cache → Pack produces the optimal plan for N=Y → measurement is clean.

The first pass is contaminated; ignore its slowest-shard time. Use the second pass.

```bash
# In tools/gotestsplit/.. (repo root)
# Edit .github/workflows/ci.yml: matrix.shard list + the -total=N flag in the run step.
git commit -am "[CI] Try N=Y first pass"
git push
# Wait for CI + consolidate-test-timings to finish.
git commit --allow-empty -m "[CI] N=Y second pass (clean)"
git push
# Wait again. The slowest shard time in this second run is the clean measurement.
```

For N=10 vs N=12 in 2026-04 the picture was:

| N | First-pass slowest | Second-pass slowest | Re-run |
|---|---|---|---|
| 8 | (skipped — was the starting state) | 8:23 | — |
| 10 | 9:19 (contaminated by N=8 cache) | 8:15 | — |
| 12 | 9:04 (contaminated by N=10 cache) | 6:39 | 8:39 |

A third N=12 run on the same cache produced a slowest of 8:39 — i.e., **CI runner variance is ~2 minutes on the slowest shard**. A single "clean" measurement can be a lucky outlier. To be confident, take 3+ samples per N and use the median; otherwise treat differences smaller than ~2 minutes as noise.

In 2026-04, N=12 was settled on as the best cost/value balance: typical slowest ~7:30–8:30 with ~2 min headroom against the 10-minute target. N=8 is similarly capable today; the extra 4 shards of N=12 buy ~1 minute of margin against test growth at ~13 CI-minutes of extra spend.

### Step 6: Pick the N and update CI

Edit `.github/workflows/ci.yml`:
- `env.SHARD_TOTAL: N` at the workflow level (the matrix, run step, job name, and prune step all reference this)
- `matrix.shard: [1, 2, ..., N]`

Commit + push. The first run after the change is the contaminated one (Pack uses old-N cache for new-N split); subsequent runs are clean.

## Common gotchas

- **Only one shard's data persists per CI run before the consolidator existed** — historical caches from before this PR (#172) only have ~1/N coverage. They'll auto-fix once the consolidator runs once on master.
- **`gh run view <run-id>` returns "still in progress" until the entire workflow completes** — for partial logs of an in-progress run, view individual jobs via the GHA web UI.
- **`actions/cache/restore@v5` and `actions/cache/save@v5` are separate actions** from `actions/cache@v5`; we use the split form so only the consolidator saves. Don't replace with the unified form — concurrent saves race and only one shard's data wins.
- **`gh pr checks` shows checks for the latest commit on the PR**, so monitors that scope by PR (not run-id) will break if a new commit is pushed mid-monitor. Scope monitors to a specific `RUN_ID` (`gh run view $RUN_ID --json jobs`).
- **The consolidator prunes orphan shard files** — `prune -junit-dir=DIR -total=N` deletes any `junit-{shard}-{seq}.xml` where `shard >= N`. This runs automatically in the `consolidate-test-timings` job before saving the cache, so changing the shard count doesn't leave stale files behind. Dead-test entries within valid shard files are not pruned (live tests overwrite them via later-wins merging, so the impact is negligible).
- **Re-running CI on the same SHA fails the consolidator's `Save consolidated cache` step** — `actions/cache/save@v5` keys are immutable per (repo, key), and the key includes `github.sha`. The first attempt's save succeeded; the re-run's save sees the key already exists and exits non-zero. This is harmless — the previous attempt's consolidated cache is still there, and subsequent commits restore it via the `gotest-timings-${{ github.ref }}-` fallback prefix — but the red ✗ on the Save step in re-runs is expected, not a regression. Don't add `github.run_attempt` to the key to "fix" it; that fragments the cache pointlessly.

## Pull-out plan (future)

The tool deliberately avoids importing anything from `pkg/` — it only depends on the standard library + `github.com/jstemmer/go-junit-report/v2`. To extract:

1. `git filter-repo --path tools/gotestsplit/`
2. `cd tools/gotestsplit && go mod init github.com/<owner>/gotestsplit`
3. Move `LICENSE` to repo root, write a real README, tag a release.

The `.github/workflows/ci.yml` integration would then become `go install` of a tagged version instead of `go run ./tools/gotestsplit`.
