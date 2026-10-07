# M4B (MP4) Format

`pkg/mp4` reads and rewrites M4B audiobooks; `pkg/filegen/m4b.go` builds downloads with it. Inspect a real file with `go run ./cmd/scripts/debug/print-mp4-atoms <file.m4b>` rather than guessing at its atoms.

- **Unknown atoms round-trip byte for byte.** Keep that when adding fields: write only the atoms Shisho models and copy the rest.
- **Rewrites stay bounded in memory.** Copy unchanged boxes, above all the audio `mdat`, through the streaming path in `rewriteToFile`; never read a whole file or `mdat` into memory.
- **Any change that resizes `moov` must keep faststart files playable**: chunk offsets shift with it (see the comment in `rewriteToFile`). Tests for writer changes need `testgen.GenerateM4B`'s `Faststart` option, because ffmpeg's default `mdat`-first output never moves `mdat` and hides the bug.
- **Chapters live in two places**, the QuickTime text track (which readers and players prefer) and Nero `chpl`. Changing how chapters are read or written means changing both.
