package services

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/ovumcy/ovumcy-web/internal/models"
)

// The dashboard overview is the picture above the journal: the countdown to the
// next period, the week around today, and a ring that shows where today sits in
// the cycle. It adds no new claim. Every number and every marked day is read off
// what the status header already publishes (DashboardCycleContext and
// DashboardCycleHero), under the same gates, so a prediction the page withholds
// is withheld here too, and a start window stays a range rather than a day.
//
// The ring is drawn from attributes, not styles: the page runs under a strict
// CSP, so the geometry is computed here and rendered as SVG presentation
// attributes. Every arc is measured against pathLength="100", which keeps the
// dash lengths in percent of the cycle whatever the radius.

// dashboardWeekRadiusDays is how many days the week strip shows on each side of
// today.
const dashboardWeekRadiusDays = 3

const (
	dashboardRingCenter = 50.0
	dashboardRingRadius = 42.0
	// dashboardRingArcGap is the visual break left between two arcs, in percent
	// of the cycle.
	dashboardRingArcGap = 0.8
)

// DashboardOverview is everything the overview renders.
type DashboardOverview struct {
	Countdown DashboardCountdown
	Week      []DashboardWeekDay
	Ring      DashboardPhaseRing
}

// DashboardCountdown is the days until the next period. Min and Max are equal for
// an exact date and span the start window when the dashboard shows a range.
type DashboardCountdown struct {
	Visible bool
	Min     int
	Max     int
	IsRange bool
	// Today is set when the period is expected today, which reads as a sentence
	// rather than as the number zero.
	Today bool
}

// DashboardWeekDay is one cell of the week strip.
type DashboardWeekDay struct {
	Date       time.Time
	DayNumber  int
	WeekdayKey string
	ISODate    string
	IsToday    bool
	// IsPeriodLogged is a recorded period day; IsPeriodPredicted and
	// IsStartWindow are estimates, and the two kinds are drawn differently so a
	// fact is never mistaken for an estimate.
	IsPeriodLogged    bool
	IsPeriodPredicted bool
	IsStartWindow     bool
}

// DashboardPhaseRing is the donut. Arcs are in cycle order.
type DashboardPhaseRing struct {
	Visible     bool
	CurrentDay  int
	CycleLength int
	Phase       string
	Arcs        []DashboardPhaseRingArc
	HasMarker   bool
	MarkerX     string
	MarkerY     string
}

// DashboardPhaseRingArc is one phase's stretch of the ring, as SVG attribute
// values.
type DashboardPhaseRingArc struct {
	Phase  string
	Dash   string
	Gap    string
	Offset string
}

// BuildDashboardOverview derives the overview from the context and the hero the
// page already built. hero.Visible gates the ring, so a cycle the page cannot
// honestly map gets no ring.
func BuildDashboardOverview(cycleContext DashboardCycleContext, hero DashboardCycleHero, logs []models.DailyLog, today time.Time, location *time.Location) DashboardOverview {
	if location == nil {
		location = time.UTC
	}
	countdown := buildDashboardCountdown(cycleContext, today)
	return DashboardOverview{
		Countdown: countdown,
		Week:      buildDashboardWeek(cycleContext, countdown, logs, today, location),
		Ring:      buildDashboardPhaseRing(hero),
	}
}

// dashboardNextPeriodPublished reports whether the status header names a next
// period at all. It mirrors the header's own branches: a disabled prediction, a
// paused estimate, a withheld date and a date already past all publish none.
func dashboardNextPeriodPublished(cycleContext DashboardCycleContext) bool {
	if cycleContext.PredictionDisabled || cycleContext.NextPeriodEstimatePaused ||
		cycleContext.DisplayNextPeriodNeedsData || cycleContext.DisplayNextPeriodPrompt ||
		cycleContext.NextPeriodInPast {
		return false
	}
	if cycleContext.DisplayNextPeriodUseRange {
		return !cycleContext.DisplayNextPeriodRangeStart.IsZero() && !cycleContext.DisplayNextPeriodRangeEnd.IsZero()
	}
	return !cycleContext.DisplayNextPeriodStart.IsZero()
}

