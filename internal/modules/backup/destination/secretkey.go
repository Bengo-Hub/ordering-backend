package destination

import (
	"context"
	"crypto/sha256"
	"os"
)

// SecretKeyCipher derives a single AES-256 key from the SECRET_KEY environment
// variable via SHA-256 (always 32 bytes). It satisfies the Cipher interface and
// is the uniform key source for the ordering backup-destination Store: PrimaryKey
// and the sole CandidateKey are both the derived key. No credential material is
// ever logged.
type SecretKeyCipher struct {
	key [32]byte
}

// NewSecretKeyCipher builds a Cipher from sha256([]byte(os.Getenv("SECRET_KEY"))).
// SECRET_KEY may be empty in local/dev environments; the derived key is still a
// valid 32-byte AES key (the JSON fallback in decrypt covers dev/migration data).
func NewSecretKeyCipher() *SecretKeyCipher {
	return &SecretKeyCipher{key: sha256.Sum256([]byte(os.Getenv("SECRET_KEY")))}
}

// PrimaryKey returns the 32-byte SECRET_KEY-derived key used to encrypt new data.
func (c *SecretKeyCipher) PrimaryKey(_ context.Context) []byte {
	k := c.key
	return k[:]
}

// CandidateKeys returns the single derived key for decryption.
func (c *SecretKeyCipher) CandidateKeys(_ context.Context) [][]byte {
	k := c.key
	return [][]byte{k[:]}
}
