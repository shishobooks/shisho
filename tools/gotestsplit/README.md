# gotestsplit

Timing-aware Go test splitter. Forked from [gotesplit](https://github.com/Songmu/gotesplit) (MIT, Songmu).

Where gotesplit splits by test count, gotestsplit reads prior JUnit XML output
to bin-pack by measured wallclock time, and chunks hot packages across shards
using `-run` regexes.

## Usage

```
gotestsplit simulate -junit-dir=.gotest-timings [-min=2] [-max=10]
gotestsplit plan -junit-dir=.gotest-timings -total=8 -index=0 ./pkg/...
gotestsplit run -junit-dir=.gotest-timings -total=8 -index=0 ./pkg/... -- -race
```

See `tools/gotestsplit/CLAUDE.md` for design rationale and architecture details.
