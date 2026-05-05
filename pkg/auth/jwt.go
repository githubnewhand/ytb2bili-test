package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWTConfig JWT 配置
type JWTConfig struct {
	SecretKey     string
	ExpiryTime    time.Duration
	RefreshTime   time.Duration
}

// Claims JWT 声明
type Claims struct {
	UserID   string `json:"user_id"`
	Email    string `json:"email"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// JWTManager JWT 管理器
type JWTManager struct {
	config *JWTConfig
}

// NewJWTManager 创建 JWT 管理器
func NewJWTManager(config *JWTConfig) *JWTManager {
	if config.SecretKey == "" {
		config.SecretKey = "ytb2bili-default-secret-key-change-in-production"
	}
	if config.ExpiryTime == 0 {
		config.ExpiryTime = 7 * 24 * time.Hour // 默认 7 天
	}
	if config.RefreshTime == 0 {
		config.RefreshTime = 30 * 24 * time.Hour // 默认 30 天
	}
	
	return &JWTManager{
		config: config,
	}
}

// GenerateToken 生成访问 token
func (m *JWTManager) GenerateToken(userID, email, username string) (string, error) {
	claims := Claims{
		UserID:   userID,
		Email:    email,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(m.config.ExpiryTime)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "ytb2bili",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(m.config.SecretKey))
}

// GenerateRefreshToken 生成刷新 token
func (m *JWTManager) GenerateRefreshToken(userID string) (string, error) {
	claims := Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(m.config.RefreshTime)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "ytb2bili-refresh",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(m.config.SecretKey))
}

// ValidateToken 验证 token
func (m *JWTManager) ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(m.config.SecretKey), nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, errors.New("invalid token")
}

// RefreshToken 刷新 token
func (m *JWTManager) RefreshToken(refreshTokenString string) (string, error) {
	claims, err := m.ValidateToken(refreshTokenString)
	if err != nil {
		return "", err
	}

	// 检查是否是刷新 token
	if claims.Issuer != "ytb2bili-refresh" {
		return "", errors.New("not a refresh token")
	}

	// 生成新的访问 token
	return m.GenerateToken(claims.UserID, claims.Email, claims.Username)
}
