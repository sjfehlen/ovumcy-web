package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/ovumcy/ovumcy-web/internal/models"
	"github.com/ovumcy/ovumcy-web/internal/services"
)

func getPageWithCookie(t *testing.T, app *fiber.App, path string, authCookie string) *http.Response {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.Header.Set("Accept-Language", "en")
	request.Header.Set("Cookie", authCookie)
	return mustAppResponse(t, app, request)
}

// The Pregnancy tab exists only while a pregnancy is declared: it is in the
// layout's navigation then, and the page itself answers a redirect otherwise.
func TestPregnancyTabAndPageFollowTheDeclaredPregnancy(t *testing.T) {
	app, database := newOnboardingTestApp(t)
	user := createOnboardingTestUser(t, database, "pregnancy-page@example.com", "StrongPass1", true)
	authCookie := loginAndExtractAuthCookie(t, app, user.Email, "StrongPass1")

	hasTab := func(markup string) bool { return strings.Contains(markup, `data-nav-link="pregnancy"`) }

	if markup := mustRenderDashboard(t, app, authCookie, "en"); hasTab(markup) {
		t.Fatal("no pregnancy is declared: the navigation must not carry a Pregnancy tab")
	}
	response := getPageWithCookie(t, app, "/pregnancy", authCookie)
	assertStatusCode(t, response, http.StatusSeeOther)
	if location := response.Header.Get("Location"); location != "/dashboard" {
		t.Fatalf("expected a redirect to /dashboard, got %q", location)
	}

	// 36 days in (week 5, day 1): due 244 days from today.
	today := services.DateAtLocation(time.Now().UTC(), time.UTC)
	due := services.AddCalendarDays(today, services.PregnancyTermDays-36, time.UTC)
	if err := database.Model(&models.User{}).Where("id = ?", user.ID).Update("pregnancy_due_date", due).Error; err != nil {
		t.Fatalf("declare pregnancy: %v", err)
	}

	markup := mustRenderDashboard(t, app, authCookie, "en")
	if !hasTab(markup) {
		t.Fatal("a pregnancy is declared: the navigation must carry the Pregnancy tab")
	}
	// The dashboard hero spells the countdown with its number.
	if !strings.Contains(markup, "244 days until the due date") {
		t.Fatalf("expected the hero to carry the day count %q", "244 days until the due date")
	}

	response = getPageWithCookie(t, app, "/pregnancy", authCookie)
	assertStatusCode(t, response, http.StatusOK)
	page := mustReadBodyString(t, response.Body)
	for _, hook := range []string{"data-pregnancy-page", "data-pregnancy-timeline", "data-pregnancy-journal-empty"} {
		if !strings.Contains(page, hook) {
			t.Fatalf("expected the pregnancy page to carry %s", hook)
		}
	}
	if got := strings.Count(page, `data-pregnancy-week-state="current"`); got != 1 {
		t.Fatalf("expected exactly one current week, got %d", got)
	}
	if !strings.Contains(page, `data-pregnancy-week="5" data-pregnancy-week-state="current"`) {
		t.Fatal("expected week 5 to be the current week")
	}
	if strings.Contains(page, `data-pregnancy-week="3"`) {
		t.Fatal("the timeline starts at the first sized week")
	}
}
