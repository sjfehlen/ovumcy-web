package services

import (
	"testing"
	"time"

	"github.com/ovumcy/ovumcy-web/internal/models"
)

func overviewDay(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func TestDashboardCountdownReadsTheStatusHeadersPublishedDate(t *testing.T) {
	today := overviewDay(2026, time.October, 27)

	exact := buildDashboardCountdown(DashboardCycleContext{DisplayNextPeriodStart: overviewDay(2026, time.November, 1)}, today)
	if !exact.Visible || exact.Min != 5 || exact.Max != 5 || exact.IsRange || exact.Today {
		t.Fatalf("an exact date 5 days out = %+v, want a 5-day countdown", exact)
	}

	// A range stays a range: the median day inside it is never published alone.
	ranged := buildDashboardCountdown(DashboardCycleContext{
		DisplayNextPeriodUseRange:   true,
		DisplayNextPeriodRangeStart: overviewDay(2026, time.October, 30),
		DisplayNextPeriodRangeEnd:   overviewDay(2026, time.November, 3),
		DisplayNextPeriodStart:      overviewDay(2026, time.November, 1),
	}, today)
	if !ranged.Visible || ranged.Min != 3 || ranged.Max != 7 || !ranged.IsRange {
		t.Fatalf("a start window = %+v, want a 3-7 day range", ranged)
	}

	if due := buildDashboardCountdown(DashboardCycleContext{DisplayNextPeriodStart: today}, today); !due.Visible || !due.Today {
		t.Fatalf("a period due today = %+v, want the today sentence", due)
	}
}

func TestDashboardCountdownIsWithheldWhereTheHeaderWithholdsTheDate(t *testing.T) {
	today := overviewDay(2026, time.October, 27)
	future := overviewDay(2026, time.November, 1)

	for name, context := range map[string]DashboardCycleContext{
		"prediction disabled": {PredictionDisabled: true, DisplayNextPeriodStart: future},
		"estimate paused":     {NextPeriodEstimatePaused: true, DisplayNextPeriodStart: future},
		"needs more cycles":   {DisplayNextPeriodNeedsData: true, DisplayNextPeriodStart: future},
		"first-cycle prompt":  {DisplayNextPeriodPrompt: true, DisplayNextPeriodStart: future},
		"date already past":   {NextPeriodInPast: true, DisplayNextPeriodStart: overviewDay(2026, time.October, 20)},
		"no date":             {},
		"range without ends":  {DisplayNextPeriodUseRange: true, DisplayNextPeriodStart: future},
	} {
		if countdown := buildDashboardCountdown(context, today); countdown.Visible {
			t.Errorf("%s: the countdown is shown (%+v) where the status header publishes no next period", name, countdown)
		}
	}
}

func TestDashboardWeekMarksRecordedAndPredictedDaysApart(t *testing.T) {
	today := overviewDay(2026, time.October, 27)
	context := DashboardCycleContext{
		DisplayNextPeriodStart: overviewDay(2026, time.October, 29),
		DisplayNextPeriodEnd:   overviewDay(2026, time.October, 30),
	}
	logs := []models.DailyLog{
		{Date: overviewDay(2026, time.October, 25), IsPeriod: true},
		// A logged day that is not a period day is not marked.
		{Date: overviewDay(2026, time.October, 26), IsPeriod: false},
		// A recorded day inside the predicted span stays a recorded day.
		{Date: overviewDay(2026, time.October, 29), IsPeriod: true},
	}
	countdown := buildDashboardCountdown(context, today)
	week := buildDashboardWeek(context, countdown, logs, today, time.UTC)

	if len(week) != 7 || !week[3].IsToday || week[3].ISODate != "2026-10-27" {
		t.Fatalf("the week = %d days, today at %v; want seven days centred on today", len(week), week[3])
	}
	if week[0].WeekdayKey != "calendar.weekday.sat" {
		t.Fatalf("first weekday key = %q, want calendar.weekday.sat for 2026-10-24", week[0].WeekdayKey)
	}
	byDate := map[string]DashboardWeekDay{}
	for _, day := range week {
		byDate[day.ISODate] = day
	}
	if !byDate["2026-10-25"].IsPeriodLogged || byDate["2026-10-26"].IsPeriodLogged {
		t.Fatal("only the day recorded as a period day is marked recorded")
	}
	if byDate["2026-10-29"].IsPeriodPredicted || !byDate["2026-10-29"].IsPeriodLogged {
		t.Fatal("a recorded day inside the predicted span must stay recorded, not predicted")
	}
	if !byDate["2026-10-30"].IsPeriodPredicted || byDate["2026-10-28"].IsPeriodPredicted {
		t.Fatal("exactly the predicted span is marked predicted")
	}

	withheld := buildDashboardWeek(DashboardCycleContext{PredictionDisabled: true, DisplayNextPeriodStart: context.DisplayNextPeriodStart, DisplayNextPeriodEnd: context.DisplayNextPeriodEnd}, DashboardCountdown{}, nil, today, time.UTC)
	for _, day := range withheld {
		if day.IsPeriodPredicted || day.IsStartWindow {
			t.Fatalf("%s is marked predicted although the prediction is withheld", day.ISODate)
		}
	}
}

func TestDashboardWeekDrawsTheStartWindowAsAWindow(t *testing.T) {
	today := overviewDay(2026, time.October, 27)
	context := DashboardCycleContext{
		DisplayNextPeriodUseRange:   true,
		DisplayNextPeriodRangeStart: overviewDay(2026, time.October, 28),
		DisplayNextPeriodRangeEnd:   overviewDay(2026, time.October, 29),
		DisplayNextPeriodStart:      overviewDay(2026, time.October, 28),
		DisplayNextPeriodEnd:        overviewDay(2026, time.November, 2),
	}
	week := buildDashboardWeek(context, buildDashboardCountdown(context, today), nil, today, time.UTC)
	for _, day := range week {
		if day.IsPeriodPredicted {
			t.Fatalf("%s: a start window is drawn as predicted period days; the median span must not be", day.ISODate)
		}
	}
	if !week[4].IsStartWindow || !week[5].IsStartWindow || week[3].IsStartWindow || week[6].IsStartWindow {
		t.Fatal("exactly the start window's days are marked")
	}
}

func TestDashboardPhaseRingIsDrawnOnlyFromAVisibleHero(t *testing.T) {
	if ring := buildDashboardPhaseRing(DashboardCycleHero{}); ring.Visible || len(ring.Arcs) != 0 {
		t.Fatalf("a hero the page cannot map produced a ring: %+v", ring)
	}

	ring := buildDashboardPhaseRing(DashboardCycleHero{
		Visible:      true,
		CycleLength:  28,
		CurrentDay:   1,
		CurrentPhase: "menstrual",
		PhaseCards: []DashboardCycleHeroPhaseCard{
			{Phase: "menstrual", StartDay: 1, EndDay: 7},
			{Phase: "follicular", StartDay: 8, EndDay: 13},
			{Phase: "luteal", StartDay: 14, EndDay: 28},
		},
	})
	if !ring.Visible || len(ring.Arcs) != 3 {
		t.Fatalf("ring = %+v, want three arcs", ring)
	}
	// 7 of 28 days is 25%, less the visual gap between arcs.
	if ring.Arcs[0].Dash != "24.20" || ring.Arcs[0].Offset != "-0.00" && ring.Arcs[0].Offset != "0.00" {
		t.Fatalf("first arc = %+v, want a 24.20 dash starting at the top", ring.Arcs[0])
	}
	if ring.Arcs[1].Offset != "-25.00" {
		t.Fatalf("second arc offset = %q, want -25.00 (it starts a quarter of the way round)", ring.Arcs[1].Offset)
	}
	if !ring.HasMarker || ring.MarkerX == "" || ring.MarkerY == "" {
		t.Fatalf("today has no marker: %+v", ring)
	}

	if noDay := buildDashboardPhaseRing(DashboardCycleHero{Visible: true, CycleLength: 28, PhaseCards: []DashboardCycleHeroPhaseCard{{Phase: "menstrual", StartDay: 1, EndDay: 5}}}); noDay.HasMarker {
		t.Fatal("a ring with no current day drew a marker")
	}
}
