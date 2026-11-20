package services

import (
	"github.com/ovumcy/ovumcy-web/internal/models"
)

// Timeline week states: a week is behind the owner, the one in progress, or ahead.
const (
	PregnancyTimelineWeekPast     = "past"
	PregnancyTimelineWeekCurrent  = "current"
	PregnancyTimelineWeekUpcoming = "upcoming"
)

// pregnancyTimelineLastWeek is the last week the timeline lists: the due-date
// week, or the current week when the pregnancy runs past it.
const pregnancyTimelineLastWeek = 40

// PregnancyTimelineWeek is one row of the pregnancy page: the reference entry
// for the week plus what the owner logged during it. Logged figures are zero
// for a week that has not started.
type PregnancyTimelineWeek struct {
	Week          int
	State         string
	FruitKey      string
	Length        PregnancyLength
	DevKey        string
	MilestoneKeys []string
	// DaysLogged counts the days of the week that carry a journal row.
	DaysLogged int
	// MoodDays counts the logged days that carry a mood rating.
	MoodDays int
	// MoodAverage is the mean mood rating over MoodDays, 0 when none was rated.
	MoodAverage float64
	// MoodAverageText is MoodAverage spelled with the language's decimal separator.
	MoodAverageText string
}

// PregnancyJournalSummary totals what was logged since the pregnancy's implied
// first day.
type PregnancyJournalSummary struct {
	DaysLogged      int
	MoodDays        int
	MoodAverage     float64
	MoodAverageText string
}

// PregnancyPageView is everything the pregnancy page renders beyond the status.
type PregnancyPageView struct {
	Weeks   []PregnancyTimelineWeek
	Summary PregnancyJournalSummary
}

// BuildPregnancyPageView lays the pregnancy out week by week from the first
// sized week, adding the owner's journal figures for every week already begun.
// It reads no stored data of its own: the logs are the ones the caller fetched
// for LastPeriodStart..today.
func BuildPregnancyPageView(status PregnancyStatus, logs []models.DailyLog, language string) PregnancyPageView {
	if !status.Active {
		return PregnancyPageView{}
	}

	type weekTotals struct {
		days, moodDays, moodSum int
	}
	perWeek := map[int]*weekTotals{}
	var total weekTotals
	for _, entry := range logs {
		offset := CalendarDaysBetween(status.LastPeriodStart, dateOnly(entry.Date))
		if offset < 0 || offset > status.GestationalDays {
			continue
		}
		week := offset / 7
		totals := perWeek[week]
		if totals == nil {
			totals = &weekTotals{}
			perWeek[week] = totals
		}
		totals.days++
		total.days++
		if entry.Mood > 0 {
			totals.moodDays++
			totals.moodSum += entry.Mood
			total.moodDays++
			total.moodSum += entry.Mood
		}
	}

	last := pregnancyTimelineLastWeek
	if status.Weeks > last {
		last = status.Weeks
	}
	if last > pregnancyLastSizedWeek {
		last = pregnancyLastSizedWeek
	}

	weeks := make([]PregnancyTimelineWeek, 0, last-pregnancyFirstSizedWeek+1)
	for week := pregnancyFirstSizedWeek; week <= last; week++ {
		info, _ := PregnancyWeekInfoFor(week)
		row := PregnancyTimelineWeek{
			Week:          week,
			State:         PregnancyTimelineWeekUpcoming,
			FruitKey:      info.FruitKey,
			Length:        FormatPregnancyLength(info.LengthCM, language),
			DevKey:        info.DevKey,
			MilestoneKeys: PregnancyMilestoneKeys(week),
		}
		switch {
		case week < status.Weeks:
			row.State = PregnancyTimelineWeekPast
		case week == status.Weeks:
			row.State = PregnancyTimelineWeekCurrent
		}
		if totals := perWeek[week]; totals != nil {
			row.DaysLogged = totals.days
			row.MoodDays = totals.moodDays
			if totals.moodDays > 0 {
				row.MoodAverage = float64(totals.moodSum) / float64(totals.moodDays)
				row.MoodAverageText = formatMoodAverage(row.MoodAverage, language)
			}
		}
		weeks = append(weeks, row)
	}

	summary := PregnancyJournalSummary{DaysLogged: total.days, MoodDays: total.moodDays}
	if total.moodDays > 0 {
		summary.MoodAverage = float64(total.moodSum) / float64(total.moodDays)
		summary.MoodAverageText = formatMoodAverage(summary.MoodAverage, language)
	}
	return PregnancyPageView{Weeks: weeks, Summary: summary}
}

func formatMoodAverage(value float64, language string) string {
	separator := ","
	if language == "en" {
		separator = "."
	}
	return formatDecimal(value, separator)
}
