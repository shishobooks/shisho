package devices

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/delivery"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// ErrNotFound is returned when a Device does not exist or is not the
// caller's.
var ErrNotFound = errors.New("device not found")

type Service struct {
	db *bun.DB
}

func NewService(db *bun.DB) *Service {
	return &Service{db: db}
}

// newCredential returns a credential no Device holds yet.
func (s *Service) newCredential(ctx context.Context, tx bun.IDB) (string, error) {
	for range 10 {
		credential, err := generateCredential()
		if err != nil {
			return "", err
		}
		exists, err := tx.NewSelect().Model((*Device)(nil)).Where("credential = ?", credential).Exists(ctx)
		if err != nil {
			return "", errors.WithStack(err)
		}
		if !exists {
			return credential, nil
		}
	}
	return "", errors.New("could not generate a unique device credential")
}

// Create adds a Device for userID with every connection its type can use.
func (s *Service) Create(ctx context.Context, userID int, name, deviceType string) (*Device, error) {
	now := time.Now()
	device := &Device{
		ID:         uuid.New().String(),
		UserID:     userID,
		Name:       name,
		DeviceType: deviceType,
		ScopeType:  ScopeAll,
		// Kobo and Kindle Devices work as soon as they're added, from
		// their presets; the others follow the owner's default.
		DeliveryRulesMode: startingRulesMode(deviceType),
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		credential, err := s.newCredential(ctx, tx)
		if err != nil {
			return err
		}
		device.Credential = credential
		_, err = tx.NewInsert().Model(device).Exec(ctx)
		return errors.WithStack(err)
	})
	if err != nil {
		return nil, err
	}
	return device, nil
}

func (s *Service) selectDevices(model any) *bun.SelectQuery {
	return s.db.NewSelect().Model(model).Relation("ScopeLibrary").Relation("ScopeList")
}

// List returns userID's Devices, oldest first.
func (s *Service) List(ctx context.Context, userID int) ([]*Device, error) {
	devices := []*Device{}
	err := s.selectDevices(&devices).
		Where("d.user_id = ?", userID).
		Order("d.created_at ASC", "d.id ASC").
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return devices, nil
}

// ListAll returns every user's Devices with their owners, ordered by owner
// and then oldest first.
func (s *Service) ListAll(ctx context.Context) ([]*Device, error) {
	devices := []*Device{}
	err := s.selectDevices(&devices).
		Relation("Owner").
		Order("owner.username ASC", "d.created_at ASC", "d.id ASC").
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return devices, nil
}

// Retrieve returns userID's Device id, or ErrNotFound.
func (s *Service) Retrieve(ctx context.Context, userID int, id string) (*Device, error) {
	device := new(Device)
	err := s.selectDevices(device).
		Where("d.id = ?", id).
		Where("d.user_id = ?", userID).
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return device, nil
}

// GetByCredential returns the Device whose credential is exactly
// credential, or nil. The column compares bytes, so a credential typed in
// another case, with a hyphen, or with a lookalike letter matches nothing.
func (s *Service) GetByCredential(ctx context.Context, credential string) (*Device, error) {
	if credential == "" {
		return nil, nil
	}
	device := new(Device)
	err := s.selectDevices(device).
		Where("d.credential = ?", credential).
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return device, nil
}

// GetByOwnerCredential returns the Device of the user named username (in
// any case, as password sign-in matches it) whose credential is exactly
// credential, or nil. It compares against each of that user's Devices in
// constant time, so a guess reveals nothing about how close it came, and
// costs one query, not a bcrypt comparison.
func (s *Service) GetByOwnerCredential(ctx context.Context, username, credential string) (*Device, error) {
	if username == "" || credential == "" {
		return nil, nil
	}
	devices := []*Device{}
	err := s.selectDevices(&devices).
		Where("d.user_id IN (SELECT id FROM users WHERE username = ? COLLATE NOCASE)", username).
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var match *Device
	for _, device := range devices {
		if subtle.ConstantTimeCompare([]byte(device.Credential), []byte(credential)) == 1 {
			match = device
		}
	}
	return match, nil
}

// UpdateDeviceOptions holds the changes Update applies. A nil field is left
// unchanged. Callers check that a scope's library or list is one the owner
// can see. A new DeviceType puts the Device back on the rules that type
// starts with, since its own list may name File Types the new type can't
// receive, unless Rules says what to follow instead.
type UpdateDeviceOptions struct {
	Name       *string
	DeviceType *string
	Scope      *Scope
	Rules      *RulesChange
}

// RulesChange makes a Device follow its owner's default, follow its type's
// preset, or use Rules as its own list (Mode is a RulesMode constant).
type RulesChange struct {
	Mode  string
	Rules []delivery.Rule
}

