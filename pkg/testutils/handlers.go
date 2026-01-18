package testutils

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/apikeys"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

type handler struct {
	db *bun.DB
}

// createUserRequest is the request body for creating a test user.
type createUserRequest struct {
	Username string  `json:"username" validate:"required"`
	Password string  `json:"password" validate:"required"`
	Email    *string `json:"email"`
}

// createUserResponse is the response body for creating a test user.
type createUserResponse struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
}

// createUser creates a test user with admin role.
// POST /test/users.
func (h *handler) createUser(c echo.Context) error {
	ctx := c.Request().Context()

	var req createUserRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	if req.Username == "" || req.Password == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "Username and password are required")
	}

	// Get admin role
	role := &models.Role{}
	err := h.db.NewSelect().
		Model(role).
		Where("name = ?", models.RoleAdmin).
		Scan(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get admin role")
	}

	// Hash password
	hashedPassword, err := auth.HashPassword(req.Password)
	if err != nil {
		return errors.Wrap(err, "failed to hash password")
	}

	// Create user
	user := &models.User{
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: hashedPassword,
		RoleID:       role.ID,
		IsActive:     true,
	}

	_, err = h.db.NewInsert().Model(user).Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create user")
	}

	// Grant access to all libraries
	access := &models.UserLibraryAccess{
		UserID:    user.ID,
		LibraryID: nil, // null = all libraries
	}
	_, err = h.db.NewInsert().Model(access).Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to grant library access")
	}

	return c.JSON(http.StatusCreated, createUserResponse{
		ID:       user.ID,
		Username: user.Username,
	})
}

// deleteAllUsersResponse is the response body for deleting all users.
type deleteAllUsersResponse struct {
	Deleted int `json:"deleted"`
}

// deleteAllUsers deletes all users from the database.
// DELETE /test/users.
func (h *handler) deleteAllUsers(c echo.Context) error {
	ctx := c.Request().Context()

	// Delete library access first (foreign key constraint)
	_, err := h.db.NewDelete().
		Model((*models.UserLibraryAccess)(nil)).
		Where("1=1").
		Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to delete library access")
	}

	// Delete all users
	result, err := h.db.NewDelete().
		Model((*models.User)(nil)).
		Where("1=1").
		Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to delete users")
	}

	deleted, _ := result.RowsAffected()

	return c.JSON(http.StatusOK, deleteAllUsersResponse{
		Deleted: int(deleted),
	})
}

// createLibraryRequest is the request body for creating a test library.
type createLibraryRequest struct {
	Name string `json:"name" validate:"required"`
}

// createLibraryResponse is the response body for creating a test library.
type createLibraryResponse struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// createLibrary creates a test library.
// POST /test/libraries.
func (h *handler) createLibrary(c echo.Context) error {
	ctx := c.Request().Context()

	var req createLibraryRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	if req.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "Name is required")
	}

	now := time.Now()
	library := &models.Library{
		Name:                     req.Name,
		CreatedAt:                now,
		UpdatedAt:                now,
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}

	_, err := h.db.NewInsert().Model(library).Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create library")
	}

	return c.JSON(http.StatusCreated, createLibraryResponse{
		ID:   library.ID,
		Name: library.Name,
	})
}

// createBookRequest is the request body for creating a test book.
type createBookRequest struct {
	LibraryID int    `json:"libraryId" validate:"required"`
	Title     string `json:"title" validate:"required"`
	Filepath  string `json:"filepath"`
	FileType  string `json:"fileType"` // "epub", "cbz", "m4b"
	AuthorID  *int   `json:"authorId"` // Optional author
	SeriesID  *int   `json:"seriesId"` // Optional series
	FileSize  *int64 `json:"fileSize"` // Optional file size in bytes
	PageCount *int   `json:"pageCount"`
}

// createBookResponse is the response body for creating a test book.
type createBookResponse struct {
	ID     int `json:"id"`
	FileID int `json:"fileId"`
}

