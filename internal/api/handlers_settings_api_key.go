package api

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/ovumcy/ovumcy-web/internal/services"
)

// API key — settings lifecycle and one-time reveal.
//
// Two OwnerOnly, CSRF-protected endpoints manage the owner's key:
//   - POST   /api/v1/users/current/api-key → GenerateAPIKey (mint, or replace)
//   - DELETE /api/v1/users/current/api-key → RevokeAPIKey
//
// Neither is reachable with an API key (middleware_api_key.go lists the only
// routes a key may use), so a leaked key cannot mint, replace or revoke keys.
//
// SECRET HANDLING mirrors the calendar feed. Generate NEVER returns the key in
// a JSON or HTML body or a redirect query: it seals the key into a one-time
// cookie and redirects to a dedicated reveal page that shows it exactly once,
// the once-ness decided by users.api_key_revealed_at (migration 042) and not by
// the cookie retraction. Revoke carries no secret. Nothing here logs the key.

var (
	apiKeyGenerateMutation = accountMutationKind{action: "settings.api_key_generate", target: "api_key"}
	apiKeyRevokeMutation   = accountMutationKind{action: "settings.api_key_revoke", target: "api_key"}
)

// apiKeyRevealEgress tags the one-time reveal of the key. The key is a standing
// capability over the owner's cycle data, so the audited moment is the one where
// it reaches a person; the key never enters the line.
var apiKeyRevealEgress = healthEgressKind{action: "settings.api_key_reveal", target: "api_key"}

// apiKeyRevealPath is the dedicated one-time reveal page.
const apiKeyRevealPath = "/settings/api-key" // #nosec G101 -- URL path of the reveal page, not a credential.

const (
	apiKeyRevealCookieName = "ovumcy_api_key" // #nosec G101 -- cookie name contains "key" but is not a secret.
	apiKeyRevealCookieTTL  = 20 * time.Minute
)

var apiKeyRevealCookieSpec = sealedCookieSpec{name: apiKeyRevealCookieName, path: "/"}

type apiKeyInput struct {
	Scope string `json:"scope" form:"scope"`
}

type apiKeyRevealPayload struct {
	UserID    uint      `json:"uid"`
	Key       string    `json:"key"`
	Scope     string    `json:"scope"`
	Rotated   bool      `json:"rotated,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`
}

type apiKeyRevealState struct {
	Key     string
	Scope   string
	Rotated bool
}

func settingsAPIKeyUpdateErrorSpec() APIErrorSpec {
	return globalErrorSpec(fiber.StatusInternalServerError, APIErrorCategoryInternal, "failed to update api key")
}

func settingsAPIKeyInvalidScopeErrorSpec() APIErrorSpec {
	return globalErrorSpec(fiber.StatusBadRequest, APIErrorCategoryValidation, "invalid api key scope")
}

// GenerateAPIKey mints a key with the chosen scope, replacing any previous one
// (which stops working at once), seals it for a single reveal and redirects to
// the reveal page.
func (handler *Handler) GenerateAPIKey(c fiber.Ctx) error {
	user, ok := currentUser(c)
	if !ok {
		return handler.failAccountMutation(c, apiKeyGenerateMutation, unauthorizedErrorSpec())
	}

	var input apiKeyInput
	if err := bindRequestBody(c, &input); err != nil {
		return handler.failAccountMutation(c, apiKeyGenerateMutation, settingsAPIKeyInvalidScopeErrorSpec())
	}

	// Whether this replaces a key is read before the write so the reveal page can
	// say which it is. A read fault is not allowed to decide it: the page copy
	// would be wrong, so the mint is refused instead.
	before, err := handler.apiKeyService.BuildKeyStatus(c.Context(), user.ID)
	if err != nil {
		return handler.failAccountMutation(c, apiKeyGenerateMutation, settingsAPIKeyUpdateErrorSpec())
	}

	key, err := handler.apiKeyService.GenerateKey(c.Context(), user.ID, input.Scope, handler.clockNow())
	if err != nil {
		if errors.Is(err, services.ErrAPIKeyScopeInvalid) {
			return handler.failAccountMutation(c, apiKeyGenerateMutation, settingsAPIKeyInvalidScopeErrorSpec())
		}
		return handler.failAccountMutation(c, apiKeyGenerateMutation, settingsAPIKeyUpdateErrorSpec())
	}
	scope, _ := services.ParseAPIKeyScope(input.Scope)
	if err := handler.setAPIKeyRevealCookie(c, user.ID, key, scope, before.Configured); err != nil {
		return handler.failAccountMutation(c, apiKeyGenerateMutation, settingsAPIKeyUpdateErrorSpec())
	}

	handler.logAccountMutationSuccess(c, apiKeyGenerateMutation, securityEventField("scope", scope))

	if acceptsJSON(c) {
		// JSON clients are told where the one-time reveal is, never handed the key.
		return c.JSON(fiber.Map{
			"ok":        true,
			"scope":     scope,
			"next_step": "api_key_reveal",
			"next_path": apiKeyRevealPath,
		})
	}
	return redirectToPath(c, apiKeyRevealPath)
}

