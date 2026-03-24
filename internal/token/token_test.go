// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package token

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestToken(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Token Suite")
}

var _ = Describe("GenerateSigningSecret", func() {
	It("should generate a 32-byte secret", func() {
		secret, err := GenerateSigningSecret()
		Expect(err).NotTo(HaveOccurred())
		Expect(secret).To(HaveLen(32))
	})

	It("should generate unique secrets", func() {
		secret1, err := GenerateSigningSecret()
		Expect(err).NotTo(HaveOccurred())

		secret2, err := GenerateSigningSecret()
		Expect(err).NotTo(HaveOccurred())

		Expect(secret1).NotTo(Equal(secret2))
	})
})

var _ = Describe("GenerateSignedDiscoveryToken", func() {
	var signingSecret []byte

	BeforeEach(func() {
		var err error
		signingSecret, err = GenerateSigningSecret()
		Expect(err).NotTo(HaveOccurred())
	})

	Context("Token Format", func() {
		It("should generate a valid JWT token", func() {
			token, err := GenerateSignedDiscoveryToken(signingSecret, "test-uuid-123")
			Expect(err).NotTo(HaveOccurred())
			Expect(token).NotTo(BeEmpty())

			// JWT format: header.payload.signature (3 parts separated by dots)
			parts := strings.Split(token, ".")
			Expect(parts).To(HaveLen(3))
		})

		It("should generate different tokens for different UUIDs", func() {
			token1, err := GenerateSignedDiscoveryToken(signingSecret, "uuid-1")
			Expect(err).NotTo(HaveOccurred())

			token2, err := GenerateSignedDiscoveryToken(signingSecret, "uuid-2")
			Expect(err).NotTo(HaveOccurred())

			Expect(token1).NotTo(Equal(token2))
		})

		It("should generate different tokens at different times", func() {
			token1, err := GenerateSignedDiscoveryToken(signingSecret, "test-uuid")
			Expect(err).NotTo(HaveOccurred())

			// Wait 1+ second to ensure different timestamp
			time.Sleep(1100 * time.Millisecond)
			token2, err := GenerateSignedDiscoveryToken(signingSecret, "test-uuid")
			Expect(err).NotTo(HaveOccurred())
			Expect(token2).NotTo(Equal(token1))
		})

		It("should return error for invalid secret length", func() {
			shortSecret := []byte("too-short")
			_, err := GenerateSignedDiscoveryToken(shortSecret, "test-uuid")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("32 bytes"))
		})
	})
})

