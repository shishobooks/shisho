# gotestsplit

Timing-aware Go test splitter used by the `test` job in `.github/workflows/ci.yml`, forked from gotesplit (MIT). It bin-packs tests across N shards by measured per-test wallclock read from JUnit history and splits hot packages across shards with `-run` regexes.

## Design rules

- **`Pack` (`pack.go`) is the single planner** for every subcommand, so their plans agree. A new subcommand that plans shards calls it rather than reimplementing packing.
- The tool stays extractable into its own repo; depguard in `.golangci.yml` limits its imports.

## Changing the shard count

Change `env.SHARD_TOTAL`, the `matrix.shard` list, and the job `name` (env is not available there) together. The first run at a new N plans from a cache shaped by the old N, so its timing is contaminated.

To pick N:

1. Confirm the latest master run's "Consolidate test timings" job found every shard artifact and saved the cache, then download any shard's artifact (it contains the full restored cache): `gh run download $RUN -n gotest-timings-shard-1 -D /tmp/ci-junit`.
2. `go run ./tools/gotestsplit simulate -junit-dir=/tmp/ci-junit -min=2 -max=14`. Simulated `slowest` overstates wallclock because parallel tests within a package overlap, so read only the relative shape: find the knee where `slowest` flattens and `cost` (slowest x N) starts climbing.
3. Validate two or three candidates in real CI. Each needs a first push at that N plus an `--allow-empty` second push; measure only the second. Runner variance on the slowest shard is about 2 minutes, so take three or more samples and use the median.
