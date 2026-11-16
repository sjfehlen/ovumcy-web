package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/ovumcy/ovumcy-web/internal/models"
)

func pregnancyTestDay(offsetDays int) string {
	return time.Now().UTC().AddDate(0, 0, offsetDays).Format("2006-01-02")
}

func pregnancyGET(t *testing.T, ctx settingsSecurityTestContext, path string) string {
	t.Helper()

	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.Header.Set("Cookie", settingsCookieHeader(ctx.authCookie, ctx.csrfCookie))
	request.Header.Set("Accept-Language", "en")
	response, err := ctx.app.Test(request, testConfigNoTimeout)
	if err != nil {
		t.Fatalf("GET %s failed: %v", path, err)
	}
	defer func() { _ = response.Body.Close() }()
	assertStatusCode(t, response, http.StatusOK)
	return mustReadBodyString(t, response.Body)
}

func startPregnancyJSON(t *testing.T, ctx settingsSecurityTestContext, form url.Values) *http.Response {
	t.Helper()
	return settingsFormRequestWithCSRF(t, ctx, http.MethodPut, "/api/v1/users/current/pregnancy", form, map[string]string{"Accept": "application/json"})
}

func loadPregnancyColumns(t *testing.T, ctx settingsSecurityTestContext, userID uint) models.User {
	t.Helper()
	persisted := models.User{}
	if err := ctx.database.Select("pregnancy_due_date", "pregnancy_ended_on", "webhook_config_version").First(&persisted, userID).Error; err != nil {
		t.Fatalf("load pregnancy columns: %v", err)
	}
	return persisted
}

// TestPregnancyStartPersistsDueDateAndPausesPredictions drives the declaration
// end to end: the due date is stored, the write response and the stats overview
// both report the derived status, and cycle predictions are paused.
func TestPregnancyStartPersistsDueDateAndPausesPredictions(t *testing.T) {
	ctx := newSettingsSecurityTestContext(t, "pregnancy-start@example.com")
	due := pregnancyTestDay(140)

	response := startPregnancyJSON(t, ctx, url.Values{"due_date": {due}})
	defer func() { _ = response.Body.Close() }()
	assertStatusCode(t, response, http.StatusOK)

	var body struct {
		OK        bool   `json:"ok"`
		Status    string `json:"status"`
		Pregnancy struct {
			Active       bool   `json:"active"`
			DueDate      string `json:"due_date"`
			Trimester    int    `json:"trimester"`
			DaysUntilDue int    `json:"days_until_due"`
		} `json:"pregnancy"`
	}
	if err := json.Unmarshal([]byte(mustReadBodyString(t, response.Body)), &body); err != nil {
		t.Fatalf("decode start response: %v", err)
	}
	if !body.OK || body.Status != "pregnancy_started" || !body.Pregnancy.Active || body.Pregnancy.DueDate != due {
		t.Fatalf("unexpected start response: %+v", body)
	}
	if body.Pregnancy.DaysUntilDue != 140 || body.Pregnancy.Trimester != 2 {
		t.Fatalf("expected 140 days left in trimester 2, got %+v", body.Pregnancy)
	}

	persisted := loadPregnancyColumns(t, ctx, ctx.user.ID)
	if persisted.PregnancyDueDate == nil || persisted.PregnancyDueDate.Format("2006-01-02") != due {
		t.Fatalf("expected stored due date %s, got %v", due, persisted.PregnancyDueDate)
	}

	overview := pregnancyGET(t, ctx, "/api/v1/stats/overview")
	var stats struct {
		PregnancyPaused bool `json:"pregnancy_paused"`
		Pregnancy       struct {
			Active  bool   `json:"active"`
			DueDate string `json:"due_date"`
		} `json:"pregnancy"`
	}
	if err := json.Unmarshal([]byte(overview), &stats); err != nil {
		t.Fatalf("decode overview: %v", err)
	}
	if !stats.PregnancyPaused || !stats.Pregnancy.Active || stats.Pregnancy.DueDate != due {
		t.Fatalf("expected a paused, active pregnancy in the overview, got %s", overview)
	}
}

// TestPregnancyStartFromLastPeriodDerivesDueDate proves the last-period anchor
// stores the 280-day due date, and that an explicit due date wins over it.
func TestPregnancyStartFromLastPeriodDerivesDueDate(t *testing.T) {
	ctx := newSettingsSecurityTestContext(t, "pregnancy-lmp@example.com")

	response := startPregnancyJSON(t, ctx, url.Values{"last_period_start": {pregnancyTestDay(-60)}})
	defer func() { _ = response.Body.Close() }()
	assertStatusCode(t, response, http.StatusOK)

	want := pregnancyTestDay(-60 + 280)
	persisted := loadPregnancyColumns(t, ctx, ctx.user.ID)
	if persisted.PregnancyDueDate == nil || persisted.PregnancyDueDate.Format("2006-01-02") != want {
		t.Fatalf("expected due date %s from the last period, got %v", want, persisted.PregnancyDueDate)
	}

	explicit := pregnancyTestDay(200)
	second := startPregnancyJSON(t, ctx, url.Values{"due_date": {explicit}, "last_period_start": {pregnancyTestDay(-60)}})
	defer func() { _ = second.Body.Close() }()
	assertStatusCode(t, second, http.StatusOK)
	persisted = loadPregnancyColumns(t, ctx, ctx.user.ID)
	if persisted.PregnancyDueDate == nil || persisted.PregnancyDueDate.Format("2006-01-02") != explicit {
		t.Fatalf("expected the explicit due date %s to win, got %v", explicit, persisted.PregnancyDueDate)
	}
}

