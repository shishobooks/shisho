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
	sharingH := &sharingHandler{appSettingsService: appSettingsSvc}

	g := e.Group("/settings")
	g.Use(authMiddleware.Authenticate)

	g.GET("/user", userH.getUserSettings)
	g.PUT("/user", userH.updateUserSettings)

	g.GET("/libraries/:library_id", libraryH.getLibrarySettings)
	g.PUT("/libraries/:library_id", libraryH.updateLibrarySettings)

	// GET is books:read so the ReviewPanel can render the missing-fields hint
	// for any user who can view books (editors and viewers, not just admins).
	// PUT remains admin-only via config:write.
	g.GET("/review-criteria", reviewCriteriaH.getReviewCriteria, authMiddleware.RequirePermission(models.ResourceBooks, models.OperationRead))
	g.PUT("/review-criteria", reviewCriteriaH.putReviewCriteria, authMiddleware.RequirePermission(models.ResourceConfig, models.OperationWrite))

	// GET allows shares:read so sharers can read the policy that shapes the
	// Share Link form, and config:read so admins can render the Sharing page.
	g.GET("/sharing", sharingH.getSharingSettings, authMiddleware.RequireAnyPermission(models.OperationRead, models.ResourceShares, models.ResourceConfig))
	g.PUT("/sharing", sharingH.updateSharingSettings, authMiddleware.RequirePermission(models.ResourceConfig, models.OperationWrite))
}