// Update applies opts to userID's Device id, in one transaction, and returns
// it. It returns a *delivery.RulesError, before saving anything, for rules
// the Device's resulting type can't follow.
func (s *Service) Update(ctx context.Context, userID int, id string, opts UpdateDeviceOptions) (*Device, error) {
	device, err := s.Retrieve(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	resultingType := device.DeviceType
	if opts.DeviceType != nil {
		resultingType = *opts.DeviceType
	}
	if opts.Rules != nil {
		if err := CheckRules(resultingType, opts.Rules.Mode, opts.Rules.Rules); err != nil {
			return nil, err
		}
	}

	columns := []string{"updated_at"}
	if opts.Name != nil {
		device.Name = *opts.Name
		columns = append(columns, "name")
	}
	typeChanged := opts.DeviceType != nil && *opts.DeviceType != device.DeviceType
	if typeChanged {
		device.DeviceType = *opts.DeviceType
		columns = append(columns, "device_type")
	}
	if opts.Scope != nil {
		device.ScopeType = opts.Scope.Type
		device.ScopeLibraryID = opts.Scope.LibraryID
		device.ScopeListID = opts.Scope.ListID
		// A scope chosen here wins over the one in a migrated Kobo's old
		// sync URL.
		device.URLScopePending = false
		columns = append(columns, "scope_type", "scope_library_id", "scope_list_id", "url_scope_pending")
	}
	device.UpdatedAt = time.Now()

	err = s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewUpdate().Model(device).Column(columns...).WherePK().Exec(ctx); err != nil {
			return errors.WithStack(err)
		}
		switch {
		case opts.Rules != nil:
			return saveRules(ctx, tx, device, opts.Rules.Mode, opts.Rules.Rules)
		case typeChanged:
			return saveRules(ctx, tx, device, startingRulesMode(device.DeviceType), nil)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.Retrieve(ctx, userID, id)
}

// RegenerateCredential gives userID's Device id a new credential, so URLs
// holding the old one stop working. Its short URLs are deleted, since they
// would otherwise redirect to the new credential.
func (s *Service) RegenerateCredential(ctx context.Context, userID int, id string) (*Device, error) {
	device, err := s.Retrieve(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	err = s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		credential, err := s.newCredential(ctx, tx)
		if err != nil {
			return err
		}
		device.Credential = credential
		device.URLScopePending = false
		device.UpdatedAt = time.Now()
		if _, err := tx.NewUpdate().Model(device).Column("credential", "url_scope_pending", "updated_at").WherePK().Exec(ctx); err != nil {
			return errors.WithStack(err)
		}
		_, err = tx.NewDelete().Model((*DeviceShortURL)(nil)).Where("device_id = ?", device.ID).Exec(ctx)
		return errors.WithStack(err)
	})
	if err != nil {
		return nil, err
	}
	return device, nil
}

// Delete removes userID's Device id, with its short URLs and Kobo sync state.
func (s *Service) Delete(ctx context.Context, userID int, id string) error {
	return s.delete(ctx, s.db.NewDelete().Model((*Device)(nil)).Where("id = ?", id).Where("user_id = ?", userID))
}

// Revoke removes any user's Device id. Admins use it to cut off a lost
// Device.
func (s *Service) Revoke(ctx context.Context, id string) error {
	return s.delete(ctx, s.db.NewDelete().Model((*Device)(nil)).Where("id = ?", id))
}

func (s *Service) delete(ctx context.Context, q *bun.DeleteQuery) error {
	result, err := q.Exec(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return errors.WithStack(err)
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ApplyURLScope sets a migrated Kobo's scope from the scope segment of its
// old sync URL, the first time it connects after upgrading, and returns
// the Device as stored afterwards. Later requests, and Devices that never
// had a scope in their URL, keep the Device's own scope.
func (s *Service) ApplyURLScope(ctx context.Context, device *Device, scope Scope) (*Device, error) {
	if !device.URLScopePending {
		return device, nil
	}
	// A URL naming a library or list that no longer exists synced
	// nothing, so the Device keeps the scope type with no id, which still
	// syncs nothing.
	if scope.LibraryID != nil {
		exists, err := s.libraryExists(ctx, *scope.LibraryID)
		if err != nil {
			return nil, err
		}
		if !exists {
			scope.LibraryID = nil
		}
	}
	if scope.ListID != nil {
		exists, err := s.db.NewSelect().Model((*models.List)(nil)).Where("id = ?", *scope.ListID).Exists(ctx)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		if !exists {
			scope.ListID = nil
		}
	}
	// Only set it while still pending, so a scope saved in the meantime,
	// or by a concurrent request, is never overwritten.
	_, err := s.db.NewUpdate().
		Model((*Device)(nil)).
		Set("scope_type = ?", scope.Type).
		Set("scope_library_id = ?", scope.LibraryID).
		Set("scope_list_id = ?", scope.ListID).
		Set("url_scope_pending = ?", false).
		Where("id = ?", device.ID).
		Where("url_scope_pending").
		Exec(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return s.Retrieve(ctx, device.UserID, device.ID)
}

const (
	shortCodeLength = 6
	shortCodeChars  = "abcdefghijklmnopqrstuvwxyz0123456789"
	shortURLTTL     = 30 * time.Minute
)

// generateShortCode creates a random short code.
func generateShortCode() (string, error) {
	bytes := make([]byte, shortCodeLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", errors.WithStack(err)
	}
	for i := range bytes {
		bytes[i] = shortCodeChars[int(bytes[i])%len(shortCodeChars)]
	}
	return string(bytes), nil
}

// GenerateShortURL creates a short URL for userID's Device id that expires
// after shortURLTTL.
func (s *Service) GenerateShortURL(ctx context.Context, userID int, id string) (*DeviceShortURL, error) {
	if _, err := s.Retrieve(ctx, userID, id); err != nil {
		return nil, err
	}

	var shortCode string
	for range 10 {
		code, err := generateShortCode()
		if err != nil {
			return nil, err
		}
		exists, err := s.db.NewSelect().
			Model((*DeviceShortURL)(nil)).
			Where("short_code = ?", code).
			Exists(ctx)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		if !exists {
			shortCode = code
			break
		}
	}
	if shortCode == "" {
		return nil, errors.New("could not generate a unique short code")
	}

	// A Device keeps one setup URL at a time, so an earlier one stops
	// working once a new one is made.
	if _, err := s.db.NewDelete().Model((*DeviceShortURL)(nil)).Where("device_id = ?", id).Exec(ctx); err != nil {
		return nil, errors.WithStack(err)
	}

	now := time.Now()
	shortURL := &DeviceShortURL{
		ID:        uuid.New().String(),
		DeviceID:  id,
		ShortCode: shortCode,
		ExpiresAt: now.Add(shortURLTTL),
		CreatedAt: now,
	}
	if _, err := s.db.NewInsert().Model(shortURL).Exec(ctx); err != nil {
		return nil, errors.WithStack(err)
	}
	return shortURL, nil
}

// ResolveShortCode returns the Device an unexpired short code belongs to,
// or nil.
func (s *Service) ResolveShortCode(ctx context.Context, shortCode string) (*Device, error) {
	var shortURL DeviceShortURL
	err := s.db.NewSelect().
		Model(&shortURL).
		Where("short_code = ?", shortCode).
		Where("expires_at > ?", time.Now()).
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}
	device := new(Device)
	err = s.selectDevices(device).Where("d.id = ?", shortURL.DeviceID).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return device, nil
}

// AuthenticateOwner loads the user who owns device, with Role,
// Role.Permissions and LibraryAccess, and checks that the owner may still use
// it. A missing or deactivated owner returns 401, and an owner whose role
// lacks books:read returns 403. Middleware.DeviceAuth calls this so a Device
// never outlives its owner's access, and stores the returned user with
// auth.SetUser for handlers to reuse. The eReader short URL calls it too.
func (s *Service) AuthenticateOwner(ctx context.Context, device *Device) (*models.User, error) {
	user, err := auth.LoadUser(ctx, s.db, auth.LoadUserOptions{ID: &device.UserID})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errcodes.UserInactive()
	}
	if err != nil {
		return nil, err
	}
	if !user.HasPermission(models.ResourceBooks, models.OperationRead) {
		return nil, errcodes.PermissionDenied(models.ResourceBooks, models.OperationRead)
	}
	return user, nil
}

// TouchLastAccessed updates the last_accessed_at timestamp for a Device.
func (s *Service) TouchLastAccessed(ctx context.Context, id string) error {
	_, err := s.db.NewUpdate().
		Model((*Device)(nil)).
		Set("last_accessed_at = ?", time.Now()).
		Where("id = ?", id).
		Exec(ctx)
	return errors.WithStack(err)
}

// koboSyncPoint mirrors the kobo.SyncPoint model for deletion purposes.
// We define it here to avoid a circular dependency between devices and kobo packages.
type koboSyncPoint struct {
	bun.BaseModel `bun:"table:kobo_sync_points"`
	ID            string `bun:"id,pk"`
	DeviceID      string `bun:"device_id"`
}

// ClearKoboSyncHistory deletes all Kobo sync points for userID's Device id,
// forcing a fresh sync.
func (s *Service) ClearKoboSyncHistory(ctx context.Context, userID int, id string) error {
	if _, err := s.Retrieve(ctx, userID, id); err != nil {
		return err
	}
	_, err := s.db.NewDelete().
		Model((*koboSyncPoint)(nil)).
		Where("device_id = ?", id).
		Exec(ctx)
	return errors.WithStack(err)
}

// libraryExists reports whether library id exists.
func (s *Service) libraryExists(ctx context.Context, id int) (bool, error) {
	exists, err := s.db.NewSelect().Model((*models.Library)(nil)).Where("id = ?", id).Exists(ctx)
	return exists, errors.WithStack(err)
}