// createBook creates a test book with a file.
// POST /test/books.
func (h *handler) createBook(c echo.Context) error {
	ctx := c.Request().Context()

	var req createBookRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	if req.LibraryID == 0 || req.Title == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "libraryId and title are required")
	}

	now := time.Now()

	// Default filepath and file type
	filepath := req.Filepath
	if filepath == "" {
		filepath = "/test/books/" + req.Title
	}
	fileType := req.FileType
	if fileType == "" {
		fileType = models.FileTypeEPUB
	}

	// Create book
	book := &models.Book{
		LibraryID:       req.LibraryID,
		Title:           req.Title,
		SortTitle:       req.Title,
		Filepath:        filepath,
		TitleSource:     "test",
		SortTitleSource: "test",
		AuthorSource:    "test",
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	_, err := h.db.NewInsert().Model(book).Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create book")
	}

	// Create file
	fileSize := int64(1024) // Default 1KB
	if req.FileSize != nil {
		fileSize = *req.FileSize
	}

	file := &models.File{
		LibraryID:     req.LibraryID,
		BookID:        book.ID,
		Filepath:      filepath + "." + fileType,
		FileType:      fileType,
		FileRole:      models.FileRoleMain,
		FilesizeBytes: fileSize,
		PageCount:     req.PageCount,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	_, err = h.db.NewInsert().Model(file).Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create file")
	}

	// Link author if provided
	if req.AuthorID != nil {
		author := &models.Author{
			BookID:    book.ID,
			PersonID:  *req.AuthorID,
			SortOrder: 1, // Must be non-zero due to bun nullzero tag
		}
		_, err = h.db.NewInsert().Model(author).Exec(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to link author")
		}
	}

	// Link series if provided
	if req.SeriesID != nil {
		bookSeries := &models.BookSeries{
			BookID:    book.ID,
			SeriesID:  *req.SeriesID,
			SortOrder: 1, // Must be non-zero due to bun nullzero tag
		}
		_, err = h.db.NewInsert().Model(bookSeries).Exec(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to link series")
		}
	}

	// Index book in FTS for search functionality
	_, err = h.db.ExecContext(ctx,
		`INSERT INTO books_fts (book_id, library_id, title, filepath, subtitle, authors, filenames, narrators, series_names)
		 VALUES (?, ?, ?, ?, '', '', '', '', '')`,
		book.ID,
		book.LibraryID,
		book.Title,
		filepath,
	)
	if err != nil {
		return errors.Wrap(err, "failed to index book in FTS")
	}

	return c.JSON(http.StatusCreated, createBookResponse{
		ID:     book.ID,
		FileID: file.ID,
	})
}

// createPersonRequest is the request body for creating a test person (author).
type createPersonRequest struct {
	LibraryID int    `json:"libraryId" validate:"required"`
	Name      string `json:"name" validate:"required"`
}

// createPersonResponse is the response body for creating a test person.
type createPersonResponse struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// createPerson creates a test person (can be used as author).
// POST /test/persons.
func (h *handler) createPerson(c echo.Context) error {
	ctx := c.Request().Context()

	var req createPersonRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	if req.LibraryID == 0 || req.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "libraryId and name are required")
	}

	now := time.Now()
	person := &models.Person{
		LibraryID:      req.LibraryID,
		Name:           req.Name,
		SortName:       req.Name,
		SortNameSource: "test",
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	_, err := h.db.NewInsert().Model(person).Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create person")
	}

	return c.JSON(http.StatusCreated, createPersonResponse{
		ID:   person.ID,
		Name: person.Name,
	})
}

// createSeriesRequest is the request body for creating a test series.
type createSeriesRequest struct {
	LibraryID int    `json:"libraryId" validate:"required"`
	Name      string `json:"name" validate:"required"`
}

// createSeriesResponse is the response body for creating a test series.
type createSeriesResponse struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// createSeries creates a test series.
// POST /test/series.
func (h *handler) createSeries(c echo.Context) error {
	ctx := c.Request().Context()

	var req createSeriesRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	if req.LibraryID == 0 || req.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "libraryId and name are required")
	}

	now := time.Now()
	series := &models.Series{
		LibraryID:      req.LibraryID,
		Name:           req.Name,
		SortName:       req.Name,
		NameSource:     "test",
		SortNameSource: "test",
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	_, err := h.db.NewInsert().Model(series).Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create series")
	}

	return c.JSON(http.StatusCreated, createSeriesResponse{
		ID:   series.ID,
		Name: series.Name,
	})
}

// createAPIKeyRequest is the request body for creating a test API key.
type createAPIKeyRequest struct {
	UserID      int      `json:"userId" validate:"required"`
	Name        string   `json:"name" validate:"required"`
	Permissions []string `json:"permissions"` // e.g., ["ereader_browser"]
}

// createAPIKeyResponse is the response body for creating a test API key.
type createAPIKeyResponse struct {
	ID          string   `json:"id"`
	Key         string   `json:"key"`
	Permissions []string `json:"permissions"`
}

