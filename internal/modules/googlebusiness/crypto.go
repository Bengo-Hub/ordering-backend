// Package googlebusiness implements a Google Business Profile (GBP) integration:
// OAuth connect/callback, encrypted token storage, location selection, and reviews.
//
// The whole module is INERT until the operator sets the OAuth env vars
// (GOOGLE_OAUTH_CLIENT_ID/SECRET/REDIRECT_URL). When unset, IsConfigured() is false
// and the service returns ErrNotConfigured so handlers can answer 503 without crashing.
//
// Credential encryption at rest (OAuth token JSON) uses AES-256-GCM under a key
// resolved by KeyProvider (DB-first, env-fallback, dev-last). See keyprovider.go.
package googlebusiness

// tokenEncryptionKeyEnv is the env var holding the base64-encoded 32-byte AES-256 key
// used to encrypt OAuth token JSON at rest. It is the ENV-fallback candidate in
// KeyProvider's resolution order. Mirrors treasury-api's loadEncryptionKey.
//
// In production the operator-configured DB key (platform ServiceConfig "encryption_key")
// takes precedence; this env var remains a fallback and a decryption candidate so data
// written before a DB key was set stays readable.
const tokenEncryptionKeyEnv = "GOOGLE_TOKEN_ENCRYPTION_KEY"
