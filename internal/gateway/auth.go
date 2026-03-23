package gateway

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const defaultTokenTTL = 24 * time.Hour

const (
	minUsernameLength = 3
	maxUsernameLength = 64
	minPasswordLength = 6
)

var (
	ErrInvalidCredentials     = errors.New("invalid_credentials")
	ErrInvalidRegisterPayload = errors.New("invalid_register_payload")
	ErrInvalidUsername        = errors.New("invalid_username")
	ErrPasswordTooShort       = errors.New("password_too_short")
	ErrUserAlreadyExists      = errors.New("user_already_exists")
	ErrInvalidRole            = errors.New("invalid_role")
	ErrUnauthorized           = errors.New("unauthorized")
)

// AuthUser is the minimal authenticated user payload.
type AuthUser struct {
	ID       uint     `json:"id"`
	Username string   `json:"username"`
	Role     UserRole `json:"role"`
}

type authClaims struct {
	UserID   uint     `json:"uid"`
	Username string   `json:"username"`
	Role     UserRole `json:"role"`
	jwt.RegisteredClaims
}

// AuthService handles login, registration, and JWT lifecycle.
type AuthService struct {
	db       *gorm.DB
	secret   []byte
	tokenTTL time.Duration
}

func NewAuthService(db *gorm.DB, jwtSecret string) *AuthService {
	trimmed := strings.TrimSpace(jwtSecret)
	if trimmed == "" {
		trimmed = "nexus-agent-dev-secret"
	}
	return &AuthService{
		db:       db,
		secret:   []byte(trimmed),
		tokenTTL: defaultTokenTTL,
	}
}

func (s *AuthService) Authenticate(username, password string) (string, *AuthUser, error) {
	if s == nil || s.db == nil {
		return "", nil, fmt.Errorf("auth service not initialized")
	}

	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return "", nil, ErrInvalidCredentials
	}

	var user User
	if err := s.db.Where("username = ?", username).Take(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil, ErrInvalidCredentials
		}
		return "", nil, fmt.Errorf("query user failed: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return "", nil, ErrInvalidCredentials
	}

	token, err := s.issueToken(&user)
	if err != nil {
		return "", nil, err
	}
	return token, &AuthUser{
		ID:       user.ID,
		Username: user.Username,
		Role:     user.Role,
	}, nil
}

// Register always creates RoleUser.
func (s *AuthService) Register(username, password string) (*AuthUser, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("auth service not initialized")
	}

	user, err := s.createUser(username, password, RoleUser)
	if err != nil {
		return nil, err
	}

	return &AuthUser{
		ID:       user.ID,
		Username: user.Username,
		Role:     user.Role,
	}, nil
}

// CreateUserByAdmin allows admin APIs to create user/admin accounts.
func (s *AuthService) CreateUserByAdmin(username, password string, role UserRole) (*User, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("auth service not initialized")
	}
	if role == "" {
		role = RoleUser
	}
	return s.createUser(username, password, role)
}

func (s *AuthService) createUser(username, password string, role UserRole) (*User, error) {
	username = strings.TrimSpace(username)
	if !isValidRegisterUsername(username) {
		return nil, ErrInvalidUsername
	}
	if len(password) < minPasswordLength {
		return nil, ErrPasswordTooShort
	}
	if role != RoleUser && role != RoleAdmin {
		return nil, ErrInvalidRole
	}

	var existing User
	err := s.db.Where("username = ?", username).Take(&existing).Error
	if err == nil {
		return nil, ErrUserAlreadyExists
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("query user failed: %w", err)
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password failed: %w", err)
	}

	user := &User{
		Username:     username,
		PasswordHash: string(hashed),
		Role:         role,
	}
	if err := s.db.Create(user).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return nil, ErrUserAlreadyExists
		}
		return nil, fmt.Errorf("create user failed: %w", err)
	}

	return user, nil
}

func isValidRegisterUsername(username string) bool {
	if len(username) < minUsernameLength || len(username) > maxUsernameLength {
		return false
	}

	for i := 0; i < len(username); i++ {
		ch := username[i]
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') {
			continue
		}
		if ch == '.' || ch == '_' || ch == '-' {
			continue
		}
		return false
	}
	return true
}

func (s *AuthService) ParseToken(tokenString string) (*AuthUser, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("auth service not initialized")
	}

	claims := &authClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrUnauthorized
		}
		return s.secret, nil
	})
	if err != nil || !token.Valid {
		return nil, ErrUnauthorized
	}

	var user User
	if err := s.db.Where("id = ?", claims.UserID).Take(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUnauthorized
		}
		return nil, fmt.Errorf("query user from token failed: %w", err)
	}

	return &AuthUser{
		ID:       user.ID,
		Username: user.Username,
		Role:     user.Role,
	}, nil
}

func (s *AuthService) issueToken(user *User) (string, error) {
	if user == nil {
		return "", errors.New("user is nil")
	}

	now := time.Now()
	claims := authClaims{
		UserID:   user.ID,
		Username: user.Username,
		Role:     user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.tokenTTL)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.secret)
	if err != nil {
		return "", fmt.Errorf("sign token failed: %w", err)
	}
	return signed, nil
}