// RevokeAPIKey turns the owner's key off. It carries no secret.
func (handler *Handler) RevokeAPIKey(c fiber.Ctx) error {
	user, ok := currentUser(c)
	if !ok {
		return handler.failAccountMutation(c, apiKeyRevokeMutation, unauthorizedErrorSpec())
	}
	if err := handler.apiKeyService.RevokeKey(c.Context(), user.ID); err != nil {
		return handler.failAccountMutation(c, apiKeyRevokeMutation, settingsAPIKeyUpdateErrorSpec())
	}
	handler.logAccountMutationSuccess(c, apiKeyRevokeMutation)

	const status = "api_key_revoked"
	if acceptsJSON(c) {
		return c.JSON(fiber.Map{"ok": true, "status": status})
	}
	handler.setFlashCookie(c, FlashPayload{SettingsSuccess: status})
	return redirectOrJSON(c, "/settings")
}

// ShowAPIKeyRevealPage renders the key exactly once: it reads the sealed
// cookie, CLAIMS the owner's reveal mark, clears the cookie and shows the key. A
// refresh, a later visit or a replay of the sealed value finds the mark already
// claimed and redirects back to /settings.
func (handler *Handler) ShowAPIKeyRevealPage(c fiber.Ctx) error {
	user, ok := currentUser(c)
	if !ok {
		return c.Redirect().Status(fiber.StatusSeeOther).To("/login")
	}

	state := handler.readAPIKeyRevealState(c, user.ID)
	if state.Key == "" {
		return c.Redirect().Status(fiber.StatusSeeOther).To("/settings")
	}

	claimed, err := handler.apiKeyService.ClaimKeyReveal(c.Context(), user.ID, time.Now())
	if err != nil || !claimed {
		if err != nil {
			handler.logEgressFailure(c, apiKeyRevealEgress, "reveal_claim_failed") // codecov:ignore -- the claim errors only on a storage fault, which AuthRequired's own user load reaches first
		} else {
			handler.logEgressDenied(c, apiKeyRevealEgress, "reveal_already_claimed")
		}
		handler.clearAPIKeyRevealCookie(c)
		return c.Redirect().Status(fiber.StatusSeeOther).To("/settings")
	}
	handler.clearAPIKeyRevealCookie(c)
	handler.logEgressSuccess(c, apiKeyRevealEgress)

	return handler.render(c, "api_key_reveal", fiber.Map{
		"Title":          localizedPageTitle(currentMessages(c), "meta.title.api_key", "Ovumcy | API key"),
		"APIKey":         state.Key,
		"APIKeyScope":    state.Scope,
		"APIKeyRotated":  state.Rotated,
		"HideNavigation": true,
	})
}

// refuseAPIKeyRevealRequest is the first-party guard's exit for
// GET /settings/api-key, and deliberately does not clear the reveal cookie, for
// the reason refuseCalendarFeedRevealRequest gives.
func (handler *Handler) refuseAPIKeyRevealRequest(c fiber.Ctx, reason string) error {
	if strings.TrimSpace(c.Cookies(apiKeyRevealCookieName)) != "" {
		handler.logEgressDenied(c, apiKeyRevealEgress, reason)
	}
	return c.Redirect().Status(fiber.StatusSeeOther).To("/settings")
}

func (handler *Handler) setAPIKeyRevealCookie(c fiber.Ctx, userID uint, key string, scope string, rotated bool) error {
	if userID == 0 {
		handler.clearAPIKeyRevealCookie(c)
		return errors.New("api key reveal requires an owner id")
	}
	if strings.TrimSpace(key) == "" {
		handler.clearAPIKeyRevealCookie(c)
		return errors.New("api key is required")
	}
	expiresAt := time.Now().Add(apiKeyRevealCookieTTL)
	serialized, err := json.Marshal(apiKeyRevealPayload{UserID: userID, Key: key, Scope: scope, Rotated: rotated, ExpiresAt: expiresAt})
	if err != nil {
		return err // codecov:ignore -- json.Marshal of a fixed struct does not fail
	}
	return handler.writeSealedCookie(c, apiKeyRevealCookieSpec, serialized, expiresAt)
}

// readAPIKeyRevealState opens the sealed cookie and returns the key, or an empty
// state when it is absent, malformed, scoped to another owner or past the expiry
// it carries. It does not decide single use — the caller claims the mark.
func (handler *Handler) readAPIKeyRevealState(c fiber.Ctx, userID uint) apiKeyRevealState {
	raw := strings.TrimSpace(c.Cookies(apiKeyRevealCookieName))
	if raw == "" {
		return apiKeyRevealState{}
	}
	decoded, err := handler.openCookieValue(apiKeyRevealCookieName, raw)
	if err != nil {
		handler.clearAPIKeyRevealCookie(c)
		return apiKeyRevealState{}
	}
	payload := apiKeyRevealPayload{}
	if err := json.Unmarshal(decoded, &payload); err != nil ||
		strings.TrimSpace(payload.Key) == "" ||
		!sealedPayloadBelongsToSession(payload.UserID, userID) ||
		time.Now().After(payload.ExpiresAt) {
		handler.clearAPIKeyRevealCookie(c)
		return apiKeyRevealState{}
	}
	return apiKeyRevealState{Key: strings.TrimSpace(payload.Key), Scope: payload.Scope, Rotated: payload.Rotated}
}

func (handler *Handler) clearAPIKeyRevealCookie(c fiber.Ctx) {
	handler.clearSealedCookie(c, apiKeyRevealCookieSpec)
}
