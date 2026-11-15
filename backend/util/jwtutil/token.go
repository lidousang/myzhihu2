// Package jwtutil signs and verifies access tokens used by the HTTP API.
package jwtutil

import (
	"errors"
	"time"

	"myzhihu2/conf"

	"github.com/golang-jwt/jwt/v5"
)

// Claims contains the authenticated user identity and the standard JWT claims.
// UserID is deliberately kept small: current profile and permission data remain
// authoritative in MySQL rather than being copied into a long-lived token.
type Claims struct {
	UserID uint `json:"user_id"`
	jwt.RegisteredClaims
}

// Sign creates one access token for an authenticated user.
func Sign(config conf.JWT, userID uint) (string, error) {
	if userID == 0 {
		return "", errors.New("jwt: user_id must be greater than zero")
	}
	if config.Algorithm != jwt.SigningMethodHS256.Alg() {
		return "", errors.New("jwt: unsupported signing algorithm")
	}

	now := time.Now()
	claims := Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    config.Issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(config.ExpireHours) * time.Hour)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(config.Secret))
}

// Parse verifies an access token and returns its typed claims. The accepted
// algorithm, issuer, and expiration are all checked here so every caller uses
// exactly the same validation rules.
func Parse(config conf.JWT, rawToken string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(
		rawToken,
		claims,
		func(token *jwt.Token) (any, error) {
			if token.Method.Alg() != config.Algorithm {
				return nil, errors.New("jwt: unexpected signing algorithm")
			}
			return []byte(config.Secret), nil
		},
		jwt.WithValidMethods([]string{config.Algorithm}),
		jwt.WithIssuer(config.Issuer),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return nil, err
	}
	if token == nil || !token.Valid || claims.UserID == 0 {
		return nil, errors.New("jwt: invalid access token")
	}
	return claims, nil
}
