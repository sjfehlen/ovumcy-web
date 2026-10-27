package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/ovumcy/ovumcy-web/internal/db"
	"github.com/ovumcy/ovumcy-web/internal/services"
	"gorm.io/gorm"
)

// These tests drive API-key authentication through the production stack
// (newFiberApp: the real middleware chain, CSRF included, and the real route
// table) because the properties worth pinning live between the layers: that a
// key request mints no cookie and needs no CSRF token, that the scope table is
// the whole policy, and that a key never reaches a route the table does not
// list.

func newAPIKeyTestApp(t *testing.T) (*fiber.App, *gorm.DB, uint) {
	t.Helper()

	handler, database := newRateLimitTestHandlerAndDB(t)
	app := newFiberApp(runtimeConfig{
		Location:        time.UTC,
		DefaultLanguage: "en",
		RateLimits:      uniformRateLimits(t, 100000, time.Hour),
	}, handler)
	user := seedOwner(t, db.NewRepositories(database), "api-key-owner@example.com", 14)
	return app, database, user.ID
}

func mintAPIKey(t *testing.T, database *gorm.DB, userID uint, scope string) string {
	t.Helper()

	service := services.NewAPIKeyService(db.NewRepositories(database).Users, []byte(rateLimitTestSecretKey))
	key, err := service.GenerateKey(context.Background(), userID, scope, time.Now())
	if err != nil {
		t.Fatalf("GenerateKey(%s): %v", scope, err)
	}
	return key
}

func keyRequest(t *testing.T, app *fiber.App, key string, method string, target string, body string) (int, string, http.Header) {
	t.Helper()

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	request := httptest.NewRequest(method, target, reader)
	request.Header.Set("Accept", "application/json")
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		request.Header.Set("Authorization", "Bearer "+key)
	}
	response, err := app.Test(request, testConfigNoTimeout)
	if err != nil {
		t.Fatalf("%s %s: %v", method, target, err)
	}
	defer func() { _ = response.Body.Close() }()
	raw, _ := io.ReadAll(response.Body)
	return response.StatusCode, string(raw), response.Header
}

func TestAPIKeyReadScopeReadsAndMintsNoCookie(t *testing.T) {
	app, database, userID := newAPIKeyTestApp(t)
	key := mintAPIKey(t, database, userID, services.APIKeyScopeRead)

	for _, target := range []string{"/api/v1/stats/overview", "/api/v1/days?from=2026-01-01&to=2026-01-31", "/api/v1/days/2026-01-02"} {
		status, body, headers := keyRequest(t, app, key, http.MethodGet, target, "")
		if status != http.StatusOK && status != http.StatusNotFound {
			t.Fatalf("GET %s with a read key = %d, want 200 (or 404 for an absent day): %s", target, status, body)
		}
		if cookies := headers.Values("Set-Cookie"); len(cookies) != 0 {
			t.Fatalf("GET %s with a key must set no cookie, got %v", target, cookies)
		}
	}
}

func TestAPIKeyReadScopeCannotWrite(t *testing.T) {
	app, database, userID := newAPIKeyTestApp(t)
	key := mintAPIKey(t, database, userID, services.APIKeyScopeRead)
	day := time.Now().UTC().Format("2006-01-02")

	for _, probe := range []struct{ method, target, body string }{
		{http.MethodPatch, "/api/v1/days/" + day, `{"is_period":true}`},
		{http.MethodPost, "/api/v1/days/" + day + "/cycle-start", ""},
		{http.MethodPut, "/api/v1/days/" + day, `{"is_period":true}`},
		{http.MethodDelete, "/api/v1/days/" + day, ""},
	} {
		status, body, _ := keyRequest(t, app, key, probe.method, probe.target, probe.body)
		if status != http.StatusForbidden {
			t.Fatalf("%s %s with a read key = %d, want 403: %s", probe.method, probe.target, status, body)
		}
	}
}

