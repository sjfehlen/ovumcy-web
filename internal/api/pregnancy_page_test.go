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

// The page totals what was logged since the pregnancy began, overall and per
// week, and leaves out days from before it.
func TestPregnancyPageTotalsTheJournalSincePregnancyBegan(t *testing.T) {
	app, database := newOnboardingTestApp(t)
	user := createOnboardingTestUser(t, database, "pregnancy-journal@example.com", "StrongPass1", true)
	authCookie := loginAndExtractAuthCookie(t, app, user.Email, "StrongPass1")

	today := services.DateAtLocation(time.Now().UTC(), time.UTC)
	due := services.AddCalendarDays(today, services.PregnancyTermDays-36, time.UTC)
	if err := database.Model(&models.User{}).Where("id = ?", user.ID).Update("pregnancy_due_date", due).Error; err != nil {
		t.Fatalf("declare pregnancy: %v", err)
	}
	start := services.AddCalendarDays(due, -services.PregnancyTermDays, time.UTC)
	for _, entry := range []models.DailyLog{
		{UserID: user.ID, Date: services.AddCalendarDays(start, -2, time.UTC), Mood: 5}, // before: ignored
		{UserID: user.ID, Date: services.AddCalendarDays(start, 29, time.UTC), Mood: 2}, // week 4
		{UserID: user.ID, Date: services.AddCalendarDays(start, 30, time.UTC), Mood: 4}, // week 4
	} {
		if err := database.Create(&entry).Error; err != nil {
			t.Fatalf("seed log: %v", err)
		}
	}

	response := getPageWithCookie(t, app, "/pregnancy", authCookie)
	assertStatusCode(t, response, http.StatusOK)
	page := mustReadBodyString(t, response.Body)
	if strings.Contains(page, "data-pregnancy-journal-empty") {
		t.Fatal("two days are logged: the empty journal wording must not render")
	}
	if got := strings.Count(page, "Days logged: 2"); got != 2 { // overall + week 4
		t.Fatalf("expected the total and week 4 to read %q, found %d", "Days logged: 2", got)
	}
	if got := strings.Count(page, "Average mood: 3"); got != 3 { // total, week 4 and its bar label
		t.Fatalf("expected the total, week 4 and its bar label to read %q, found %d", "Average mood: 3", got)
	}
}
