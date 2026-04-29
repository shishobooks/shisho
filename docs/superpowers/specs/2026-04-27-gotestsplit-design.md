# gotestsplit Design

## Overview

Replace the current CI use of [gotesplit](https://github.com/Songmu/gotesplit) with an in-tree Go tool that splits Go tests across CI shards based on **measured per-test duration**, not test count. The tool also exposes a `simulate` subcommand so the optimal shard count can be chosen by reading existing timing data, without burning CI runs on trial matrices.

## Motivation

Today's CI runs `gotesplit -total=5` over `./pkg/...` with `-race`. gotesplit splits the sorted, flattened test list into N equal-count buckets — it has no awareness of how long each test takes. The result is uneven shards: the fastest finishes in ~4 min while the slowest takes ~10 min.

Measured baseline (sequential `go test -race -count=1 ./pkg/...`):

- Total wallclock: 30.1 min across 51 packages
- Three hot packages dominate (56% of total time): `pkg/worker` 6.8m, `pkg/books` 5.4m, `pkg/plugins` 4.6m
- Theoretical floor with 5 shards: 6.0 min
- Floor with package-level splitting only: 6.8 min (`pkg/worker` alone exceeds the ideal shard time)
- Floor with test-level chunking of the 3 hot packages: 6.1–6.3 min

Test-level splitting is required to meaningfully improve over the package-level floor. The hot packages have plenty of granularity (worker has 311 top-level tests, plugins 416, books 138; longest individual test is 38s).

## Non-goals

- Replacing gotesplit upstream or publishing this tool as a reusable library. We may extract it later, but for now it lives in-tree and only shisho's CI consumes it.
- Cross-language support. Go-only.
- Distributing tests across machines for non-CI use cases (local parallel runs, etc.). Future work if useful.

## Tool location & distribution

Lives at `tools/gotestsplit/` in this repo as a self-contained Go module (no imports from `pkg/`). Built and run from CI with `go run ./tools/gotestsplit ...`. No separate release process.

The core packing logic stays free of shisho-specific assumptions so the tool can be lifted out into its own repo later if useful.

## CLI surface

Three subcommands sharing a common bin-packer:

```
# Project per-shard time and CI cost across a range of N
gotestsplit simulate -junit-dir=.gotest-timings [-min=2] [-max=10]

# Print the assignment for one (N, I) — useful for debugging
gotestsplit plan -junit-dir=... -total=N -index=I [-detail]

# Plan + exec `go test` for shard I, then write JUnit for the next run
gotestsplit run -junit-dir=... -total=N -index=I ./pkg/... -- -race
```

`run` is the default: `gotestsplit -total=N -index=I ./pkg/... -- -race` works as a drop-in replacement for the current `gotesplit` invocation.

### `simulate` output (default format)

```
N=2  slowest=15.1m  cost=30.2 min
N=3  slowest=10.5m  cost=31.5 min
N=5  slowest= 6.3m  cost=31.5 min
N=7  slowest= 6.3m  cost=44.1 min   ← more shards, no faster
N=10 slowest= 6.2m  cost=62.0 min
```

`cost` = `slowest × N` and represents billable CI-minutes. Surfaces the parallelism-vs-spend tradeoff so the operator can stop adding shards once `slowest` flatlines.

### `plan -detail` output

```
shard 1: 6.1m  [worker#1/2, kobo, auth, ereader, ...]
shard 2: 6.2m  [worker#2/2, books#1/2, settings, jobs, ...]
...
```

Used to debug "why is shard 3 slow?" Not on by default — verbose.

## Bin-packing algorithm

A pure `pack(history, packages, N) → [N][]assignment` function used by all three subcommands so simulate, plan, and run always agree.

1. **Read history.** Walk `-junit-dir`, parse JUnit XMLs into `map[pkg]map[test]duration`. Top-level tests only — subtests of the form `Test/Subtest` are skipped because we can't target them individually with `-run` (the parent regex captures them anyway).
2. **Discover tests.** `go test -list . pkg...` per package. (Reuse gotesplit's existing `getTestListsFromPkgs`.) `simulate` skips this step and projects from JUnit history alone — newly-added tests without history are invisible to it, which is acceptable since simulation is for tuning N, not for an exact CI dry-run.
3. **Smart-chunk hot packages.**
   - `total = sum(known package wallclocks)`; `ideal = total / N`.
   - For each package where `pkg_wallclock > ideal * 0.8`, split into `K = ceil(pkg_wallclock / (ideal * 0.8))` chunks.
   - Within the package, assign tests to chunks via LPT (longest test first into shortest chunk). Stable across runs given stable history.
4. **LPT shards.** Bin-pack chunks-and-whole-packages across N shards, longest item first into shortest shard.

LPT (Longest Processing Time) is the standard ⁴⁄₃-approximation greedy for makespan minimization — optimal-enough for this problem and trivial to implement.

### Unknown tests

A test in `go test -list` with no JUnit history → assigned the package's median test duration. If the entire package has no history → assigned a 30s placeholder. New tests therefore don't blow projections; in the worst case a single new long test ends up on whichever shard the placeholder was assigned to and is corrected on the next run.

### Determinism

`pack()` is pure: same JUnit dir + same `(N, I)` → same assignments. Each shard `I` independently recomputes the same packing — no coordination, no shared state at runtime.

## Runtime emission

For shard `I`, group its assigned items by package:

- **Whole packages** → `go test [opts] pkg1 pkg2 ...` (one invocation; lets `go test`'s native package parallelism do its thing).
- **Chunks** → `go test [opts] -run "^(?:T1|T2|...)$" pkg` (one invocation per chunk).

Capture `go test -v` output, parse with `github.com/jstemmer/go-junit-report/v2` (already a gotesplit dep), write `junit-{I}-{seq}.xml` into `-junit-dir` for the next run. Keep gotesplit's existing coverprofile-merging behavior unchanged.

## CI integration

```yaml
- uses: actions/cache@v4
  with:
    path: .gotest-timings
    key: gotest-timings-${{ github.ref }}-${{ github.sha }}
    restore-keys: |
      gotest-timings-${{ github.ref }}-
      gotest-timings-refs/heads/master-
- run: |
    go run ./tools/gotestsplit run \
      -junit-dir=.gotest-timings \
      -total=5 -index=$((SHARD - 1)) \
      ./pkg/... -- -race
```

Cache miss → `pack()` finds an empty history → falls back to gotesplit's count-based split. The first build on a fresh branch transparently degrades to today's behavior, then improves on the next run once master timings populate.

The cache key includes `github.sha` so each run produces a fresh entry; `restore-keys` lets it fall back to the most recent ref-or-master entry. PR runs have `github.ref = refs/pull/N/merge`, which won't match a feature branch's prior cache, but the master fallback keeps them well-served. JUnit dirs from all 5 shards merge naturally — each shard writes uniquely-named files (`junit-{I}-{seq}.xml`).

## Testing

- **Unit tests for `pack()`**: golden cases covering skewed input (one giant package), missing history (all-unknown package, partial history), exact-fits, single-test packages.
- **Unit tests for the JUnit parser**: a few real shisho JUnit files committed under `tools/gotestsplit/testdata/`.
- **One integration test**: build the tool, run it against a tiny fixture Go module with known test durations (using `time.Sleep` to control timings), assert the resulting shard assignments match expectations.

## Open follow-ups (out of scope for first cut)

- Choosing the new CI shard count. Run `simulate` against real timing data once the tool is built and pick the cost/wallclock sweet spot.
- Tuning the `0.8` smart-chunking threshold if simulation suggests a different value works better.
- Surfacing the simulator output in PR comments or as a CI artifact for visibility.