// createAPIKey creates a test API key with permissions.
// POST /test/api-keys.
func (h *handler) createAPIKey(c echo.Context) error {
	ctx := c.Request().Context()

	var req createAPIKeyRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	if req.UserID == 0 || req.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "userId and name are required")
	}

	now := time.Now()

	// Generate API key
	keyBytes := make([]byte, 32)
	for i := range keyBytes {
		keyBytes[i] = byte(i + 65) // Simple deterministic key for testing
	}
	key := "ak_test_" + uuid.New().String()[:8]

	apiKey := &apikeys.APIKey{
		ID:        uuid.New().String(),
		UserID:    req.UserID,
		Name:      req.Name,
		Key:       key,
		CreatedAt: now,
		UpdatedAt: now,
	}

	_, err := h.db.NewInsert().Model(apiKey).Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create api key")
	}

	// Add permissions (only if provided)
	for _, perm := range req.Permissions {
		permission := &apikeys.APIKeyPermission{
			ID:         uuid.New().String(),
			APIKeyID:   apiKey.ID,
			Permission: perm,
			CreatedAt:  now,
		}
		_, err = h.db.NewInsert().Model(permission).Exec(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to create permission")
		}
	}

	return c.JSON(http.StatusCreated, createAPIKeyResponse{
		ID:          apiKey.ID,
		Key:         key,
		Permissions: req.Permissions,
	})
}

// deleteAllEReaderDataResponse is the response for deleting eReader test data.
type deleteAllEReaderDataResponse struct {
	Libraries int `json:"libraries"`
	Books     int `json:"books"`
	Files     int `json:"files"`
	APIKeys   int `json:"apiKeys"`
}

// deleteAllEReaderData deletes all eReader-related test data.
// DELETE /test/ereader.
func (h *handler) deleteAllEReaderData(c echo.Context) error {
	ctx := c.Request().Context()

	var resp deleteAllEReaderDataResponse

	// Delete API key permissions
	_, _ = h.db.NewDelete().
		Model((*apikeys.APIKeyPermission)(nil)).
		Where("1=1").
		Exec(ctx)

	// Delete API key short URLs
	_, _ = h.db.NewDelete().
		Model((*apikeys.APIKeyShortURL)(nil)).
		Where("1=1").
		Exec(ctx)

	// Delete API keys
	result, _ := h.db.NewDelete().
		Model((*apikeys.APIKey)(nil)).
		Where("1=1").
		Exec(ctx)
	if n, _ := result.RowsAffected(); n > 0 {
		resp.APIKeys = int(n)
	}

	// Delete authors
	_, _ = h.db.NewDelete().
		Model((*models.Author)(nil)).
		Where("1=1").
		Exec(ctx)

	// Delete narrators
	_, _ = h.db.NewDelete().
		Model((*models.Narrator)(nil)).
		Where("1=1").
		Exec(ctx)

	// Delete book series
	_, _ = h.db.NewDelete().
		Model((*models.BookSeries)(nil)).
		Where("1=1").
		Exec(ctx)

	// Delete book genres
	_, _ = h.db.NewDelete().
		Model((*models.BookGenre)(nil)).
		Where("1=1").
		Exec(ctx)

	// Delete book tags
	_, _ = h.db.NewDelete().
		Model((*models.BookTag)(nil)).
		Where("1=1").
		Exec(ctx)

	// Delete file identifiers
	_, _ = h.db.NewDelete().
		Model((*models.FileIdentifier)(nil)).
		Where("1=1").
		Exec(ctx)

	// Delete files
	result, _ = h.db.NewDelete().
		Model((*models.File)(nil)).
		Where("1=1").
		Exec(ctx)
	if n, _ := result.RowsAffected(); n > 0 {
		resp.Files = int(n)
	}

	// Delete books
	result, _ = h.db.NewDelete().
		Model((*models.Book)(nil)).
		Where("1=1").
		Exec(ctx)
	if n, _ := result.RowsAffected(); n > 0 {
		resp.Books = int(n)
	}

	// Delete series
	_, _ = h.db.NewDelete().
		Model((*models.Series)(nil)).
		Where("1=1").
		Exec(ctx)

	// Delete persons
	_, _ = h.db.NewDelete().
		Model((*models.Person)(nil)).
		Where("1=1").
		Exec(ctx)

	// Delete library paths
	_, _ = h.db.NewDelete().
		Model((*models.LibraryPath)(nil)).
		Where("1=1").
		Exec(ctx)

	// Delete libraries
	result, _ = h.db.NewDelete().
		Model((*models.Library)(nil)).
		Where("1=1").
		Exec(ctx)
	if n, _ := result.RowsAffected(); n > 0 {
		resp.Libraries = int(n)
	}

	return c.JSON(http.StatusOK, resp)
}
