package books

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// RequireFileOnDisk warn-logs the file ID and path when a GET finds the file
// missing, stays quiet on HEAD, and returns nil when the file exists.
//
// Not parallel: it swaps the process-wide logger output to capture the line.
func TestRequireFileOnDisk(t *testing.T) {
	t.Setenv("LOG_LEVEL", "")
	var buf bytes.Buffer
	prev := logger.Output()
	logger.SetOutput(&buf)
	t.Cleanup(func() { logger.SetOutput(prev) })

	dir := t.TempDir()
	present := filepath.Join(dir, "present.epub")
	require.NoError(t, os.WriteFile(present, []byte("epub"), 0o644))
	missing := filepath.Join(dir, "missing.epub")

	tests := []struct {
		name    string
		method  string
		path    string
		wantErr bool
		wantLog bool
	}{
		{"missing GET", http.MethodGet, missing, true, true},
		{"missing HEAD", http.MethodHead, missing, true, false},
		{"present GET", http.MethodGet, present, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf.Reset()
			c := echo.New().NewContext(httptest.NewRequest(tt.method, "/", nil), httptest.NewRecorder())
			file := &models.File{ID: 42, Filepath: tt.path}

			err := RequireFileOnDisk(c, file, "Source file")

			if tt.wantErr {
				var codeErr *errcodes.Error
				require.ErrorAs(t, err, &codeErr)
				assert.Equal(t, http.StatusNotFound, codeErr.HTTPCode)
				assert.Equal(t, "Source file not found.", codeErr.Message)
			} else {
				require.NoError(t, err)
			}

			type logLine struct {
				Level   string         `json:"level"`
				Message string         `json:"message"`
				Data    map[string]any `json:"data"`
			}
			var lines []logLine
			scanner := bufio.NewScanner(bytes.NewReader(buf.Bytes()))
			for scanner.Scan() {
				var line logLine
				require.NoError(t, json.Unmarshal(scanner.Bytes(), &line))
				lines = append(lines, line)
			}

			if !tt.wantLog {
				assert.Empty(t, lines)
				return
			}
			require.Len(t, lines, 1)
			assert.Equal(t, "warn", lines[0].Level)
			assert.Equal(t, "file not found on disk", lines[0].Message)
			assert.InDelta(t, 42, lines[0].Data["file_id"], 0)
			assert.Equal(t, tt.path, lines[0].Data["path"])
		})
	}
}