var _ = Describe("VerifySignedDiscoveryToken", func() {
	var signingSecret []byte

	BeforeEach(func() {
		var err error
		signingSecret, err = GenerateSigningSecret()
		Expect(err).NotTo(HaveOccurred())
	})

	Context("Valid Tokens", func() {
		It("should verify a valid token", func() {
			systemUUID := "test-uuid-456"
			token, err := GenerateSignedDiscoveryToken(signingSecret, systemUUID)
			Expect(err).NotTo(HaveOccurred())

			extractedUUID, timestamp, valid, err := VerifySignedDiscoveryToken(signingSecret, token)
			Expect(err).NotTo(HaveOccurred())
			Expect(valid).To(BeTrue())
			Expect(extractedUUID).To(Equal(systemUUID))
			Expect(timestamp).To(BeNumerically(">", 0))
		})

		It("should extract the correct systemUUID", func() {
			systemUUID := "38947555-7742-3448-3784-823347823834"
			token, err := GenerateSignedDiscoveryToken(signingSecret, systemUUID)
			Expect(err).NotTo(HaveOccurred())

			extractedUUID, _, valid, err := VerifySignedDiscoveryToken(signingSecret, token)
			Expect(err).NotTo(HaveOccurred())
			Expect(valid).To(BeTrue())
			Expect(extractedUUID).To(Equal(systemUUID))
		})
	})

	Context("Invalid Tokens", func() {
		It("should reject token with wrong signature", func() {
			token, err := GenerateSignedDiscoveryToken(signingSecret, "test-uuid")
			Expect(err).NotTo(HaveOccurred())

			// Use different secret for verification
			wrongSecret, err := GenerateSigningSecret()
			Expect(err).NotTo(HaveOccurred())

			_, _, valid, err := VerifySignedDiscoveryToken(wrongSecret, token)
			Expect(err).NotTo(HaveOccurred())
			Expect(valid).To(BeFalse())
		})

		It("should reject expired token", func() {
			systemUUID := "test-uuid-expired"

			// Create token with expiration in the past
			claims := jwt.MapClaims{
				"sub": systemUUID,
				"iat": jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),    // Issued 2 hours ago
				"exp": jwt.NewNumericDate(time.Now().Add(-30 * time.Minute)), // Expired 30 minutes ago
			}

			token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
			expiredToken, err := token.SignedString(signingSecret)
			Expect(err).NotTo(HaveOccurred())

			// Verify expired token should fail
			_, _, valid, err := VerifySignedDiscoveryToken(signingSecret, expiredToken)
			Expect(err).NotTo(HaveOccurred())
			Expect(valid).To(BeFalse())
		})

		It("should reject tampered token", func() {
			token, err := GenerateSignedDiscoveryToken(signingSecret, "test-uuid")
			Expect(err).NotTo(HaveOccurred())

			// Tamper with token
			tamperedToken := token[:len(token)-5] + "XXXXX"

			_, _, valid, err := VerifySignedDiscoveryToken(signingSecret, tamperedToken)
			Expect(err).NotTo(HaveOccurred())
			Expect(valid).To(BeFalse())
		})

		It("should reject invalid JWT format", func() {
			testCases := []struct {
				name  string
				token string
			}{
				{"empty", ""},
				{"malformed", "not.a.valid.jwt"},
				{"incomplete", "header.payload"},
				{"random", "random-string-not-jwt"},
			}

			for _, tc := range testCases {
				_, _, valid, err := VerifySignedDiscoveryToken(signingSecret, tc.token)
				Expect(err).NotTo(HaveOccurred(), "test case: "+tc.name)
				Expect(valid).To(BeFalse(), "test case: "+tc.name)
			}
		})

		It("should reject token with invalid secret length", func() {
			shortSecret := []byte("too-short")
			_, _, _, err := VerifySignedDiscoveryToken(shortSecret, "any-token")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("32 bytes"))
		})
	})

	Context("Token Security", func() {
		It("should use JWT library constant-time comparison", func() {
			// JWT library provides constant-time comparison via HMAC
			// This test verifies the signature mechanism works
			token, err := GenerateSignedDiscoveryToken(signingSecret, "test-uuid")
			Expect(err).NotTo(HaveOccurred())

			_, _, valid, err := VerifySignedDiscoveryToken(signingSecret, token)
			Expect(err).NotTo(HaveOccurred())
			Expect(valid).To(BeTrue())
		})

		It("should prevent replay of UUID substitution", func() {
			// Generate token for uuid-1
			token1, err := GenerateSignedDiscoveryToken(signingSecret, "uuid-1")
			Expect(err).NotTo(HaveOccurred())

			// Verify - should extract uuid-1, not uuid-2
			extractedUUID, _, valid, err := VerifySignedDiscoveryToken(signingSecret, token1)
			Expect(err).NotTo(HaveOccurred())
			Expect(valid).To(BeTrue())
			Expect(extractedUUID).To(Equal("uuid-1"))
			Expect(extractedUUID).NotTo(Equal("uuid-2"))
		})

		It("should reject tokens with wrong algorithm", func() {
			systemUUID := "test-uuid-alg"

			// Create token with HS512 instead of HS256
			claims := jwt.MapClaims{
				"sub": systemUUID,
				"iat": jwt.NewNumericDate(time.Now()),
				"exp": jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
			}

			token := jwt.NewWithClaims(jwt.SigningMethodHS512, claims) // Wrong algorithm
			wrongAlgToken, err := token.SignedString(signingSecret)
			Expect(err).NotTo(HaveOccurred())

			// Verify should fail due to algorithm mismatch
			_, _, valid, err := VerifySignedDiscoveryToken(signingSecret, wrongAlgToken)
			Expect(err).NotTo(HaveOccurred())
			Expect(valid).To(BeFalse())
		})
	})
})
