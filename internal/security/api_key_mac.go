package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"io"

	"golang.org/x/crypto/hkdf"
)

// HKDF label pair for the API key authenticator. It is its own purpose, so the
// derived key is unrelated to the sealed-cookie, field-encryption and
// calendar-feed keys: a leak or rotation of one never affects another. Bumping
// the version suffix, like rotating SECRET_KEY, invalidates every stored
// authenticator and so disarms every issued API key.
const (
	apiKeyMACSaltLabel = "ovumcy.api-key.salt.v1"
	apiKeyMACInfoLabel = "ovumcy.api-key.key.v1"
)

// ErrAPIKeyMACKeyMissing is returned when no secret key is available to derive
// the authenticator key from. Callers treat it as a hard failure and never fall
// back to an unkeyed digest.
var ErrAPIKeyMACKeyMissing = errors.New("api key mac: secret key missing")

// APIKeyMAC derives the keyed authenticator stored in place of an API key. The
// key carries 200 bits from crypto/rand, so no work factor is needed to resist
// guessing, and keying the digest means a database leak alone cannot be used to
// check guesses offline.
func APIKeyMAC(secretKey []byte, apiKey string) (string, error) {
	if len(secretKey) == 0 {
		return "", ErrAPIKeyMACKeyMissing
	}
	macKey := make([]byte, sha256.Size)
	reader := hkdf.New(sha256.New, secretKey, []byte(apiKeyMACSaltLabel), []byte(apiKeyMACInfoLabel))
	if _, err := io.ReadFull(reader, macKey); err != nil {
		return "", err // codecov:ignore -- hkdf over sha256 cannot short-read this size
	}
	mac := hmac.New(sha256.New, macKey)
	_, _ = mac.Write([]byte(apiKey))
	return hex.EncodeToString(mac.Sum(nil)), nil
}

// VerifyAPIKeyMAC recomputes the authenticator and compares it in constant time.
// A missing key, an empty stored value and any mismatch all report false.
func VerifyAPIKeyMAC(secretKey []byte, apiKey string, storedMAC string) bool {
	if storedMAC == "" {
		return false
	}
	computed, err := APIKeyMAC(secretKey, apiKey)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(computed), []byte(storedMAC)) == 1
}
