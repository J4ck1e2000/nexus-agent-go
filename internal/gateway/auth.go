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

var (
	// ErrInvalidCredentials 表示用户名或密码错误。
	ErrInvalidCredentials = errors.New("invalid_credentials")
	// ErrUnauthorized 表示 token 缺失或无效。
	ErrUnauthorized = errors.New("unauthorized")
)

// AuthUser 表示认证后的最小用户信息。
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

// AuthService 负责登录认证与 token 签发。
type AuthService struct {
	db       *gorm.DB
	secret   []byte
	tokenTTL time.Duration
}

// NewAuthService 创建鉴权服务。
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

// Authenticate 使用用户名密码登录并返回 JWT。
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

// ParseToken 解析 token 并返回当前用户信息。
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
