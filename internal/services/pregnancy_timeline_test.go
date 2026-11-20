package services

import (
	"testing"
	"time"

	"github.com/ovumcy/ovumcy-web/internal/models"
)

func TestBuildPregnancyPageViewTracksTheJournalPerWeek(t *testing.T) {
	today := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	// 6w2d in: week 6 is current, weeks 4-5 are past, the rest upcoming.
	due := AddCalendarDays(today, PregnancyTermDays-(6*7+2), time.UTC)
	status := ResolvePregnancyStatus(&models.User{PregnancyDueDate: &due}, today)
	if status.Weeks != 6 {
		t.Fatalf("setup: expected week 6, got %d", status.Weeks)
	}
	day := func(offset int, mood int) models.DailyLog {
		return models.DailyLog{Date: AddCalendarDays(status.LastPeriodStart, offset, time.UTC), Mood: mood}
	}
	logs := []models.DailyLog{
		day(-3, 5),  // before the pregnancy: ignored
		day(28, 2),  // week 4
		day(29, 4),  // week 4
		day(30, 0),  // week 4, no mood
		day(43, 3),  // week 6
		day(100, 5), // after today: ignored
	}

	view := BuildPregnancyPageView(status, logs, "en")

	if got := view.Weeks[0].Week; got != 4 {
		t.Fatalf("timeline starts at week 4, got %d", got)
	}
	if got := len(view.Weeks); got != 37 {
		t.Fatalf("expected weeks 4-40, got %d rows", got)
	}
	states := map[int]string{}
	for _, week := range view.Weeks {
		states[week.Week] = week.State
	}
	if states[5] != PregnancyTimelineWeekPast || states[6] != PregnancyTimelineWeekCurrent || states[7] != PregnancyTimelineWeekUpcoming {
		t.Fatalf("unexpected states: 5=%s 6=%s 7=%s", states[5], states[6], states[7])
	}
	week4 := view.Weeks[0]
	if week4.DaysLogged != 3 || week4.MoodDays != 2 || week4.MoodAverage != 3 || week4.MoodAverageText != "3" {
		t.Fatalf("week 4 journal: %+v", week4)
	}
	if view.Summary.DaysLogged != 4 || view.Summary.MoodDays != 3 || view.Summary.MoodAverageText != "3" {
		t.Fatalf("summary: %+v", view.Summary)
	}
}

func TestBuildPregnancyPageViewRunsPastTheDueDateWeek(t *testing.T) {
	today := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	due := AddCalendarDays(today, -10, time.UTC) // 41w3d
	status := ResolvePregnancyStatus(&models.User{PregnancyDueDate: &due}, today)
	view := BuildPregnancyPageView(status, nil, "de")
	last := view.Weeks[len(view.Weeks)-1]
	if last.Week != 41 || last.State != PregnancyTimelineWeekCurrent {
		t.Fatalf("expected the current week 41 to close the timeline, got %+v", last)
	}
	if got := BuildPregnancyPageView(PregnancyStatus{}, nil, "en"); len(got.Weeks) != 0 {
		t.Fatal("an inactive pregnancy has no timeline")
	}
}