func TestPregnancyStartRejectsUnusableDates(t *testing.T) {
	cases := map[string]url.Values{
		"missing":          {},
		"malformed":        {"due_date": {"not-a-date"}},
		"past the window":  {"due_date": {pregnancyTestDay(-60)}},
		"beyond full term": {"due_date": {pregnancyTestDay(400)}},
	}
	for name, form := range cases {
		t.Run(name, func(t *testing.T) {
			ctx := newSettingsSecurityTestContext(t, "pregnancy-bad-"+strings.ReplaceAll(name, " ", "-")+"@example.com")

			response := startPregnancyJSON(t, ctx, form)
			defer func() { _ = response.Body.Close() }()
			assertStatusCode(t, response, http.StatusBadRequest)

			if persisted := loadPregnancyColumns(t, ctx, ctx.user.ID); persisted.PregnancyDueDate != nil {
				t.Fatalf("a refused declaration must not write, got %v", persisted.PregnancyDueDate)
			}
		})
	}
}

// TestPregnancyEndClosesAndLiftsPause proves ending clears the due date, records
// the ending day, resumes predictions, and answers 409 on a second submit.
func TestPregnancyEndClosesAndLiftsPause(t *testing.T) {
	ctx := newSettingsSecurityTestContext(t, "pregnancy-end@example.com")
	start := startPregnancyJSON(t, ctx, url.Values{"due_date": {pregnancyTestDay(90)}})
	_ = start.Body.Close()

	response := settingsFormRequestWithCSRF(t, ctx, http.MethodDelete, "/api/v1/users/current/pregnancy", url.Values{}, map[string]string{"Accept": "application/json"})
	defer func() { _ = response.Body.Close() }()
	assertStatusCode(t, response, http.StatusOK)

	persisted := loadPregnancyColumns(t, ctx, ctx.user.ID)
	if persisted.PregnancyDueDate != nil || persisted.PregnancyEndedOn == nil {
		t.Fatalf("expected an ended pregnancy, got due=%v ended=%v", persisted.PregnancyDueDate, persisted.PregnancyEndedOn)
	}

	overview := pregnancyGET(t, ctx, "/api/v1/stats/overview")
	if !strings.Contains(overview, `"pregnancy":{"active":false}`) || !strings.Contains(overview, `"pregnancy_paused":false`) {
		t.Fatalf("expected predictions to resume with no active pregnancy, got %s", overview)
	}

	again := settingsFormRequestWithCSRF(t, ctx, http.MethodDelete, "/api/v1/users/current/pregnancy", url.Values{}, map[string]string{"Accept": "application/json"})
	defer func() { _ = again.Body.Close() }()
	assertStatusCode(t, again, http.StatusConflict)
}

// TestPregnancyWritesAdvanceTheWebhookRevocationEpoch pins that a declaration and
// an ending both advance webhook_config_version, so a notify pass holding a
// snapshot from before either cannot still deliver a reminder.
func TestPregnancyWritesAdvanceTheWebhookRevocationEpoch(t *testing.T) {
	ctx := newSettingsSecurityTestContext(t, "pregnancy-epoch@example.com")
	before := loadPregnancyColumns(t, ctx, ctx.user.ID).WebhookConfigVersion

	start := startPregnancyJSON(t, ctx, url.Values{"due_date": {pregnancyTestDay(90)}})
	_ = start.Body.Close()
	afterStart := loadPregnancyColumns(t, ctx, ctx.user.ID).WebhookConfigVersion
	if afterStart != before+1 {
		t.Fatalf("expected the epoch to advance on start: %d -> %d", before, afterStart)
	}

	end := settingsFormRequestWithCSRF(t, ctx, http.MethodDelete, "/api/v1/users/current/pregnancy", url.Values{}, map[string]string{"Accept": "application/json"})
	_ = end.Body.Close()
	if afterEnd := loadPregnancyColumns(t, ctx, ctx.user.ID).WebhookConfigVersion; afterEnd != afterStart+1 {
		t.Fatalf("expected the epoch to advance on end: %d -> %d", afterStart, afterEnd)
	}
}