func buildDashboardCountdown(cycleContext DashboardCycleContext, today time.Time) DashboardCountdown {
	if !dashboardNextPeriodPublished(cycleContext) {
		return DashboardCountdown{}
	}
	if cycleContext.DisplayNextPeriodUseRange {
		minDays := max(CalendarDaysBetween(today, cycleContext.DisplayNextPeriodRangeStart), 0)
		maxDays := max(CalendarDaysBetween(today, cycleContext.DisplayNextPeriodRangeEnd), minDays)
		return DashboardCountdown{Visible: true, Min: minDays, Max: maxDays, IsRange: maxDays != minDays, Today: maxDays == 0}
	}
	days := max(CalendarDaysBetween(today, cycleContext.DisplayNextPeriodStart), 0)
	return DashboardCountdown{Visible: true, Min: days, Max: days, Today: days == 0}
}

func buildDashboardWeek(cycleContext DashboardCycleContext, countdown DashboardCountdown, logs []models.DailyLog, today time.Time, location *time.Location) []DashboardWeekDay {
	loggedPeriod := make(map[string]struct{}, len(logs))
	for _, log := range logs {
		if log.IsPeriod {
			loggedPeriod[CalendarDay(log.Date, location).Format("2006-01-02")] = struct{}{}
		}
	}

	week := make([]DashboardWeekDay, 0, dashboardWeekRadiusDays*2+1)
	for offset := -dashboardWeekRadiusDays; offset <= dashboardWeekRadiusDays; offset++ {
		day := AddCalendarDays(today, offset, location)
		iso := day.Format("2006-01-02")
		cell := DashboardWeekDay{
			Date:       day,
			DayNumber:  day.Day(),
			WeekdayKey: "calendar.weekday." + strings.ToLower(day.Weekday().String()[:3]),
			ISODate:    iso,
			IsToday:    offset == 0,
		}
		_, cell.IsPeriodLogged = loggedPeriod[iso]
		if countdown.Visible && !cell.IsPeriodLogged {
			if cycleContext.DisplayNextPeriodUseRange {
				cell.IsStartWindow = dashboardDayWithin(day, cycleContext.DisplayNextPeriodRangeStart, cycleContext.DisplayNextPeriodRangeEnd)
			} else {
				cell.IsPeriodPredicted = dashboardDayWithin(day, cycleContext.DisplayNextPeriodStart, cycleContext.DisplayNextPeriodEnd)
			}
		}
		week = append(week, cell)
	}
	return week
}

// dashboardDayWithin compares calendar days, so a time-of-day on either side
// cannot move a day in or out of the span.
func dashboardDayWithin(day time.Time, start time.Time, end time.Time) bool {
	if start.IsZero() {
		return false
	}
	if end.IsZero() || end.Before(start) {
		end = start
	}
	key := day.Format("2006-01-02")
	return key >= start.Format("2006-01-02") && key <= end.Format("2006-01-02")
}

func buildDashboardPhaseRing(hero DashboardCycleHero) DashboardPhaseRing {
	if !hero.Visible || hero.CycleLength <= 0 || len(hero.PhaseCards) == 0 {
		return DashboardPhaseRing{}
	}
	total := float64(hero.CycleLength)
	ring := DashboardPhaseRing{
		Visible:     true,
		CurrentDay:  hero.CurrentDay,
		CycleLength: hero.CycleLength,
		Phase:       hero.CurrentPhase,
	}
	for _, card := range hero.PhaseCards {
		if card.EndDay < card.StartDay {
			continue
		}
		span := float64(card.EndDay-card.StartDay+1) / total * 100
		start := float64(card.StartDay-1) / total * 100
		dash := math.Max(span-dashboardRingArcGap, 0.1)
		ring.Arcs = append(ring.Arcs, DashboardPhaseRingArc{
			Phase:  card.Phase,
			Dash:   fmt.Sprintf("%.2f", dash),
			Gap:    fmt.Sprintf("%.2f", 100-dash),
			Offset: fmt.Sprintf("%.2f", -start),
		})
	}
	if len(ring.Arcs) == 0 {
		return DashboardPhaseRing{}
	}
	if hero.CurrentDay > 0 && hero.CurrentDay <= hero.CycleLength {
		angle := (float64(hero.CurrentDay)-0.5)/total*2*math.Pi - math.Pi/2
		ring.HasMarker = true
		ring.MarkerX = fmt.Sprintf("%.2f", dashboardRingCenter+dashboardRingRadius*math.Cos(angle))
		ring.MarkerY = fmt.Sprintf("%.2f", dashboardRingCenter+dashboardRingRadius*math.Sin(angle))
	}
	return ring
}
