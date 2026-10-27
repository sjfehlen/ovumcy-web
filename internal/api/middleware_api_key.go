package api

import (
	"encoding/json"
	"path"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/ovumcy/ovumcy-web/internal/httpx"
	"github.com/ovumcy/ovumcy-web/internal/services"
)

// API key authentication.
//
// A request that carries an Authorization: Bearer header is authenticated by
// that key and by nothing else: the auth cookie it may also carry is never
// consulted, and a key that does not resolve is a 401 that does not fall back to
// the cookie. The two credential kinds therefore never mix inside one request.
//
// A key is accepted on exactly the routes in apiKeyRoutes and nowhere else. That
// table is the whole policy: a route that is not listed refuses every key with a
// 403 whatever its scope, so a route added later is closed to keys until someone
// opens it here on purpose. TestAPIKeyIsRefusedOnEveryRouteOutsideItsTable walks
// the registered routes to hold that.
//
// The same table decides the CSRF skip (IsAPIKeyRequest). A key request carries
// no ambient credential — a browser never attaches an Authorization header to a
// cross-site request on its own — so there is nothing for a forged request to
// ride, and the cookie-bound CSRF token has no meaning for it. The skip is
// limited to the listed routes so it can never stand in for CSRF on a route that
// is not key-reachable.

const (
	contextAPIKeyScopeKey = "current_api_key_scope"

	apiKeyBearerScheme = "bearer"
	apiKeyScopeErrKey  = "api key scope" // #nosec G101 -- error spec key, not a credential.
)

type apiKeyRoute struct {
	method   string
	segments []string
	// minScope is the weakest scope that may use the route. The two scopes are
	// ordered: period includes everything read does.
	minScope string
}

// apiKeyRoutes is the allowlist. ":date" stands for one path segment.
var apiKeyRoutes = []apiKeyRoute{
	{fiber.MethodGet, []string{"api", "v1", "stats", "overview"}, services.APIKeyScopeRead},
	{fiber.MethodGet, []string{"api", "v1", "days"}, services.APIKeyScopeRead},
	{fiber.MethodGet, []string{"api", "v1", "days", ":date"}, services.APIKeyScopeRead},
	{fiber.MethodHead, []string{"api", "v1", "days", ":date"}, services.APIKeyScopeRead},
	{fiber.MethodPost, []string{"api", "v1", "days", ":date", "cycle-start"}, services.APIKeyScopePeriod},
	{fiber.MethodPatch, []string{"api", "v1", "days", ":date"}, services.APIKeyScopePeriod},
}

// apiKeyDayPatchFields are the only members an API-key PATCH of a day may name:
// the period flag and flow, each with its base_ twin. Everything else the day
// carries is out of reach of the period scope.
var apiKeyDayPatchFields = map[string]struct{}{
	"is_period":      {},
	"flow":           {},
	"base_is_period": {},
	"base_flow":      {},
}

// bearerAPIKey returns the credential of an Authorization: Bearer header and
// whether the request presented one at all. A header with another scheme is not
// a key request.
func bearerAPIKey(c fiber.Ctx) (string, bool) {
	header := strings.TrimSpace(c.Get(fiber.HeaderAuthorization))
	scheme, credential, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, apiKeyBearerScheme) {
		return "", false
	}
	return strings.TrimSpace(credential), true
}

// matchAPIKeyRoute finds the table entry for a request, or reports none. It
// decides on the routing-normalised paths it is handed and refuses any spelling the router
// might rewrite before matching (dot or empty segments), the same posture the
// onboarding gate takes: an unusual path is never granted what a plain one is.
func matchAPIKeyRoute(method string, wirePath string, routed string) (apiKeyRoute, bool) {
	// Both paths arrive already routing-normalised (see apiKeyRouteForRequest).
	if wirePath != routed || path.Clean(routed) != routed {
		return apiKeyRoute{}, false
	}
	segments := strings.Split(strings.TrimPrefix(routed, "/"), "/")
	for _, route := range apiKeyRoutes {
		if route.method != method || len(route.segments) != len(segments) {
			continue
		}
		matched := true
		for index, want := range route.segments {
			if segments[index] == "" || (want != ":date" && want != segments[index]) {
				matched = false
				break
			}
		}
		if matched {
			return route, true
		}
	}
	return apiKeyRoute{}, false
}

