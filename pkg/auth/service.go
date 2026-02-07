package auth

import (
	"context"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
	"golang.org/x/crypto/bcrypt"
)

const (
	// BcryptCost is the cost factor for bcrypt hashing.
	BcryptCost = 12
	// TokenExpiry is how long JWT tokens are valid.
	TokenExpiry = 7 * 24 * time.Hour // 7 days
)

// JWTClaims represents the claims in a JWT token.
type JWTClaims struct {
	UserID   int    `json:"user_id"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// Service handles authentication operations.
type Service struct {
	db        *bun.DB
	jwtSecret []byte
}

// NewService creates a new auth service.
func NewService(db *bun.DB, jwtSecret string) *Service {
	return &Service{
		db:        db,
		jwtSecret: []byte(jwtSecret),
	}
}

// CountUsers returns the total number of users.
func (s *Service) CountUsers(ctx context.Context) (int, error) {
	count, err := s.db.NewSelect().Model((*models.User)(nil)).Count(ctx)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	return count, nil
}

// Authenticate validates credentials and returns the user if valid.
func (s *Service) Authenticate(ctx context.Context, username, password string) (*models.User, error) {
	user := &models.User{}
	err := s.db.NewSelect().
		Model(user).
		Relation("Role").
		Relation("Role.Permissions").
		Relation("LibraryAccess").
		Where("u.username = ? COLLATE NOCASE", username).
		Where("u.is_active = ?", true).
		Scan(ctx)
	if err != nil {
		return nil, errcodes.Unauthorized("Invalid username or password")
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	if err != nil {
		return nil, errcodes.Unauthorized("Invalid username or password")
	}

	return user, nil
}

// GenerateToken creates a new JWT token for the user.
func (s *Service) GenerateToken(user *models.User) (string, error) {
	now := time.Now()
	claims := JWTClaims{
		UserID:   user.ID,
		Username: user.Username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(TokenExpiry)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, err := token.SignedString(s.jwtSecret)
	if err != nil {
		return "", errors.WithStack(err)
	}

	return signedToken, nil
}

// ValidateToken validates a JWT token and returns the claims.
func (s *Service) ValidateToken(tokenString string) (*JWTClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return s.jwtSecret, nil
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}

	claims, ok := token.Claims.(*JWTClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}

	return claims, nil
}

// GetUserByID retrieves a user by ID with relations.
func (s *Service) GetUserByID(ctx context.Context, id int) (*models.User, error) {
	user := &models.User{}
	err := s.db.NewSelect().
		Model(user).
		Relation("Role").
		Relation("Role.Permissions").
		Relation("LibraryAccess").
		Where("u.id = ?", id).
		Where("u.is_active = ?", true).
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return user, nil
}

// CreateFirstAdmin creates the first admin user during setup.
func (s *Service) CreateFirstAdmin(ctx context.Context, username string, email *string, password string) (*models.User, error) {
	// Check if any users exist
	count, err := s.CountUsers(ctx)
	if err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, errcodes.Forbidden("Setup has already been completed")
	}

	// Get admin role
	role := &models.Role{}
	err = s.db.NewSelect().
		Model(role).
		Where("name = ?", models.RoleAdmin).
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), BcryptCost)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// Create user
	user := &models.User{
		Username:           username,
		Email:              email,
		PasswordHash:       string(hashedPassword),
		RoleID:             role.ID,
		IsActive:           true,
		MustChangePassword: false,
	}

	_, err = s.db.NewInsert().Model(user).Exec(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// Grant access to all libraries
	access := &models.UserLibraryAccess{
		UserID:    user.ID,
		LibraryID: nil, // null = all libraries
	}
	_, err = s.db.NewInsert().Model(access).Exec(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// Reload user with relations
	return s.GetUserByID(ctx, user.ID)
}

// HashPassword hashes a password using bcrypt.
func HashPassword(password string) (string, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), BcryptCost)
	if err != nil {
		return "", errors.WithStack(err)
	}
	return string(hashedPassword), nil
}

// CheckPassword compares a password with a hash.
func CheckPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}
