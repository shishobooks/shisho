package apikeys

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

var ErrNotFound = errors.New("api key not found")

type Service struct {
	db *bun.DB
}

func NewService(db *bun.DB) *Service {
	return &Service{db: db}
}

// generateKey creates a cryptographically secure random API key.
func generateKey() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return "ak_" + base64.URLEncoding.EncodeToString(bytes), nil
}

// Create creates a new API key for a user.
func (s *Service) Create(ctx context.Context, userID int, name string) (*APIKey, error) {
	key, err := generateKey()
	if err != nil {
		return nil, err
	}

	now := time.Now()
	apiKey := &APIKey{
		ID:        uuid.New().String(),
		UserID:    userID,
		Name:      name,
		Key:       key,
		CreatedAt: now,
		UpdatedAt: now,
	}

	_, err = s.db.NewInsert().Model(apiKey).Exec(ctx)
	if err != nil {
		return nil, err
	}

	return apiKey, nil
}

// List returns all API keys for a user.
func (s *Service) List(ctx context.Context, userID int) ([]*APIKey, error) {
	var keys []*APIKey
	err := s.db.NewSelect().
		Model(&keys).
		Relation("Permissions").
		Where("user_id = ?", userID).
		Order("created_at ASC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return keys, nil
}

// GetByKey retrieves an API key by its key value.
func (s *Service) GetByKey(ctx context.Context, key string) (*APIKey, error) {
	apiKey := new(APIKey)
	err := s.db.NewSelect().
		Model(apiKey).
		Relation("Permissions").
		Where("key = ?", key).
		Scan(ctx)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, nil
		}
		return nil, err
	}
	return apiKey, nil
}

// Delete removes an API key (only if owned by the user).
func (s *Service) Delete(ctx context.Context, userID int, keyID string) error {
	result, err := s.db.NewDelete().
		Model((*APIKey)(nil)).
		Where("id = ?", keyID).
		Where("user_id = ?", userID).
		Exec(ctx)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateName updates an API key's name.
func (s *Service) UpdateName(ctx context.Context, userID int, keyID string, name string) (*APIKey, error) {
	now := time.Now()
	result, err := s.db.NewUpdate().
		Model((*APIKey)(nil)).
		Set("name = ?", name).
		Set("updated_at = ?", now).
		Where("id = ?", keyID).
		Where("user_id = ?", userID).
		Exec(ctx)
	if err != nil {
		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if rowsAffected == 0 {
		return nil, ErrNotFound
	}

	// Fetch the updated key
	var apiKey APIKey
	err = s.db.NewSelect().
		Model(&apiKey).
		Relation("Permissions").
		Where("ak.id = ?", keyID).
		Scan(ctx)
	if err != nil {
		return nil, err
	}

	return &apiKey, nil
}

// AddPermission adds a permission to an API key.
func (s *Service) AddPermission(ctx context.Context, userID int, keyID string, permission string) (*APIKey, error) {
	// Verify ownership
	var apiKey APIKey
	err := s.db.NewSelect().
		Model(&apiKey).
		Where("id = ?", keyID).
		Where("user_id = ?", userID).
		Scan(ctx)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, ErrNotFound
		}
		return nil, err
	}

	// Insert permission (ignore if already exists)
	perm := &APIKeyPermission{
		ID:         uuid.New().String(),
		APIKeyID:   keyID,
		Permission: permission,
		CreatedAt:  time.Now(),
	}
	_, err = s.db.NewInsert().
		Model(perm).
		On("CONFLICT (api_key_id, permission) DO NOTHING").
		Exec(ctx)
	if err != nil {
		return nil, err
	}

	// Fetch updated key with permissions
	err = s.db.NewSelect().
		Model(&apiKey).
		Relation("Permissions").
		Where("ak.id = ?", keyID).
		Scan(ctx)
	if err != nil {
		return nil, err
	}

	return &apiKey, nil
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
		return "", err
	}
	for i := range bytes {
		bytes[i] = shortCodeChars[int(bytes[i])%len(shortCodeChars)]
	}
	return string(bytes), nil
}

// GenerateShortURL creates a temporary short URL for an API key.
func (s *Service) GenerateShortURL(ctx context.Context, userID int, keyID string) (*APIKeyShortURL, error) {
	// Verify ownership
	var apiKey APIKey
	err := s.db.NewSelect().
		Model(&apiKey).
		Where("id = ?", keyID).
		Where("user_id = ?", userID).
		Scan(ctx)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, ErrNotFound
		}
		return nil, err
	}

	// Generate unique short code
	var shortCode string
	for i := 0; i < 10; i++ {
		shortCode, err = generateShortCode()
		if err != nil {
			return nil, err
		}
		// Check if already exists
		exists, err := s.db.NewSelect().
			Model((*APIKeyShortURL)(nil)).
			Where("short_code = ?", shortCode).
			Exists(ctx)
		if err != nil {
			return nil, err
		}
		if !exists {
			break
		}
	}

	now := time.Now()
	shortURL := &APIKeyShortURL{
		ID:        uuid.New().String(),
		APIKeyID:  keyID,
		ShortCode: shortCode,
		ExpiresAt: now.Add(shortURLTTL),
		CreatedAt: now,
	}

	_, err = s.db.NewInsert().Model(shortURL).Exec(ctx)
	if err != nil {
		return nil, err
	}

	return shortURL, nil
}

// ResolveShortCode looks up a short code and returns the associated API key.
func (s *Service) ResolveShortCode(ctx context.Context, shortCode string) (*APIKey, error) {
	var shortURL APIKeyShortURL
	err := s.db.NewSelect().
		Model(&shortURL).
		Relation("APIKey").
		Where("short_code = ?", shortCode).
		Where("expires_at > ?", time.Now()).
		Scan(ctx)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, nil
		}
		return nil, err
	}

	return shortURL.APIKey, nil
}

// RemovePermission removes a permission from an API key.
func (s *Service) RemovePermission(ctx context.Context, userID int, keyID string, permission string) (*APIKey, error) {
	// Verify ownership
	var apiKey APIKey
	err := s.db.NewSelect().
		Model(&apiKey).
		Where("id = ?", keyID).
		Where("user_id = ?", userID).
		Scan(ctx)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, ErrNotFound
		}
		return nil, err
	}

	// Delete permission
	_, err = s.db.NewDelete().
		Model((*APIKeyPermission)(nil)).
		Where("api_key_id = ?", keyID).
		Where("permission = ?", permission).
		Exec(ctx)
	if err != nil {
		return nil, err
	}

	// Fetch updated key with permissions
	err = s.db.NewSelect().
		Model(&apiKey).
		Relation("Permissions").
		Where("ak.id = ?", keyID).
		Scan(ctx)
	if err != nil {
		return nil, err
	}

	return &apiKey, nil
}

// TouchLastAccessed updates the last_accessed_at timestamp for an API key.
func (s *Service) TouchLastAccessed(ctx context.Context, keyID string) error {
	_, err := s.db.NewUpdate().
		Model((*APIKey)(nil)).
		Set("last_accessed_at = ?", time.Now()).
		Where("id = ?", keyID).
		Exec(ctx)
	return err
}
