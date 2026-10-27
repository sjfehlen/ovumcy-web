package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ovumcy/ovumcy-web/internal/models"
	"github.com/ovumcy/ovumcy-web/internal/security"
)

// Per-account API key. A bearer credential for non-browser clients (a Home
// Assistant integration, a script) that signs in nothing and holds no session:
// each request names the key and is authorised by it alone.
//
// One key per owner, with a scope fixed at mint time. Changing the scope means
// minting a new key, which retires the old one — so a key can never quietly
// gain reach it was not issued with. The key is shown to the owner exactly once
// and only a keyed MAC of it is stored (security.APIKeyMAC), the same posture as
// the calendar feed token and the recovery code.

const (
	// APIKeyScopeRead lets the key read the owner's cycle data and nothing else.
	APIKeyScopeRead = "read"
	// APIKeyScopePeriod is APIKeyScopeRead plus the two period writes: marking a
	// cycle start and setting or clearing the period flag (and its flow) on a
	// day. It cannot touch any other field, setting, export or the account.
	APIKeyScopePeriod = "period"

	// APIKeyPrefix is the fixed, non-secret start of every key. It lets a human
	// or a secret scanner recognise a leaked key for what it is, and lets the
	// request path discard anything else without a lookup.
	APIKeyPrefix = "ovk_"

	// apiKeySecretLength is the random part in characters. Over the 32-symbol
	// alphabet below that is 200 bits, well above the 128-bit secret floor.
	apiKeySecretLength = 40
	// APIKeyLength is the total length of a well-formed key.
	APIKeyLength = len(APIKeyPrefix) + apiKeySecretLength
)

var (
	// ErrAPIKeyScopeInvalid is returned for a scope that is not one of the two.
	ErrAPIKeyScopeInvalid = errors.New("api key scope invalid")
	// ErrAPIKeyGenerate wraps a key-generation failure.
	ErrAPIKeyGenerate = errors.New("api key generate")
	// ErrAPIKeyPersist wraps a repository write or read failure.
	ErrAPIKeyPersist = errors.New("api key persist")
)

// ParseAPIKeyScope reports whether raw names a scope, returning the canonical
// value. It accepts nothing else, so an unrecognised scope never becomes a
// default.
func ParseAPIKeyScope(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case APIKeyScopeRead:
		return APIKeyScopeRead, true
	case APIKeyScopePeriod:
		return APIKeyScopePeriod, true
	default:
		return "", false
	}
}

// IsAPIKeyShape reports whether token could be an API key: the fixed prefix and
// the exact length over the key alphabet. It is a cheap filter in front of the
// MAC, not a credential check.
func IsAPIKeyShape(token string) bool {
	if len(token) != APIKeyLength || !strings.HasPrefix(token, APIKeyPrefix) {
		return false
	}
	for _, char := range token[len(APIKeyPrefix):] {
		if !strings.ContainsRune(calendarFeedAlphabet, char) {
			return false
		}
	}
	return true
}

// GenerateAPIKey mints a fresh key and the keyed authenticator to store. The
// plaintext is returned once and is not recoverable from the authenticator.
func GenerateAPIKey(secretKey []byte) (key string, mac string, err error) {
	secret, err := security.RandomString(apiKeySecretLength, calendarFeedAlphabet)
	if err != nil {
		return "", "", err // codecov:ignore -- crypto/rand failure, not reachable in tests
	}
	key = APIKeyPrefix + secret
	mac, err = security.APIKeyMAC(secretKey, key)
	if err != nil {
		return "", "", err
	}
	return key, mac, nil
}

// APIKeyRepository is the narrow persistence seam the API key lifecycle needs.
// None of it bumps auth_session_version: a key is a per-surface capability, not
// a login session.
type APIKeyRepository interface {
	SaveAPIKey(ctx context.Context, userID uint, mac string, scope string, createdAt time.Time) error
	ClearAPIKey(ctx context.Context, userID uint) error
	FindByAPIKeyMAC(ctx context.Context, mac string) (models.User, bool, error)
	ClaimAPIKeyReveal(ctx context.Context, userID uint, revealedAt time.Time) (bool, error)
	LoadAPIKeyStatus(ctx context.Context, userID uint) (models.User, error)
}

// APIKeyService owns the owner-driven lifecycle of the key (mint, revoke, the
// shown-once reveal claim) and the request-time resolution of a presented key
// to its owner.
type APIKeyService struct {
	users     APIKeyRepository
	secretKey []byte
}