// TestPregnancyWritesAreScopedToTheSessionOwner proves a second owner's pregnancy
// is never touched by this session's start or end.
func TestPregnancyWritesAreScopedToTheSessionOwner(t *testing.T) {
	ctx := newSettingsSecurityTestContext(t, "pregnancy-owner@example.com")
	other := createOnboardingTestUser(t, ctx.database, "pregnancy-bystander@example.com", "StrongPass1", true)
	bystanderDue := time.Now().UTC().AddDate(0, 0, 50)
	if err := ctx.database.Model(&models.User{}).Where("id = ?", other.ID).Update("pregnancy_due_date", bystanderDue).Error; err != nil {
		t.Fatalf("seed bystander pregnancy: %v", err)
	}

	end := settingsFormRequestWithCSRF(t, ctx, http.MethodDelete, "/api/v1/users/current/pregnancy", url.Values{}, map[string]string{"Accept": "application/json"})
	defer func() { _ = end.Body.Close() }()
	assertStatusCode(t, end, http.StatusConflict)

	if persisted := loadPregnancyColumns(t, ctx, other.ID); persisted.PregnancyDueDate == nil || persisted.PregnancyEndedOn != nil {
		t.Fatalf("the bystander's pregnancy must be untouched, got due=%v ended=%v", persisted.PregnancyDueDate, persisted.PregnancyEndedOn)
	}
}

// TestPregnancyRoutesRefuseAnAnonymousRequest pins that both routes sit behind
// the session gate.
func TestPregnancyRoutesRefuseAnAnonymousRequest(t *testing.T) {
	ctx := newSettingsSecurityTestContext(t, "pregnancy-anon@example.com")

	for _, method := range []string{http.MethodPut, http.MethodDelete} {
		request := httptest.NewRequest(method, "/api/v1/users/current/pregnancy", strings.NewReader("due_date="+pregnancyTestDay(90)))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request.Header.Set("Accept", "application/json")
		response, err := ctx.app.Test(request, testConfigNoTimeout)
		if err != nil {
			t.Fatalf("%s request failed: %v", method, err)
		}
		_ = response.Body.Close()
		if response.StatusCode == http.StatusOK {
			t.Fatalf("%s without a session must not succeed", method)
		}
	}
}

// TestPregnancySurfacesRenderForADeclaredPregnancy proves the dashboard shows the
// pregnancy card in the hero slot and the settings card shows the active state,
// and that neither appears when no pregnancy is declared.
func TestPregnancySurfacesRenderForADeclaredPregnancy(t *testing.T) {
	ctx := newSettingsSecurityTestContext(t, "pregnancy-render@example.com")

	if dashboard := pregnancyGET(t, ctx, "/dashboard"); strings.Contains(dashboard, "data-dashboard-pregnancy") {
		t.Fatal("the pregnancy card must not render without a declared pregnancy")
	}
	if settings := pregnancyGET(t, ctx, "/settings"); strings.Contains(settings, "data-settings-pregnancy-active") {
		t.Fatal("the active pregnancy summary must not render without a declared pregnancy")
	}

	start := startPregnancyJSON(t, ctx, url.Values{"due_date": {pregnancyTestDay(100)}})
	_ = start.Body.Close()

	if dashboard := pregnancyGET(t, ctx, "/dashboard"); !strings.Contains(dashboard, "data-dashboard-pregnancy") {
		t.Fatal("expected the pregnancy card on the dashboard")
	}
	dashboard := pregnancyGET(t, ctx, "/dashboard")
	if !strings.Contains(dashboard, "data-dashboard-pregnancy-week") || !strings.Contains(dashboard, "Roughly the size of:") {
		t.Fatal("expected the week card with a size comparison on the dashboard")
	}
	if settings := pregnancyGET(t, ctx, "/settings"); !strings.Contains(settings, "data-settings-pregnancy-active") {
		t.Fatal("expected the active pregnancy summary in settings")
	}
}

// TestPregnancyOverviewCarriesTheWeeklySize proves the JSON overview names the
// week's size comparison and length once the pregnancy reaches week 4, and
// leaves both out before it.
func TestPregnancyOverviewCarriesTheWeeklySize(t *testing.T) {
	ctx := newSettingsSecurityTestContext(t, "pregnancy-size@example.com")

	// 280 - 90 = 190 days to the due date puts today at gestational day 90: week 12.
	start := startPregnancyJSON(t, ctx, url.Values{"due_date": {pregnancyTestDay(190)}})
	_ = start.Body.Close()
	overview := pregnancyGET(t, ctx, "/api/v1/stats/overview")
	if !strings.Contains(overview, `"size_comparison":"lime"`) || !strings.Contains(overview, `"length_cm":5.4`) || !strings.Contains(overview, `"weeks":12`) {
		t.Fatalf("expected week 12 to be a lime of 5.4 cm, got %s", overview)
	}

	early := startPregnancyJSON(t, ctx, url.Values{"due_date": {pregnancyTestDay(260)}})
	_ = early.Body.Close()
	overview = pregnancyGET(t, ctx, "/api/v1/stats/overview")
	if strings.Contains(overview, "size_comparison") || strings.Contains(overview, "length_cm") {
		t.Fatalf("week 2 has nothing to compare, got %s", overview)
	}
}
