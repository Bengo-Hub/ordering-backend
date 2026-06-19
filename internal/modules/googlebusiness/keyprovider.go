package googlebusiness

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/bengobox/ordering-backend/internal/ent"
	"github.com/bengobox/ordering-backend/internal/ent/serviceconfig"
)

// EncryptionKeyConfigKey is the platform-level ServiceConfig key (tenant_id IS NULL)
// holding the operator-configured AES-256-GCM credential-encryption key, stored as
// base64 of exactly 32 random bytes.
const EncryptionKeyConfigKey = "encryption_key"

// devEncryptionKey is the deterministic development fallback used when neither a DB
// key nor a usable env key is present. It MUST stay constant so data encrypted under
// it in dev remains decryptable. Mirrors the historical loadEncryptionKey() fallback.
const devEncryptionKey = "bengobox-ordering-google-dev-key" // 32 bytes

// keyCacheTTL bounds how long the resolved candidate set is cached before the DB key
// is re-read. Short enough that a freshly-rotated key takes effect quickly even
// without an explicit Invalidate().
const keyCacheTTL = 30 * time.Second

// keySource labels where the PRIMARY (candidate #0) key came from.
type keySource string

const (
	sourceDB   keySource = "db"
	sourceEnv  keySource = "env"
	sourceDev  keySource = "dev"
	sourceNone keySource = ""
)

// keyCandidate is one 32-byte AES key plus its provenance.
type keyCandidate struct {
	key    []byte
	source keySource
}

// resolvedKeys is the cached, ordered, de-duplicated candidate set plus the DB key's
// updated_at (nil when there is no DB key).
type resolvedKeys struct {
	candidates []keyCandidate
	dbUpdated  *time.Time
	expiresAt  time.Time
}

// KeyProvider resolves the ordered list of candidate AES-256 keys used to encrypt and
// decrypt credentials at rest. Resolution order (PRIMARY first):
//
//	#0 DB  — ServiceConfig{config_key=="encryption_key", tenant_id IS NULL}, base64 → 32 bytes
//	#1 ENV — GOOGLE_TOKEN_ENCRYPTION_KEY (base64→32, else raw padded/truncated)
//	#2 DEV — devEncryptionKey padded/truncated to 32
//
// Encryption always uses candidate #0. Decryption tries every candidate in order and
// returns the first success, so ciphertext written under an older key (e.g. the dev key
// before a DB key was configured) stays readable after rotation.
//
// The resolved set is cached for ~30s behind an RWMutex; Invalidate() clears it.
// Key material is never logged.
type KeyProvider struct {
	client *ent.Client

	mu     sync.RWMutex
	cached *resolvedKeys
}

// NewKeyProvider constructs a KeyProvider bound to the given ent client. A nil client is
// tolerated — resolution simply skips the DB candidate.
func NewKeyProvider(client *ent.Client) *KeyProvider {
	return &KeyProvider{client: client}
}

// Invalidate drops the cached candidate set so the next resolution re-reads the DB key.
func (p *KeyProvider) Invalidate() {
	p.mu.Lock()
	p.cached = nil
	p.mu.Unlock()
}

// resolve returns the cached candidate set, recomputing it when absent or expired.
func (p *KeyProvider) resolve(ctx context.Context) *resolvedKeys {
	p.mu.RLock()
	cached := p.cached
	p.mu.RUnlock()
	if cached != nil && time.Now().Before(cached.expiresAt) {
		return cached
	}

	rk := p.compute(ctx)

	p.mu.Lock()
	p.cached = rk
	p.mu.Unlock()
	return rk
}

// compute builds the ordered, de-duplicated candidate set from DB → ENV → DEV.
func (p *KeyProvider) compute(ctx context.Context) *resolvedKeys {
	rk := &resolvedKeys{expiresAt: time.Now().Add(keyCacheTTL)}
	seen := make(map[string]struct{}, 3)

	add := func(key []byte, source keySource) {
		if len(key) != 32 {
			return
		}
		fp := string(key)
		if _, dup := seen[fp]; dup {
			return
		}
		seen[fp] = struct{}{}
		rk.candidates = append(rk.candidates, keyCandidate{key: key, source: source})
	}

	// (a) DB key — PRIMARY when present and valid.
	if dbKey, updated := p.loadDBKey(ctx); dbKey != nil {
		rk.dbUpdated = updated
		add(dbKey, sourceDB)
	}

	// (b) ENV key.
	if envKey := loadEnvKey(); envKey != nil {
		add(envKey, sourceEnv)
	}

	// (c) DEV key — always last.
	add(normalizeKey([]byte(devEncryptionKey)), sourceDev)

	return rk
}

