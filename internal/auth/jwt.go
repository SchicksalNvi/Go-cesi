package auth

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

// getJWTSecret 从环境变量获取JWT密钥，如果未设置则返回错误
func getJWTSecret() ([]byte, error) {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		return nil, fmt.Errorf("JWT_SECRET environment variable is required but not set")
	}
	if len(secret) < 32 {
		return nil, fmt.Errorf("JWT_SECRET must be at least 32 characters long for security")
	}
	return []byte(secret), nil
}

type Claims struct {
	UserID       string `json:"user_id"`
	TokenVersion uint64 `json:"token_version"`
	jwt.RegisteredClaims
}

// GenerateToken 生成JWT令牌
func GenerateToken(userID string, tokenVersions ...uint64) (string, error) {
	secret, err := getJWTSecret()
	if err != nil {
		return "", fmt.Errorf("failed to get JWT secret: %w", err)
	}

	var tokenVersion uint64
	if len(tokenVersions) > 0 {
		tokenVersion = tokenVersions[0]
	}

	claims := Claims{
		UserID:       userID,
		TokenVersion: tokenVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			Issuer:    "cesi",
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secret)
}

// ParseToken 解析JWT令牌
func ParseToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		// 验证签名方法
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		if token.Method.Alg() != "HS256" {
			return nil, fmt.Errorf("unexpected signing method: %s", token.Method.Alg())
		}
		secret, err := getJWTSecret()
		if err != nil {
			return nil, fmt.Errorf("failed to get JWT secret: %w", err)
		}
		return secret, nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, errors.New("invalid token")
}
