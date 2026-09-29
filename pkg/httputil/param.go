package httputil

import (
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/errcodes"
)

// ParamID parses the numeric path parameter name as a row ID. A value that is
// not a positive integer names no row, so it returns errcodes.NotFound(resource),
// the same 404 as a well-formed ID with no row behind it.
func ParamID(c echo.Context, name, resource string) (int, error) {
	id, err := strconv.Atoi(c.Param(name))
	if err != nil || id < 1 {
		return 0, errcodes.NotFound(resource)
	}
	return id, nil
}
