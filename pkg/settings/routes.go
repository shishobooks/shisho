package settings

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/appsettings"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/jobs"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

func RegisterRoutes(e *echo.Group, db *bun.DB, authMiddleware *auth.Middleware, appSettingsSvc *appsettings.Service) {
	svc := NewService(db)

	userH := &handler{settingsService: svc}
	libraryH := &libraryHandler{settingsService: svc}
	reviewCriteriaH := &reviewCriteriaHandler{
		db:                 db,
		appSettingsService: appSettingsSvc,
		jobService:         jobs.NewService(db),
	}

	g := e.Group("/settings")
	g.Use(authMiddleware.Authenticate)

	g.GET("/user", userH.getUserSettings)
	g.PUT("/user", userH.updateUserSettings)

	g.GET("/libraries/:library_id", libraryH.getLibrarySettings)
	g.PUT("/libraries/:library_id", libraryH.updateLibrarySettings)

	// GET allows books:read so the ReviewPanel can render the missing-fields
	// hint for any user who can view books (editors and viewers, not just
	// admins), and config:read so the review criteria settings page loads for
	// any role that can open it. PUT remains admin-only via config:write.
	g.GET("/review-criteria", reviewCriteriaH.getReviewCriteria, authMiddleware.RequireAnyPermission(
		auth.Permission{Resource: models.ResourceBooks, Operation: models.OperationRead},
		auth.Permission{Resource: models.ResourceConfig, Operation: models.OperationRead},
	))
	g.PUT("/review-criteria", reviewCriteriaH.putReviewCriteria, authMiddleware.RequirePermission(models.ResourceConfig, models.OperationWrite))
}