// loadDBKey reads the platform-level encryption_key ServiceConfig and returns the
// decoded 32-byte key plus its updated_at. Returns (nil, nil) when absent or malformed.
// Malformed values are ignored rather than fatal so a bad write can't lock out decryption.
func (p *KeyProvider) loadDBKey(ctx context.Context) ([]byte, *time.Time) {
	if p.client == nil {
		return nil, nil
	}
	cfg, err := p.client.ServiceConfig.Query().
		Where(
			serviceconfig.ConfigKey(EncryptionKeyConfigKey),
			serviceconfig.TenantIDIsNil(),
		).
		First(ctx)
	if err != nil {
		return nil, nil
	}
	decoded, derr := base64.StdEncoding.DecodeString(cfg.ConfigValue)
	if derr != nil || len(decoded) != 32 {
		return nil, nil
	}
	updated := cfg.UpdatedAt
	return decoded, &updated
}

// loadEnvKey normalizes GOOGLE_TOKEN_ENCRYPTION_KEY into a 32-byte key, or nil when unset.
// base64-of-32 is used as-is; otherwise the raw bytes are padded/truncated to 32.
func loadEnvKey() []byte {
	keyStr := os.Getenv(tokenEncryptionKeyEnv)
	if keyStr == "" {
		return nil
	}
	if decoded, err := base64.StdEncoding.DecodeString(keyStr); err == nil && len(decoded) == 32 {
		return decoded
	}
	return normalizeKey([]byte(keyStr))
}

// normalizeKey pads (with zeros) or truncates raw bytes to exactly 32.
func normalizeKey(raw []byte) []byte {
	if len(raw) >= 32 {
		out := make([]byte, 32)
		copy(out, raw[:32])
		return out
	}
	out := make([]byte, 32)
	copy(out, raw)
	return out
}

// PrimaryKey returns the PRIMARY (candidate #0) key and its source. Returns (nil, "")
// only when no candidate at all is resolvable (the dev key guarantees this won't happen
// in practice).
func (p *KeyProvider) PrimaryKey(ctx context.Context) ([]byte, keySource) {
	rk := p.resolve(ctx)
	if len(rk.candidates) == 0 {
		return nil, sourceNone
	}
	c := rk.candidates[0]
	return c.key, c.source
}

// PrimarySource returns the provenance of the PRIMARY key.
func (p *KeyProvider) PrimarySource(ctx context.Context) keySource {
	_, src := p.PrimaryKey(ctx)
	return src
}

// DBUpdatedAt returns the DB key's updated_at, or nil when there is no DB key.
func (p *KeyProvider) DBUpdatedAt(ctx context.Context) *time.Time {
	return p.resolve(ctx).dbUpdated
}

// Fingerprint returns the first 8 hex chars of sha256(PRIMARY key), or "" when no key.
// This is a non-reversible identifier — it never exposes key material.
func (p *KeyProvider) Fingerprint(ctx context.Context) string {
	key, src := p.PrimaryKey(ctx)
	if src == sourceNone || len(key) == 0 {
		return ""
	}
	sum := sha256.Sum256(key)
	return hex.EncodeToString(sum[:])[:8]
}

// Encrypt encrypts plaintext with AES-256-GCM under the PRIMARY key and returns
// nonce-prepended base64.
func (p *KeyProvider) Encrypt(ctx context.Context, plaintext string) (string, error) {
	key, src := p.PrimaryKey(ctx)
	if src == sourceNone {
		return "", fmt.Errorf("no encryption key available")
	}
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

// Decrypt reverses Encrypt, trying each candidate key in order (DB → ENV → DEV) and
// returning the first that successfully opens the ciphertext. This keeps data encrypted
// under an older key readable after rotation. Returns an error only when no candidate
// can decrypt (or the input is structurally invalid).
func (p *KeyProvider) Decrypt(ctx context.Context, b64 string) (string, error) {
	if b64 == "" {
		return "", nil
	}
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", fmt.Errorf("decode ciphertext: %w", err)
	}

	rk := p.resolve(ctx)
	if len(rk.candidates) == 0 {
		return "", fmt.Errorf("no decryption key available")
	}

	var lastErr error
	for _, c := range rk.candidates {
		block, berr := aes.NewCipher(c.key)
		if berr != nil {
			lastErr = berr
			continue
		}
		gcm, gerr := cipher.NewGCM(block)
		if gerr != nil {
			lastErr = gerr
			continue
		}
		nonceSize := gcm.NonceSize()
		if len(data) < nonceSize {
			lastErr = fmt.Errorf("ciphertext too short")
			continue
		}
		nonce, ciphertext := data[:nonceSize], data[nonceSize:]
		plaintext, oerr := gcm.Open(nil, nonce, ciphertext, nil)
		if oerr != nil {
			lastErr = oerr
			continue
		}
		return string(plaintext), nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("decrypt: no candidate key succeeded")
	}
	return "", fmt.Errorf("decrypt: %w", lastErr)
}

// GenerateKey returns 32 cryptographically-random bytes suitable as a new AES-256 key.
func GenerateKey() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}
	return key, nil
}
