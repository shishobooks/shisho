# M4B audio playback in the browser

Read before changing the M4B player (`app/components/pages/M4BReader.tsx`), the chapter audio preview (`app/components/files/FileChaptersTab.tsx`), or `app/utils/audioCodec.ts`, which holds the codec decisions and is unit-tested.

## The xHE-AAC trap

Some M4B files use xHE-AAC. Outside WebKit (Safari and every iOS browser), the browser answers `canPlayType('audio/mp4; codecs="mp4a.40.42"')` with "probably" and then hangs on any seek: `seeking` stays true, `readyState` stays at 1 (HAVE_METADATA), and `canplay` never fires. Nothing throws, so playback code that waits only on events hangs forever. AAC-LC and HE-AAC play and seek normally. Check a file with `ffprobe -v error -show_streams <file.m4b> | grep profile`.

## Rules

- **Never call `play()` before `readyState >= 3`** (HAVE_FUTURE_DATA). Wait for `canplay`, and put a timeout on every wait for `seeked` or `canplay`. When a timeout fires, remove its listeners, reset the playing state so the control stops showing "playing", and tell the user the codec may be unsupported.
- **The player detects the problem in three layers:** `resolveCodecSupport(file.audiobook_codec, userAgent)` warns before playback (a warning only, the controls stay usable); the `error` event (filtered by `shouldIgnoreMediaError`) and `stalled` at `readyState <= HAVE_METADATA` set a runtime-failure flag; and every seek goes through the one `seekTo` path, which arms `SEEK_TIMEOUT_MS`.
- **`canplay` clears the runtime-failure flag.** The stall signal is a guess, and a slow first load of a playable file can trip it. A stream that truly cannot be decoded never fires `canplay`, so real failures still stick.
- Users see this caveat in `website/docs/supported-formats.md` (the xHE-AAC note under Audiobooks). Keep it in sync with player behavior.