// NewAPIKeyService wires the service from the user repository and the
// application secret that keys the authenticator.
func NewAPIKeyService(users APIKeyRepository, secretKey []byte) *APIKeyService {
	return &APIKeyService{users: users, secretKey: secretKey}
}

// APIKeyStatus is the render-safe projection of an owner's key. It carries no
// key and no authenticator, so the settings page can describe the key without
// any secret reaching it.
type APIKeyStatus struct {
	Configured bool
	Scope      string
	CreatedAt  time.Time
}

// GenerateKey mints a key with the given scope for the owner, replacing any
// previous one (which stops authenticating the moment the write commits), and
// returns the plaintext for a single reveal. now is the mint time shown on the
// settings card.
func (service *APIKeyService) GenerateKey(ctx context.Context, userID uint, scope string, now time.Time) (string, error) {
	canonical, ok := ParseAPIKeyScope(scope)
	if !ok {
		return "", ErrAPIKeyScopeInvalid
	}
	if userID == 0 {
		return "", fmt.Errorf("%w: api key requires an owner id", ErrAPIKeyPersist)
	}
	key, mac, err := GenerateAPIKey(service.secretKey)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrAPIKeyGenerate, err)
	}
	if err := service.users.SaveAPIKey(ctx, userID, mac, canonical, now.UTC()); err != nil {
		return "", fmt.Errorf("%w: %v", ErrAPIKeyPersist, err)
	}
	return key, nil
}

// RevokeKey disables the owner's key. Idempotent.
func (service *APIKeyService) RevokeKey(ctx context.Context, userID uint) error {
	if err := service.users.ClearAPIKey(ctx, userID); err != nil {
		return fmt.Errorf("%w: %v", ErrAPIKeyPersist, err)
	}
	return nil
}

// ClaimKeyReveal consumes the owner's one-time reveal and reports whether this
// call is the one that got it. See CalendarFeedSettingsService.ClaimFeedReveal
// for why the server-side mark, and not the cookie retraction, is what makes
// "shown once" true.
func (service *APIKeyService) ClaimKeyReveal(ctx context.Context, userID uint, revealedAt time.Time) (bool, error) {
	if userID == 0 {
		return false, fmt.Errorf("%w: api key reveal requires an owner id", ErrAPIKeyPersist)
	}
	claimed, err := service.users.ClaimAPIKeyReveal(ctx, userID, revealedAt)
	if err != nil {
		return false, fmt.Errorf("%w: %v", ErrAPIKeyPersist, err)
	}
	return claimed, nil
}

// BuildKeyStatus reads the owner's key status. A read fault is reported as an
// error so the page can say it could not tell, rather than claiming no key.
func (service *APIKeyService) BuildKeyStatus(ctx context.Context, userID uint) (APIKeyStatus, error) {
	user, err := service.users.LoadAPIKeyStatus(ctx, userID)
	if err != nil {
		return APIKeyStatus{}, fmt.Errorf("%w: %v", ErrAPIKeyPersist, err)
	}
	status := APIKeyStatus{Configured: user.APIKeyHash != "", Scope: user.APIKeyScope}
	if status.Configured && user.APIKeyCreatedAt != nil {
		status.CreatedAt = *user.APIKeyCreatedAt
	}
	return status, nil
}

// ResolveKey maps a presented key to its owner and scope. ok is false for every
// refusal — a malformed key, an unknown key, a revoked key — and the three are
// indistinguishable to the caller. A storage fault is returned as an error and
// is never read as a refusal.
func (service *APIKeyService) ResolveKey(ctx context.Context, presented string) (user models.User, scope string, ok bool, err error) {
	if !IsAPIKeyShape(presented) {
		return models.User{}, "", false, nil
	}
	mac, err := security.APIKeyMAC(service.secretKey, presented)
	if err != nil {
		return models.User{}, "", false, err
	}
	found, exists, err := service.users.FindByAPIKeyMAC(ctx, mac)
	if err != nil {
		return models.User{}, "", false, fmt.Errorf("%w: %v", ErrAPIKeyPersist, err)
	}
	// The lookup is by the authenticator, so a hit already proves a match. The
	// constant-time compare is kept as the decision so that a repository that
	// returned the wrong row can never authenticate a request.
	if !exists || !security.VerifyAPIKeyMAC(service.secretKey, presented, found.APIKeyHash) {
		return models.User{}, "", false, nil
	}
	scope, valid := ParseAPIKeyScope(found.APIKeyScope)
	if !valid {
		return models.User{}, "", false, nil
	}
	return found, scope, true, nil
}
