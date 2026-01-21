package testgen

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// FFmpegAvailable checks if ffmpeg is available on the system.
func FFmpegAvailable() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}

// SkipIfNoFFmpeg skips the test if ffmpeg is not available.
func SkipIfNoFFmpeg(t *testing.T) {
	t.Helper()
	if !FFmpegAvailable() {
		t.Skip("ffmpeg not available, skipping M4B test")
	}
}

// GenerateM4B creates a valid M4B file at the specified path with the given options.
// This function requires ffmpeg to be installed. Use SkipIfNoFFmpeg() to skip
// tests when ffmpeg is not available. The generated M4B contains a short audio
// track, iTunes-style metadata, and optionally a cover image.
func GenerateM4B(t *testing.T, dir, filename string, opts M4BOptions) string {
	t.Helper()
	SkipIfNoFFmpeg(t)

	path := filepath.Join(dir, filename)

	// Set defaults
	duration := opts.Duration
	if duration <= 0 {
		duration = 1.0 // 1 second default
	}

	// Build ffmpeg command
	// Note: ffmpeg is sensitive to option order - inputs first, then mapping, then output options
	args := []string{
		"-f", "lavfi",
		"-i", "sine=frequency=440:duration=" + strconv.FormatFloat(duration, 'f', 1, 64),
	}

	// Add cover image input if requested (must be before output options)
	var hasCover bool
	if opts.HasCover {
		// Generate a temporary cover image
		coverPath := filepath.Join(dir, "temp_cover.png")
		coverData := generateImage(t, "image/png")
		if err := os.WriteFile(coverPath, coverData, 0600); err != nil {
			t.Fatalf("failed to write temp cover: %v", err)
		}
		defer os.Remove(coverPath)

		args = append(args, "-i", coverPath)
		hasCover = true
	}

	// Add chapters via ffmetadata file if requested
	var hasChapters bool
	if len(opts.Chapters) > 0 {
		metadataPath := filepath.Join(dir, "ffmetadata.txt")
		metadataContent := buildFFMetadata(opts.Chapters, duration)
		if err := os.WriteFile(metadataPath, []byte(metadataContent), 0600); err != nil {
			t.Fatalf("failed to write ffmetadata file: %v", err)
		}
		defer os.Remove(metadataPath)

		args = append(args, "-i", metadataPath)
		hasChapters = true
	}

	// Now add output options (after all inputs)
	args = append(args, "-y") // Overwrite output file

	// Add metadata (output options)
	if opts.Title != "" {
		args = append(args, "-metadata", "title="+opts.Title)
	}
	if opts.Artist != "" {
		args = append(args, "-metadata", "artist="+opts.Artist)
	}
	if opts.Album != "" {
		args = append(args, "-metadata", "album="+opts.Album)
	}
	if opts.Composer != "" {
		args = append(args, "-metadata", "composer="+opts.Composer)
	}
	if opts.Genre != "" {
		args = append(args, "-metadata", "genre="+opts.Genre)
	}
	if opts.Copyright != "" {
		args = append(args, "-metadata", "copyright="+opts.Copyright)
	}
	if opts.Date != "" {
		args = append(args, "-metadata", "date="+opts.Date)
	}
	if opts.AlbumArtist != "" {
		args = append(args, "-metadata", "album_artist="+opts.AlbumArtist)
	}
	if opts.Comment != "" {
		args = append(args, "-metadata", "comment="+opts.Comment)
	}

	// Add mapping if we have cover or chapters
	if hasCover || hasChapters {
		// Map audio from input 0
		args = append(args, "-map", "0:a")

		// Map video (cover) if present
		if hasCover {
			coverInput := 1
			args = append(args,
				"-map", strconv.Itoa(coverInput)+":v",
				"-c:v", "png",
				"-disposition:v:0", "attached_pic",
			)
		}

		// Map chapters metadata if present
		if hasChapters {
			chaptersInput := 1
			if hasCover {
				chaptersInput = 2
			}
			args = append(args, "-map_metadata", strconv.Itoa(chaptersInput))
		}
	}

	// Output settings for M4B (AAC audio in MP4 container)
	args = append(args,
		"-c:a", "aac",
		"-b:a", "64k",
		path,
	)

	cmd := exec.CommandContext(t.Context(), "ffmpeg", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ffmpeg failed: %v\nOutput: %s", err, output)
	}

	return path
}

// ffprobeFormat represents the format section of ffprobe JSON output.
type ffprobeFormat struct {
	Tags map[string]string `json:"tags"`
}

// ffprobeOutput represents the ffprobe JSON output structure.
type ffprobeOutput struct {
	Format ffprobeFormat `json:"format"`
}

// GetM4BTags reads the metadata tags from an M4B file using ffprobe.
// Returns a map of tag names to values. This is useful for verifying
// that tags were correctly written to files.
func GetM4BTags(t *testing.T, path string) map[string]string {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		path,
	)
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("ffprobe failed for %s: %v", path, err)
	}

	var result ffprobeOutput
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("failed to parse ffprobe output: %v", err)
	}

	if result.Format.Tags == nil {
		return make(map[string]string)
	}
	return result.Format.Tags
}

// buildFFMetadata creates an ffmetadata format string with chapters.
// See https://ffmpeg.org/ffmpeg-formats.html#Metadata-1 for format specification.
func buildFFMetadata(chapters []M4BChapter, totalDuration float64) string {
	var content strings.Builder
	content.WriteString(";FFMETADATA1\n")

	for i, ch := range chapters {
		// Calculate end time (next chapter start or total duration)
		var end float64
		if i+1 < len(chapters) {
			end = chapters[i+1].Start
		} else {
			end = totalDuration
		}

		// Times are in milliseconds for ffmetadata TIMEBASE=1/1000
		// Using nanoseconds (TIMEBASE=1/1000000000) gives more precision
		startNs := int64(ch.Start * 1e9)
		endNs := int64(end * 1e9)

		content.WriteString("[CHAPTER]\n")
		content.WriteString("TIMEBASE=1/1000000000\n")
		content.WriteString("START=")
		content.WriteString(strconv.FormatInt(startNs, 10))
		content.WriteString("\n")
		content.WriteString("END=")
		content.WriteString(strconv.FormatInt(endNs, 10))
		content.WriteString("\n")
		content.WriteString("title=")
		content.WriteString(ch.Title)
		content.WriteString("\n")
	}

	return content.String()
}
