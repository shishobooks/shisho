package books

import (
	"net/http"
	"os"

	"github.com/labstack/echo/v4"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
)

// RequireFileOnDisk returns a 404 naming resource when the file's path is
// missing from disk. The message stays generic for the client, and the file ID
// and path go to the server log. HEAD requests skip the log because the web
// download button sends one before every GET, so the GET logs it once.
//
// Use it in every handler that serves a file's bytes instead of an inline
// os.Stat, so a missing file gets the same 404 and log line on each route.
func RequireFileOnDisk(c echo.Context, file *models.File, resource string) error {
	if _, err := os.Stat(file.Filepath); os.IsNotExist(err) {
		if c.Request().Method != http.MethodHead {
			logger.FromContext(c.Request().Context()).Warn("file not found on disk", logger.Data{"file_id": file.ID, "path": file.Filepath})
		}
		return errcodes.NotFound(resource)
	}
	return nil
}
