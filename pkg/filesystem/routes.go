package filesystem

import (
	"github.com/labstack/echo/v4"
)

func RegisterRoutes(e *echo.Echo) {
	filesystemService := NewService()

	h := &handler{
		filesystemService: filesystemService,
	}

	e.GET("/filesystem/browse", h.browse)
}
