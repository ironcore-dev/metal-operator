// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package token

import (
	"crypto/rand"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// GenerateSigningSecret generates a cryptographically secure signing secret
// for HMAC token generation. Returns a 32-byte (256-bit) secret.
func GenerateSigningSecret() ([]byte, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, fmt.Errorf("failed to generate random secret: %w", err)
	}
	return secret, nil
}

// GenerateSignedDiscoveryToken generates a JWT-based signed discovery token.
// The token uses HMAC-SHA256 (HS256) signing with standard JWT claims.
//
// This allows the registry to verify the token was issued by the controller
// for a specific systemUUID without storing any state.
//
// signingSecret: 32-byte shared secret between controller and registry
// systemUUID: The server's system UUID
// Returns: signed JWT token
func GenerateSignedDiscoveryToken(signingSecret []byte, systemUUID string) (string, error) {
	if len(signingSecret) != 32 {
		return "", fmt.Errorf("signing secret must be exactly 32 bytes")
	}

	// Create JWT claims
	claims := jwt.MapClaims{
		"sub": systemUUID,                                        // Subject: scoped to this server
		"iat": jwt.NewNumericDate(time.Now()),                    // Issued at
		"exp": jwt.NewNumericDate(time.Now().Add(1 * time.Hour)), // Expires in 1 hour
	}

	// Create token with HS256 signing method
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// Sign token with secret key
	tokenString, err := token.SignedString(signingSecret)
	if err != nil {
		return "", fmt.Errorf("failed to sign JWT token: %w", err)
	}

	return tokenString, nil
}

// VerifySignedDiscoveryToken verifies and extracts information from a JWT token.
// Returns (systemUUID, timestamp, valid, error).
// For invalid tokens, returns ("", 0, false, nil) - error is only for system errors.
func VerifySignedDiscoveryToken(signingSecret []byte, tokenString string) (string, int64, bool, error) {
	if len(signingSecret) != 32 {
		return "", 0, false, fmt.Errorf("signing secret must be exactly 32 bytes")
	}

	// Parse and validate JWT token
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {
		// Verify signing method is exactly HS256 (HMAC-SHA256)
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return signingSecret, nil
	})

	if err != nil {
		return "", 0, false, nil // Invalid token, not system error
	}

	// Verify token is valid and not expired
	if !token.Valid {
		return "", 0, false, nil
	}

	// Extract claims
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", 0, false, nil
	}

	// Extract systemUUID from "sub" claim
	systemUUID, ok := claims["sub"].(string)
	if !ok || systemUUID == "" {
		return "", 0, false, nil
	}

	// Extract issued-at timestamp from "iat" claim
	var issuedAt int64
	if iatClaim, ok := claims["iat"]; ok {
		switch v := iatClaim.(type) {
		case float64:
			issuedAt = int64(v)
		case int64:
			issuedAt = v
		default:
			return "", 0, false, nil
		}
	} else {
		// If no iat claim, use current time (backward compatible)
		issuedAt = time.Now().Unix()
	}

	return systemUUID, issuedAt, true, nil
}