func apiKeyRouteForRequest(c fiber.Ctx) (apiKeyRoute, bool) {
	wire := httpx.RoutingNormalizedPath(string(c.Request().URI().PathOriginal()))
	routed := httpx.RoutingNormalizedPath(c.Path())
	return matchAPIKeyRoute(c.Method(), wire, routed)
}

// IsAPIKeyRequest reports whether the request is a bearer-key request for a
// route a key may reach. It is the predicate the CSRF middleware skips on.
func IsAPIKeyRequest(c fiber.Ctx) bool {
	if _, presented := bearerAPIKey(c); !presented {
		return false
	}
	_, listed := apiKeyRouteForRequest(c)
	return listed
}

func apiKeyScopeAllows(scope string, required string) bool {
	switch required {
	case services.APIKeyScopeRead:
		return scope == services.APIKeyScopeRead || scope == services.APIKeyScopePeriod
	case services.APIKeyScopePeriod:
		return scope == services.APIKeyScopePeriod
	default:
		return false
	}
}

func apiKeyScopeErrorSpec() APIErrorSpec {
	return globalErrorSpec(fiber.StatusForbidden, APIErrorCategoryForbidden, apiKeyScopeErrKey)
}

// currentAPIKeyScope returns the scope of the key that authenticated this
// request, or false for a request authenticated by session cookie.
func currentAPIKeyScope(c fiber.Ctx) (string, bool) {
	scope, ok := c.Locals(contextAPIKeyScopeKey).(string)
	return scope, ok
}

// authRequiredByAPIKey is AuthRequired for a request that presented a bearer
// credential.
func (handler *Handler) authRequiredByAPIKey(c fiber.Ctx, presented string) error {
	user, scope, ok, err := handler.apiKeyService.ResolveKey(c.Context(), presented)
	if err != nil {
		// A fault is not a refusal: the key may be good.
		handler.logSecurityEvent(c, "auth.api_key", "failure", securityEventField("reason", "key resolve failed"))
		return handler.respondSessionFault(c)
	}
	if !ok {
		handler.logSecurityEvent(c, "auth.api_key", "denied", securityEventField("reason", "invalid key"))
		c.Set(fiber.HeaderWWWAuthenticate, "Bearer")
		return handler.respondGlobalMappedError(c, unauthorizedErrorSpec())
	}

	route, listed := apiKeyRouteForRequest(c)
	if !listed || !apiKeyScopeAllows(scope, route.minScope) {
		spec := apiKeyScopeErrorSpec()
		handler.logSecurityError(c, "access.api_key_scope", spec)
		return handler.respondGlobalMappedError(c, spec)
	}

	c.Locals(contextUserKey, &user)
	c.Locals(contextAPIKeyScopeKey, scope)
	// An owner who has not finished onboarding has no cycle to read or write, and
	// onboarding is a page flow a key has no part in.
	if services.RequiresOnboarding(&user) {
		return handler.respondGlobalMappedError(c, onboardingRequiredErrorSpec())
	}
	handler.resolveOwnerRequestTimezone(c)
	return c.Next()
}

// RestrictAPIKeyDayPatch holds a key-authenticated day PATCH to the period
// fields. A request authenticated by session is untouched: this guards only what
// a period-scoped key may write, and the full-day editor keeps its reach.
func (handler *Handler) RestrictAPIKeyDayPatch(c fiber.Ctx) error {
	if _, byKey := currentAPIKeyScope(c); !byKey {
		return c.Next()
	}
	// A form body or a query string is read by other paths in the day writer
	// than the JSON members checked below, so a key is held to JSON.
	if !hasJSONBody(c) {
		return handler.respondGlobalMappedError(c, invalidPayloadErrorSpec())
	}
	members := map[string]json.RawMessage{}
	if err := json.Unmarshal(c.Body(), &members); err != nil {
		return handler.respondGlobalMappedError(c, invalidPayloadErrorSpec())
	}
	for name := range members {
		if _, allowed := apiKeyDayPatchFields[name]; !allowed {
			spec := apiKeyScopeErrorSpec()
			handler.logSecurityError(c, "access.api_key_scope", spec)
			return handler.respondGlobalMappedError(c, spec)
		}
	}
	return c.Next()
}
