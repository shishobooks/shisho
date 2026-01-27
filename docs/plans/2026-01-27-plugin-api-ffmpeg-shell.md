# Plugin API: FFmpeg Enhancements and Shell Exec

## Overview

Enhance the plugin FFmpeg API and add a new shell execution capability:

1. **FFmpeg API changes**: Rename `run()` to `transcode()`, add `probe()` for metadata extraction, add `version()` for querying FFmpeg capabilities
2. **Shell exec capability**: New `shisho.shell.exec()` API with command allowlist security model

## FFmpeg API Changes

### API Surface

```typescript
interface ShishoFFmpeg {
  // Renamed from run(). Executes ffmpeg for transcoding.
  // Prepends -protocol_whitelist file,pipe for security.
  transcode(args: string[]): TranscodeResult;

  // Executes ffprobe with auto-parsed JSON output.
  // Automatically adds -print_format json to args.
  probe(args: string[]): ProbeResult;

  // Returns ffmpeg version and configuration.
  version(): VersionResult;
}

interface TranscodeResult {
  exitCode: number;
  stdout: string;
  stderr: string;
}

interface VersionResult {
  version: string;                    // e.g., "8.0"
  configuration: string[];            // e.g., ["--enable-libx264", "--enable-gpl", ...]
  libraries: Record<string, string>;  // e.g., { libavcodec: "62.11.100", ... }
}

interface ProbeResult {
  format: ProbeFormat;
  streams: ProbeStream[];
  chapters: ProbeChapter[];
  stderr: string;  // for debugging
}
```

### ProbeResult Types

Full typing of ffprobe JSON output:

```typescript
interface ProbeFormat {
  filename: string;
  nb_streams: number;
  nb_programs: number;
  format_name: string;
  format_long_name: string;
  start_time: string;
  duration: string;
  size: string;
  bit_rate: string;
  probe_score: number;
  tags?: Record<string, string>;
}

interface ProbeStream {
  index: number;
  codec_name: string;
  codec_long_name: string;
  codec_type: "video" | "audio" | "subtitle" | "data" | "attachment";
  codec_tag_string: string;
  codec_tag: string;

  // Video-specific
  width?: number;
  height?: number;
  coded_width?: number;
  coded_height?: number;
  closed_captions?: number;
  has_b_frames?: number;
  sample_aspect_ratio?: string;
  display_aspect_ratio?: string;
  pix_fmt?: string;
  level?: number;
  color_range?: string;
  color_space?: string;
  color_transfer?: string;
  color_primaries?: string;
  chroma_location?: string;
  field_order?: string;
  refs?: number;

  // Audio-specific
  sample_fmt?: string;
  sample_rate?: string;
  channels?: number;
  channel_layout?: string;
  bits_per_sample?: number;

  // Common
  r_frame_rate: string;
  avg_frame_rate: string;
  time_base: string;
  start_pts?: number;
  start_time?: string;
  duration_ts?: number;
  duration?: string;
  bit_rate?: string;
  bits_per_raw_sample?: string;
  nb_frames?: string;
  disposition: ProbeDisposition;
  tags?: Record<string, string>;
}

interface ProbeDisposition {
  default: number;
  dub: number;
  original: number;
  comment: number;
  lyrics: number;
  karaoke: number;
  forced: number;
  hearing_impaired: number;
  visual_impaired: number;
  clean_effects: number;
  attached_pic: number;
  timed_thumbnails: number;
}

interface ProbeChapter {
  id: number;
  time_base: string;
  start: number;
  start_time: string;
  end: number;
  end_time: string;
  tags?: Record<string, string>;
}
```

### Backward Compatibility

**Breaking change**: `shisho.ffmpeg.run()` is removed. Plugins must update to `shisho.ffmpeg.transcode()`.

### Capability

All three methods (`transcode`, `probe`, `version`) require the existing `ffmpegAccess` capability:

```json
{
  "capabilities": {
    "ffmpegAccess": {
      "description": "Transcode audio files and probe metadata"
    }
  }
}
```

## Shell Exec Capability

### Manifest Declaration

```json
{
  "capabilities": {
    "shellAccess": {
      "description": "Run ImageMagick for image processing",
      "commands": ["convert", "magick", "identify"]
    }
  }
}
```

### API Surface

```typescript
interface ShishoShell {
  // Execute an allowed command with arguments.
  // Command must be declared in manifest shellAccess.commands.
  // Uses exec directly (no shell expansion) to prevent injection.
  exec(command: string, args: string[]): ExecResult;
}

interface ExecResult {
  exitCode: number;
  stdout: string;
  stderr: string;
}
```

### Security Model

- Commands are validated against the manifest allowlist before execution
- Uses `exec.Command()` directly (not `sh -c`), preventing shell injection
- Arguments are passed directly to the command with no shell expansion
- Same timeout as ffmpeg (5 minutes default)

### Example Usage

```javascript
var result = shisho.shell.exec("convert", [
  "input.png", "-resize", "50%", "output.png"
]);
if (result.exitCode !== 0) {
  shisho.log.error("convert failed: " + result.stderr);
}
```

## Implementation

### Files to Create

| File | Purpose |
|------|---------|
| `pkg/plugins/hostapi_shell.go` | `injectShellNamespace()` with `exec(command, args)` |
| `pkg/plugins/hostapi_shell_test.go` | Tests for shell exec capability |

### Files to Modify

| File | Changes |
|------|---------|
| `pkg/plugins/hostapi_ffmpeg.go` | Rename `run`→`transcode`, add `probe()`, add `version()`, add `ffprobeBinary` var |
| `pkg/plugins/hostapi_ffmpeg_test.go` | Update all tests for new API |
| `pkg/plugins/manifest.go` | Add `ShellAccessCap` struct with `Commands []string` |
| `pkg/plugins/hostapi.go` | Add `injectShellNamespace()` call |
| `packages/plugin-types/host-api.d.ts` | Update `ShishoFFmpeg`, add `ShishoShell`, add all result types |
| `packages/plugin-types/manifest.d.ts` | Add `ShellAccessCap` interface |
