package services

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestPregnancyWeekTableCoversEveryWeekFromFourToFortyTwoInOrder(t *testing.T) {
	if len(pregnancyWeekTable) != pregnancyLastSizedWeek-pregnancyFirstSizedWeek+1 {
		t.Fatalf("table has %d entries", len(pregnancyWeekTable))
	}
	previous := 0.0
	for index, entry := range pregnancyWeekTable {
		if entry.Week != pregnancyFirstSizedWeek+index {
			t.Fatalf("entry %d is week %d", index, entry.Week)
		}
		if entry.LengthCM <= previous && entry.Week != pregnancyLastSizedWeek-1 && entry.Week != pregnancyLastSizedWeek {
			t.Fatalf("week %d length %.1f does not grow from %.1f", entry.Week, entry.LengthCM, previous)
		}
		if entry.FruitKey != "pregnancy.fruit."+entry.FruitID || entry.DevKey != "pregnancy.week."+strconv.Itoa(entry.Week)+".dev" {
			t.Fatalf("week %d keys do not follow the naming scheme: %+v", entry.Week, entry)
		}
		previous = entry.LengthCM
	}
}

// Every key the table, the milestones and the length formatter name must exist
// in every shipped catalogue, or the card would render a bare key.
func TestPregnancyWeekKeysExistInEveryLocale(t *testing.T) {
	keys := []string{"pregnancy.length_cm", "pregnancy.length_mm", "pregnancy.size_label", "pregnancy.week_title", "pregnancy.milestone_title", "pregnancy.week_disclaimer"}
	for _, entry := range pregnancyWeekTable {
		keys = append(keys, entry.FruitKey, entry.DevKey)
	}
	for _, milestone := range pregnancyMilestones {
		keys = append(keys, milestone.key)
	}
	files, err := filepath.Glob(filepath.Join("..", "i18n", "locales", "*.json"))
	if err != nil || len(files) != 6 {
		t.Fatalf("expected six catalogues, got %v %v", files, err)
	}
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		catalogue := map[string]string{}
		if err := json.Unmarshal(raw, &catalogue); err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		for _, key := range keys {
			if catalogue[key] == "" {
				t.Errorf("%s is missing %q", filepath.Base(file), key)
			}
		}
	}
}

func TestPregnancyWeekInfoForBoundaries(t *testing.T) {
	if _, ok := PregnancyWeekInfoFor(3); ok {
		t.Fatal("week 3 has nothing to compare")
	}
	if info, ok := PregnancyWeekInfoFor(4); !ok || info.FruitID != "poppy_seed" {
		t.Fatalf("week 4: %+v %t", info, ok)
	}
	if info, ok := PregnancyWeekInfoFor(12); !ok || info.FruitID != "lime" {
		t.Fatalf("week 12: %+v %t", info, ok)
	}
	if info, ok := PregnancyWeekInfoFor(42); !ok || info.Week != 42 {
		t.Fatalf("week 42: %+v %t", info, ok)
	}
	if info, ok := PregnancyWeekInfoFor(45); !ok || info.Week != 42 {
		t.Fatalf("a pregnancy past week 42 keeps the last entry, got %+v %t", info, ok)
	}
}

func TestPregnancyMilestoneKeysByWeek(t *testing.T) {
	cases := map[int][]string{
		9:  {},
		10: {"pregnancy.milestone.first_trimester_screen"},
		14: {"pregnancy.milestone.first_trimester_screen"},
		15: {},
		20: {"pregnancy.milestone.anatomy_scan"},
		26: {"pregnancy.milestone.glucose"},
		29: {},
		35: {"pregnancy.milestone.gbs"},
		36: {"pregnancy.milestone.gbs", "pregnancy.milestone.birth_prep"},
		38: {"pregnancy.milestone.birth_prep"},
		43: {},
	}
	for week, want := range cases {
		got := PregnancyMilestoneKeys(week)
		if len(got) != len(want) {
			t.Fatalf("week %d: got %v, want %v", week, got, want)
		}
		for index := range want {
			if got[index] != want[index] {
				t.Fatalf("week %d: got %v, want %v", week, got, want)
			}
		}
	}
}

func TestFormatPregnancyLength(t *testing.T) {
	cases := []struct {
		cm       float64
		language string
		unit     string
		value    string
	}{
		{0.1, "en", "pregnancy.length_mm", "1"},
		{0.6, "de", "pregnancy.length_mm", "6"},
		{5.4, "en", "pregnancy.length_cm", "5.4"},
		{5.4, "fr", "pregnancy.length_cm", "5,4"},
		{13, "ru", "pregnancy.length_cm", "13"},
		{51.7, "es", "pregnancy.length_cm", "51,7"},
	}
	for _, tc := range cases {
		got := FormatPregnancyLength(tc.cm, tc.language)
		if got.UnitKey != tc.unit || got.Value != tc.value {
			t.Fatalf("%.1f %s: got %+v, want %s %s", tc.cm, tc.language, got, tc.unit, tc.value)
		}
	}
}

func TestBuildPregnancyWeekView(t *testing.T) {
	if BuildPregnancyWeekView(PregnancyStatus{}, "en").Visible {
		t.Fatal("no pregnancy, no card")
	}

	due := ppDay(2026, time.October, 8)
	// Gestational day 90 = week 12, day 6.
	today := AddCalendarDays(PregnancyDueDateFromLastPeriod(ppDay(2026, time.January, 1)), -280+90, time.UTC)
	view := BuildPregnancyWeekView(ResolvePregnancyStatus(pregnancyUser(due), today), "en")
	if !view.Visible || view.Week != 12 || view.FruitKey != "pregnancy.fruit.lime" || view.DevKey != "pregnancy.week.12.dev" || view.Length.Value != "5.4" {
		t.Fatalf("unexpected week 12 card: %+v", view)
	}
	if len(view.MilestoneKeys) != 1 || view.MilestoneKeys[0] != "pregnancy.milestone.first_trimester_screen" {
		t.Fatalf("unexpected milestones: %v", view.MilestoneKeys)
	}

	early := AddCalendarDays(due, -280+20, time.UTC)
	if BuildPregnancyWeekView(ResolvePregnancyStatus(pregnancyUser(due), early), "en").Visible {
		t.Fatal("before week 4 there is no size to show")
	}
}
