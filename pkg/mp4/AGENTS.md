# M4B (MP4) Format

`pkg/mp4` reads and rewrites M4B audiobooks; `pkg/filegen/m4b.go` (`M4BGenerator`) builds downloads through `ParseFullContext` and `WriteToFileContext` so request cancellation reaches parsing and the write. The atom-to-field mapping and fallback order live in `reader.go`, `atoms.go`, and `metadata.go`. Inspect a real file with `go run ./cmd/scripts/debug/print-mp4-atoms <file.m4b>`. Chapter lookup from Audible is `pkg/audnexus`.

## Reading

- **Narrators fall back `©nrt`, then `©cmp` (common in ffmpeg output), then `©wrt`.** The writer puts narrators in both `©nrt` and `©cmp` for player compatibility.
- **Data type 18 holds UTF-8 genre text** in some files; treat it like type 1.
- **Series comes from the `com.apple.iTunes:SERIES` / `SERIES-PART` freeform atoms first, then `©grp`.** A blank freeform `SERIES` must not mask a valid grouping. Grouping keywords (`Book`, `Volume`) need token boundaries, so labels like `Booker Prize` are not series. Numbers go through `seriesnum.ParseRange`; a malformed range is dropped as a whole while the series name is kept. M4B never sets the series unit.
- **Chapters prefer the QuickTime text track (`tref/chap`) over Nero `chpl`**, in `readChapters`. Players and ffprobe do the same, which is why the writer must rebuild both (below).

## Writing

The writer always sets `©alb` to the book title and mirrors `desc` into `©cmt`. Unknown atoms (`aART`, `cprt`, unrecognized freeform atoms) round-trip byte for byte; keep that when adding fields.

### Bounded-memory rewrite

`rewriteToFile` (`writer.go`) inspects top-level box headers through `io.ReaderAt` (including 64-bit `largesize`) without reading payloads, rebuilds only `moov` plus modified metadata, cover, and chapter samples, and copies every unchanged box, above all the audio `mdat`, through a fixed 64 KiB buffer. Peak memory must scale with the modified structures, never the audiobook size; `writer_streaming_test.go` fails if total allocation for a sparse 64 MiB `mdat` exceeds 8 MiB.

- The destination is a unique temporary file beside it, removed on error or cancellation, synced and renamed on success. Unique names stop concurrent generations sharing an inode or following a planted symlink. The source size and mtime are rechecked after copying so a source changed mid-copy is not published.
- `Write` (in place) resolves symlinks and keeps the source file mode; `WriteToFile`/`WriteToFileContext` create download output at 0644 whatever the source mode.
- Limits against malformed files: `moov` at most 256 MiB, at most 100,000 top-level boxes, box offsets and sizes bounds-checked against the file (`inspectTopLevelBoxes`) and nested sizes against the `moov` buffer (`shiftChunkOffsetsInChildren`).

### Chunk offsets move when `moov` grows (faststart)

`stco`/`co64` hold absolute file offsets into `mdat`. When `moov` precedes the first `mdat` (faststart: Audible, Apple Books, ffmpeg `-movflags +faststart`), a rebuilt `moov` of a different size shifts `mdat`, so every `stco`/`co64` entry under `moov > trak > mdia > minf > stbl` (and `edts`) is shifted by the size delta. Without it the decoder reads metadata as audio and strict players (Apple Books, Bound) refuse the file. In `mdat`-first layout nothing moves and nothing is shifted. A `stco` entry that would overflow `uint32`, or a `co64` entry that would overflow or go negative, returns an error instead of wrapping.

Test fixtures must opt in with `testgen.GenerateM4B`'s `Faststart` option; ffmpeg's default output is `mdat`-first and cannot reproduce the bug (`writer_chunkoffset_test.go`).

### Chapters are written to both the QuickTime track and `chpl`

Writing only `chpl` leaves edited chapters masked by the stale QuickTime track that readers prefer. `rebuildChapterTextTrack` (`writer_chapters.go`) therefore rebuilds the existing text track from `metadata.Chapters`:

- It finds the track the way the reader does: the audio track's `tref/chap` id matched against `tkhd`, falling back to the first `hdlr` == `text` track. Matching by id keeps reader and writer agreed when a file has several text tracks.
- `tkhd`, `edts`, `mdhd`, `hdlr`, and `stsd` are kept verbatim so the track id and the `tref/chap` reference stay valid; only `stts`/`stsc`/`stsz` are regenerated, and the chunk offset is a `co64`.
- Audio sample bytes are never modified. New samples (`[uint16 len][utf8 title][12-byte encd atom]`, as ffmpeg and Apple write them) go into a new trailing `mdat`, or are appended to a final size-zero `mdat` so boxes over 4 GiB stay streamable. Old chapter samples remain as unreferenced bytes.
- The chapter `co64` holds a placeholder during the faststart shift (so the shift cannot underflow it) and is then set to the absolute sample offset; audio offsets move only by the `moov` delta.
- A file with no QuickTime chapter track keeps the `chpl`-only path (`ok=false`). Synthesizing a new track would mean editing the audio `tref` and `mvhd` next-track-id and is out of scope. Stale `tkhd`/`mdhd` durations are cosmetic; players use `stts`.

### Series atoms

`©grp` is `Series #N` or `Series #N-M` (decimals allowed); `SERIES-PART` carries the same number without the name. An invalid number group is omitted from both atoms while the name is still written.
