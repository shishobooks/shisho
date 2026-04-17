package logs

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
)

type handler struct {
	buffer *RingBuffer
}

func (h *handler) listLogs(c echo.Context) error {
	params := ListLogsQuery{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	level := ""
	if params.Level != nil {
		level = *params.Level
	}
	search := ""
	if params.Search != nil {
		search = *params.Search
	}
	limit := 100
	if params.Limit != nil {
		limit = *params.Limit
	}
	var afterID uint64
	if params.AfterID != nil {
		afterID = *params.AfterID
	}

	entries := h.buffer.Query(level, search, limit, afterID)

	resp := struct {
		Entries []LogEntry `json:"entries"`
	}{Entries: entries}

	return errors.WithStack(c.JSON(http.StatusOK, resp))
}
