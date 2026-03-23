// SPDX-License-Identifier: MIT

package token

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"time"
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

// GenerateSignedDiscoveryToken generates an HMAC-SHA256 signed discovery token.
// The token format is: base64url(systemUUID||timestamp||signature)
//
// This allows the registry to verify the token was issued by the controller
// for a specific systemUUID without storing any state.
//
// signingSecret: 32-byte shared secret between controller and registry
// systemUUID: The server's system UUID
// Returns: signed token (base64url encoded)
func GenerateSignedDiscoveryToken(signingSecret []byte, systemUUID string) (string, error) {
	if len(signingSecret) != 32 {
		return "", fmt.Errorf("signing secret must be exactly 32 bytes")
	}

	// Create payload: systemUUID||timestamp (Unix seconds)
	timestamp := time.Now().Unix()
	payload := fmt.Sprintf("%s||%d", systemUUID, timestamp)

	// Sign the payload with HMAC-SHA256
	mac := hmac.New(sha256.New, signingSecret)
	mac.Write([]byte(payload))
	signature := mac.Sum(nil)

	// Encode as: payload+signature (then base64url, no separator)
	tokenBytes := append([]byte(payload), signature...)
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)

	return token, nil
}

// VerifySignedDiscoveryToken verifies and extracts information from a signed token.
// Returns (systemUUID, timestamp, valid, error).
func VerifySignedDiscoveryToken(signingSecret []byte, token string) (string, int64, bool, error) {
	if len(signingSecret) != 32 {
		return "", 0, false, fmt.Errorf("signing secret must be exactly 32 bytes")
	}

	// Decode base64url
	tokenBytes, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return "", 0, false, fmt.Errorf("failed to decode token: %w", err)
	}

	// Token format: systemUUID||timestamp||signature (32 bytes)
	if len(tokenBytes) < 32 {
		return "", 0, false, fmt.Errorf("token too short: %d bytes", len(tokenBytes))
	}

	// Split payload and signature
	payloadBytes := tokenBytes[:len(tokenBytes)-32]
	receivedSignature := tokenBytes[len(tokenBytes)-32:]

	// Parse payload: systemUUID||timestamp
	payload := string(payloadBytes)

	// Find the first occurrence of "||" to split systemUUID from timestamp
	var systemUUID string
	var timestamp int64

	// Parse by splitting on "||"
	firstDelim := -1
	for i := 0; i < len(payload)-1; i++ {
		if payload[i] == '|' && payload[i+1] == '|' {
			firstDelim = i
			break
		}
	}

	if firstDelim == -1 {
		return "", 0, false, fmt.Errorf("invalid format: no delimiter found")
	}

	systemUUID = payload[:firstDelim]
	timestampStr := payload[firstDelim+2:]

	timestamp, err = strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		return "", 0, false, fmt.Errorf("invalid timestamp: %w", err)
	}

	// Verify signature
	mac := hmac.New(sha256.New, signingSecret)
	mac.Write(payloadBytes)
	expectedSignature := mac.Sum(nil)

	if !hmac.Equal(receivedSignature, expectedSignature) {
		return "", 0, false, fmt.Errorf("invalid HMAC signature")
	}

	// Check token age (reject tokens older than 1 hour to prevent replay)
	tokenAge := time.Now().Unix() - timestamp
	if tokenAge > 3600 || tokenAge < -300 { // 1 hour max age, 5 min clock skew allowance
		return "", 0, false, fmt.Errorf("token expired or clock skew: age=%d seconds", tokenAge)
	}

	return systemUUID, timestamp, true, nil
}
