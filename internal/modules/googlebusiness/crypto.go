// Package googlebusiness implements a Google Business Profile (GBP) integration:
// OAuth connect/callback, encrypted token storage, location selection, and reviews.
//
// The whole module is INERT until the operator sets the OAuth env vars
// (GOOGLE_OAUTH_CLIENT_ID/SECRET/REDIRECT_URL). When unset, IsConfigured() is false
// and the service returns ErrNotConfigured so handlers can answer 503 without crashing.
package googlebusiness

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"os"
)

// tokenEncryptionKeyEnv is the env var holding the base64-encoded 32-byte AES-256 key
// used to encrypt OAuth token JSON at rest. Mirrors treasury-api's loadEncryptionKey.
const tokenEncryptionKeyEnv = "GOOGLE_TOKEN_ENCRYPTION_KEY"

// loadEncryptionKey reads the token encryption key from the environment.
// Falls back to a deterministic dev key when unset. This is adapted from
// treasury-api internal/modules/gateways/resolver.go loadEncryptionKey.
//
// In production, GOOGLE_TOKEN_ENCRYPTION_KEY must be set (base64 of 32 random bytes)
// via the K8s secret.
func loadEncryptionKey() []byte {
	keyStr := os.Getenv(tokenEncryptionKeyEnv)
	if keyStr == "" {
		// Development fallback — 32 bytes from a fixed string.
		devKey := "bengobox-ordering-google-dev-key" // 32 bytes
		if len(devKey) < 32 {
			padded := make([]byte, 32)
			copy(padded, devKey)
			return padded
		}
		return []byte(devKey)[:32]
	}
	decoded, err := base64.StdEncoding.DecodeString(keyStr)
	if err != nil || len(decoded) != 32 {
		// If decoding fails or key isn't 32 bytes, use raw bytes (truncated/padded).
		raw := []byte(keyStr)
		if len(raw) >= 32 {
			return raw[:32]
		}
		padded := make([]byte, 32)
		copy(padded, raw)
		return padded
	}
	return decoded
}

// encrypt encrypts plaintext with AES-256-GCM and returns nonce-prepended base64.
func encrypt(plaintext string) (string, error) {
	key := loadEncryptionKey()

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// decrypt reverses encrypt: base64-decode, split nonce, AES-256-GCM open.
func decrypt(b64 string) (string, error) {
	if b64 == "" {
		return "", nil
	}

	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", fmt.Errorf("decode ciphertext: %w", err)
	}

	key := loadEncryptionKey()

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create gcm: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}

	return string(plaintext), nil
}
