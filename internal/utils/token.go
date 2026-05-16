// Package utils provides shared helper functions for the application.
package utils

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// TokenClaims is the payload embedded in every JWT issued by this server.
// It mirrors the Node.js { userId, roles } shape so tokens are fully
// wire-compatible with the existing React frontend expectations.
type TokenClaims struct {
	UserID string   `json:"userId"`
	Roles  []string `json:"roles"`
	jwt.RegisteredClaims
}

// GenerateAccessToken signs a short-lived access JWT (default 15 min).
func GenerateAccessToken(userID string, roles []string, secret string, expiry time.Duration) (string, error) {
	claims := TokenClaims{
		UserID: userID,
		Roles:  roles,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// GenerateRefreshToken signs a longer-lived refresh JWT (default 7 days).
func GenerateRefreshToken(userID string, roles []string, secret string, expiry time.Duration) (string, error) {
	claims := TokenClaims{
		UserID: userID,
		Roles:  roles,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// VerifyToken parses and validates a JWT signed with the given secret.
// Returns the decoded *TokenClaims on success, or an error on failure.
func VerifyToken(tokenString, secret string) (*TokenClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &TokenClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*TokenClaims)
	if !ok || !token.Valid {
		return nil, jwt.ErrTokenInvalidClaims
	}
	return claims, nil
}