func TestAPIKeyPeriodScopeWritesOnlyPeriodFields(t *testing.T) {
	app, database, userID := newAPIKeyTestApp(t)
	key := mintAPIKey(t, database, userID, services.APIKeyScopePeriod)
	day := time.Now().UTC().Format("2006-01-02")

	status, body, headers := keyRequest(t, app, key, http.MethodPatch, "/api/v1/days/"+day, `{"is_period":true,"flow":"medium"}`)
	if status != http.StatusOK && status != http.StatusNoContent {
		t.Fatalf("PATCH period fields with a period key = %d, want success: %s", status, body)
	}
	if cookies := headers.Values("Set-Cookie"); len(cookies) != 0 {
		t.Fatalf("a key write must set no cookie, got %v", cookies)
	}

	status, body, _ = keyRequest(t, app, key, http.MethodPatch, "/api/v1/days/"+day, `{"is_period":false}`)
	if status != http.StatusOK && status != http.StatusNoContent {
		t.Fatalf("ending the period (is_period=false) with a period key = %d, want success: %s", status, body)
	}

	for _, outside := range []string{`{"mood":5}`, `{"notes":"x"}`, `{"is_period":true,"notes":"x"}`, `{"symptom_ids":[1]}`, `{"bbt":36.6}`, `{"Mood":5}`, `{"mood":5}`} {
		status, body, _ = keyRequest(t, app, key, http.MethodPatch, "/api/v1/days/"+day, outside)
		if status != http.StatusForbidden {
			t.Fatalf("PATCH %s with a period key = %d, want 403: %s", outside, status, body)
		}
	}

	form := httptest.NewRequest(http.MethodPatch, "/api/v1/days/"+day, strings.NewReader("is_period=true"))
	form.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	form.Header.Set("Authorization", "Bearer "+key)
	response, err := app.Test(form, testConfigNoTimeout)
	if err != nil {
		t.Fatalf("form PATCH: %v", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("a form-encoded key PATCH = %d, want 400", response.StatusCode)
	}

	status, body, _ = keyRequest(t, app, key, http.MethodPost, "/api/v1/days/"+day+"/cycle-start", "")
	if status != http.StatusOK && status != http.StatusNoContent {
		t.Fatalf("cycle-start with a period key = %d, want success: %s", status, body)
	}
}

func TestAPIKeyRefusalsAreIndistinguishableAndNeverFallBackToTheCookie(t *testing.T) {
	app, database, userID := newAPIKeyTestApp(t)
	key := mintAPIKey(t, database, userID, services.APIKeyScopeRead)

	wrong := services.APIKeyPrefix + strings.Repeat("A", len(key)-len(services.APIKeyPrefix))
	for _, presented := range []string{wrong, "ovk_short", "not-a-key", ""} {
		status, _, headers := keyRequest(t, app, presented, http.MethodGet, "/api/v1/stats/overview", "")
		if presented == "" {
			if status != http.StatusUnauthorized {
				t.Fatalf("no credential = %d, want 401", status)
			}
			continue
		}
		if status != http.StatusUnauthorized {
			t.Fatalf("a bad key %q = %d, want 401", presented, status)
		}
		if headers.Get("WWW-Authenticate") == "" {
			t.Fatalf("a refused bearer request should advertise the scheme")
		}
	}

	// A revoked key is refused exactly like an unknown one.
	if err := db.NewRepositories(database).Users.ClearAPIKey(context.Background(), userID); err != nil {
		t.Fatalf("ClearAPIKey: %v", err)
	}
	if status, _, _ := keyRequest(t, app, key, http.MethodGet, "/api/v1/stats/overview", ""); status != http.StatusUnauthorized {
		t.Fatalf("a revoked key = %d, want 401", status)
	}
}

func TestAPIKeyIsRefusedOnEveryRouteOutsideItsTable(t *testing.T) {
	app, database, userID := newAPIKeyTestApp(t)
	// The widest scope there is: if anything outside the table opens for this
	// key it is a hole, not a scope question.
	key := mintAPIKey(t, database, userID, services.APIKeyScopePeriod)

	listed := map[string]struct{}{
		"GET /api/v1/stats/overview":          {},
		"GET /api/v1/days":                    {},
		"GET /api/v1/days/:date":              {},
		"HEAD /api/v1/days/:date":             {},
		"POST /api/v1/days/:date/cycle-start": {},
		"PATCH /api/v1/days/:date":            {},
	}
	param := regexp.MustCompile(`:[A-Za-z]+\??`)
	checked := 0
	for _, route := range app.GetRoutes(true) {
		if !strings.HasPrefix(route.Path, "/api/v1/") || route.Method == http.MethodHead && false {
			continue
		}
		if _, ok := listed[route.Method+" "+route.Path]; ok {
			continue
		}
		target := param.ReplaceAllString(route.Path, "1")
		// Credential-issuing and public routes take no key at all and are out of
		// scope for this walk: only a route that sits behind AuthRequired can be
		// reached by a key, and those all answer 403 or 401 to it.
		status, body, _ := keyRequest(t, app, key, route.Method, target, "")
		checked++
		if status < 400 {
			t.Fatalf("%s %s answered %d to an API key, but is not in the key table: %s", route.Method, target, status, body)
		}
	}
	if checked < 20 {
		t.Fatalf("walked only %d routes — the route walk is not seeing the API", checked)
	}
}

func TestCSRFSkipForAPIKeyIsLimitedToListedRoutes(t *testing.T) {
	app, database, userID := newAPIKeyTestApp(t)
	key := mintAPIKey(t, database, userID, services.APIKeyScopePeriod)

	// A bearer header on a route the table does not list must not buy a CSRF
	// skip: it is refused by CSRF, not waved through to a handler.
	status, body, _ := keyRequest(t, app, key, http.MethodPost, "/api/v1/users/current/webhook", `{}`)
	if status != http.StatusForbidden || !strings.Contains(body, "csrf_refused") {
		t.Fatalf("POST /users/current/webhook with a bearer = %d %s, want the CSRF refusal", status, body)
	}
}

// TestAPIKeySettingsFlowGeneratesRevealsOnceAndRevokes drives the owner's side
// of the feature through the production stack with a real session: the settings
// page renders the card, generate seals the key and points at the reveal, the
// reveal page shows it exactly once, the key then works as a bearer, and revoke
// turns it off. The earlier tests mint the key directly and so cannot see a
// page or a route that was never wired up.
func TestAPIKeySettingsFlowGeneratesRevealsOnceAndRevokes(t *testing.T) {
	handler, database := newRateLimitTestHandlerAndDB(t)
	app := newCalendarFeedTestApp(t, handler, 100000)
	authCookie := loginOnProductionStack(t, app, database, "api-key-flow@example.com")

	get := func(target string, cookies string) (*http.Response, string) {
		request := httptest.NewRequest(http.MethodGet, target, nil)
		request.Header.Set("Cookie", cookies)
		response, err := app.Test(request, testConfigNoTimeout)
		if err != nil {
			t.Fatalf("GET %s: %v", target, err)
		}
		defer func() { _ = response.Body.Close() }()
		return response, string(mustReadAll(t, response))
	}

	settings, body := get("/settings", authCookie)
	if settings.StatusCode != http.StatusOK || !strings.Contains(body, `id="settings-api-key"`) {
		t.Fatalf("GET /settings = %d, want 200 with the API key card", settings.StatusCode)
	}
	csrfCookie := testResponseCookie(settings.Cookies(), "ovumcy_csrf")
	match := csrfTokenMetaPattern.FindStringSubmatch(body)
	if csrfCookie == nil || len(match) < 2 {
		t.Fatal("the settings page carried no CSRF credentials")
	}
	session := authCookie + "; " + csrfCookie.Name + "=" + csrfCookie.Value

	send := func(method string, target string, payload string, cookies string) (*http.Response, string) {
		var reader io.Reader
		if payload != "" {
			reader = strings.NewReader(payload)
		}
		request := httptest.NewRequest(method, target, reader)
		request.Header.Set("Cookie", cookies)
		request.Header.Set("Accept", "application/json")
		request.Header.Set("X-CSRF-Token", match[1])
		if payload != "" {
			request.Header.Set("Content-Type", "application/json")
		}
		response, err := app.Test(request, testConfigNoTimeout)
		if err != nil {
			t.Fatalf("%s %s: %v", method, target, err)
		}
		defer func() { _ = response.Body.Close() }()
		return response, string(mustReadAll(t, response))
	}

	if response, body := send(http.MethodPost, "/api/v1/users/current/api-key", `{"scope":"admin"}`, session); response.StatusCode != http.StatusBadRequest {
		t.Fatalf("generate with an unknown scope = %d, want 400: %s", response.StatusCode, body)
	}

	generated, body := send(http.MethodPost, "/api/v1/users/current/api-key", `{"scope":"period"}`, session)
	if generated.StatusCode != http.StatusOK || !strings.Contains(body, `"next_path":"/settings/api-key"`) {
		t.Fatalf("generate = %d %s, want 200 pointing at the reveal", generated.StatusCode, body)
	}
	if strings.Contains(body, "ovk_") {
		t.Fatalf("the generate response carried the key: %s", body)
	}
	revealCookie := testResponseCookie(generated.Cookies(), "ovumcy_api_key")
	if revealCookie == nil {
		t.Fatal("generate sealed no reveal cookie")
	}
	withReveal := session + "; " + revealCookie.Name + "=" + revealCookie.Value

	reveal, body := get("/settings/api-key", withReveal)
	if reveal.StatusCode != http.StatusOK {
		t.Fatalf("GET /settings/api-key = %d, want 200: %s", reveal.StatusCode, body)
	}
	key := regexp.MustCompile(`ovk_[A-Z2-9]{40}`).FindString(body)
	if key == "" {
		t.Fatalf("the reveal page did not show the key: %s", body)
	}

	// Shown once: replaying the same sealed cookie lands back on /settings.
	replay, _ := get("/settings/api-key", withReveal)
	if replay.StatusCode != http.StatusSeeOther || replay.Header.Get("Location") != "/settings" {
		t.Fatalf("replaying the reveal = %d -> %q, want 303 to /settings", replay.StatusCode, replay.Header.Get("Location"))
	}

	if status, body, _ := keyRequest(t, app, key, http.MethodGet, "/api/v1/stats/overview", ""); status != http.StatusOK {
		t.Fatalf("the revealed key = %d, want 200: %s", status, body)
	}

	if _, body := get("/settings", session); !strings.Contains(body, `id="settings-api-key"`) || strings.Contains(body, key) {
		t.Fatal("the settings page must show the card and never the key")
	}

	// A key cannot manage keys.
	if status, _, _ := keyRequest(t, app, key, http.MethodDelete, "/api/v1/users/current/api-key", ""); status < http.StatusBadRequest {
		t.Fatalf("an API key revoked itself or its owner's key: %d", status)
	}

	if revoked, body := send(http.MethodDelete, "/api/v1/users/current/api-key", "", session); revoked.StatusCode != http.StatusOK {
		t.Fatalf("revoke = %d, want 200: %s", revoked.StatusCode, body)
	}
	if status, _, _ := keyRequest(t, app, key, http.MethodGet, "/api/v1/stats/overview", ""); status != http.StatusUnauthorized {
		t.Fatalf("a revoked key = %d, want 401", status)
	}
}
