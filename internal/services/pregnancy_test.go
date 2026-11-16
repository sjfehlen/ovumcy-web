package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ovumcy/ovumcy-web/internal/models"
)

func pregnancyUser(due time.Time) *models.User {
	user := regularWebhookUser()
	user.PregnancyDueDate = &due
	return user
}

func TestResolvePregnancyStatusInactiveWithoutADueDate(t *testing.T) {
	if status := ResolvePregnancyStatus(&models.User{}, ppDay(2026, time.March, 1)); status.Active {
		t.Fatalf("expected inactive, got %+v", status)
	}
	if status := ResolvePregnancyStatus(nil, ppDay(2026, time.March, 1)); status.Active {
		t.Fatalf("expected inactive for a nil user, got %+v", status)
	}
}

// TestResolvePregnancyStatusGestationalAgeAndTrimesterBoundaries pins the
// derivation against the due date: day 0 is the implied last period, week and
// day come from the day count, and trimesters close at 13w6d and 27w6d.
func TestResolvePregnancyStatusGestationalAgeAndTrimesterBoundaries(t *testing.T) {
	lmp := ppDay(2026, time.January, 1)
	due := PregnancyDueDateFromLastPeriod(lmp)
	if want := ppDay(2026, time.October, 8); !due.Equal(want) {
		t.Fatalf("due date = %s, want %s", due, want)
	}

	cases := []struct {
		name      string
		gestDays  int
		weeks     int
		days      int
		trimester int
	}{
		{"first day", 0, 0, 0, 1},
		{"end of trimester one", 97, 13, 6, 1},
		{"start of trimester two", 98, 14, 0, 2},
		{"end of trimester two", 195, 27, 6, 2},
		{"start of trimester three", 196, 28, 0, 3},
		{"due day", 280, 40, 0, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			today := AddCalendarDays(lmp, tc.gestDays, time.UTC)
			status := ResolvePregnancyStatus(pregnancyUser(due), today)
			if !status.Active || status.GestationalDays != tc.gestDays || status.Weeks != tc.weeks || status.DaysIntoWeek != tc.days || status.Trimester != tc.trimester {
				t.Fatalf("got %+v, want %d days = %dw%dd trimester %d", status, tc.gestDays, tc.weeks, tc.days, tc.trimester)
			}
			if status.DaysUntilDue != 280-tc.gestDays {
				t.Fatalf("days until due = %d, want %d", status.DaysUntilDue, 280-tc.gestDays)
			}
			if !status.LastPeriodStart.Equal(lmp) {
				t.Fatalf("implied last period = %s, want %s", status.LastPeriodStart, lmp)
			}
		})
	}
}

func TestResolvePregnancyStatusPastTheDueDateIsOverdueAndProgressClamps(t *testing.T) {
	due := ppDay(2026, time.October, 8)
	status := ResolvePregnancyStatus(pregnancyUser(due), ppDay(2026, time.October, 12))
	if !status.Overdue || status.DaysUntilDue != -4 || status.ProgressPercent != 100 || status.Trimester != 3 {
		t.Fatalf("expected an overdue third-trimester pregnancy at 100%%, got %+v", status)
	}
	if onDue := ResolvePregnancyStatus(pregnancyUser(due), due); onDue.Overdue || onDue.ProgressPercent != 100 {
		t.Fatalf("the due day itself is not overdue, got %+v", onDue)
	}
}

func TestResolvePregnancyStatusBeforeTheImpliedLastPeriodClampsToDayZero(t *testing.T) {
	due := ppDay(2026, time.October, 8)
	status := ResolvePregnancyStatus(pregnancyUser(due), AddCalendarDays(due, -300, time.UTC))
	if status.GestationalDays != 0 || status.Weeks != 0 || status.Trimester != 1 || status.ProgressPercent != 0 {
		t.Fatalf("expected the clamp at day zero, got %+v", status)
	}
}

func TestPregnancyDueDateFromInput(t *testing.T) {
	today := ppDay(2026, time.March, 10)

	t.Run("due date is used as given", func(t *testing.T) {
		due, err := PregnancyDueDateFromInput("2026-09-01", "", today)
		if err != nil || !due.Equal(ppDay(2026, time.September, 1)) {
			t.Fatalf("got %s, %v", due, err)
		}
	})
	t.Run("last period derives 280 days", func(t *testing.T) {
		due, err := PregnancyDueDateFromInput("", "2026-01-01", today)
		if err != nil || !due.Equal(ppDay(2026, time.October, 8)) {
			t.Fatalf("got %s, %v", due, err)
		}
	})
	t.Run("due date wins over last period", func(t *testing.T) {
		due, err := PregnancyDueDateFromInput("2026-09-01", "2026-01-01", today)
		if err != nil || !due.Equal(ppDay(2026, time.September, 1)) {
			t.Fatalf("got %s, %v", due, err)
		}
	})
	t.Run("nothing supplied", func(t *testing.T) {
		if _, err := PregnancyDueDateFromInput("", "", today); !errors.Is(err, ErrPregnancyDateRequired) {
			t.Fatalf("got %v", err)
		}
	})
	for name, args := range map[string][2]string{
		"unparseable due date":        {"soon", ""},
		"unparseable last period":     {"", "soon"},
		"due beyond a full term":      {"2026-12-16", ""},
		"due long past":               {"2026-02-23", ""},
		"last period over a term ago": {"", "2025-01-01"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := PregnancyDueDateFromInput(args[0], args[1], today); !errors.Is(err, ErrPregnancyDateInvalid) {
				t.Fatalf("got %v, want ErrPregnancyDateInvalid", err)
			}
		})
	}
	t.Run("boundaries are accepted", func(t *testing.T) {
		if _, err := PregnancyDueDateFromInput("2026-12-15", "", today); err != nil {
			t.Fatalf("a due date exactly one term out must be accepted, got %v", err)
		}
		if _, err := PregnancyDueDateFromInput("2026-02-24", "", today); err != nil {
			t.Fatalf("a due date exactly %d days past must be accepted, got %v", pregnancyMaxOverdueDays, err)
		}
	})
}

func TestResolvePregnancyPauseForUserDeclaredPregnancyPausesWithNoLogs(t *testing.T) {
	due := ppDay(2026, time.October, 8)
	date, paused := ResolvePregnancyPauseForUser(pregnancyUser(due), nil, BoundaryContext{})
	if !paused || !date.Equal(ppDay(2026, time.January, 1)) {
		t.Fatalf("expected a pause dated at the implied last period, got %s paused=%t", date, paused)
	}
}

// A declared pregnancy is not lifted by a later period: the owner ends it
// explicitly, unlike the inferred pause which a new cycle start resolves.
func TestResolvePregnancyPauseForUserDeclaredPregnancyIgnoresLaterCycleStart(t *testing.T) {
	logs := []models.DailyLog{{Date: ppDay(2026, time.April, 5), IsPeriod: true, CycleStart: true}}
	if _, paused := ResolvePregnancyPauseForUser(pregnancyUser(ppDay(2026, time.October, 8)), logs, BoundaryContext{}); !paused {
		t.Fatal("a declared pregnancy must stay paused until the owner ends it")
	}
}

func TestResolvePregnancyPauseForUserWithoutARecordMatchesTheInferredPause(t *testing.T) {
	logs := []models.DailyLog{
		{Date: ppDay(2026, time.March, 1), IsPeriod: true, CycleStart: true},
		{Date: ppDay(2026, time.March, 10), PregnancyTest: models.PregnancyTestPositive},
	}
	want, wantPaused := ResolvePregnancyPause(logs, BoundaryContext{})
	got, paused := ResolvePregnancyPauseForUser(&models.User{}, logs, BoundaryContext{})
	if paused != wantPaused || !got.Equal(want) {
		t.Fatalf("got %s/%t, want %s/%t", got, paused, want, wantPaused)
	}
	if _, nilPaused := ResolvePregnancyPauseForUser(nil, logs, BoundaryContext{}); !nilPaused {
		t.Fatal("a nil user falls back to the inferred pause")
	}
}

// TestResolvePregnancyPauseForUserEndingRetiresEarlierPositiveTests pins that
// ending a pregnancy lifts the pause its own positive test would otherwise keep
// in force, while a positive test AFTER the ending pauses again, and that the
// caller's slice is not modified.
func TestResolvePregnancyPauseForUserEndingRetiresEarlierPositiveTests(t *testing.T) {
	ended := ppDay(2026, time.June, 1)
	user := &models.User{PregnancyEndedOn: &ended}
	logs := []models.DailyLog{
		{Date: ppDay(2026, time.March, 1), IsPeriod: true, CycleStart: true},
		{Date: ppDay(2026, time.March, 10), PregnancyTest: models.PregnancyTestPositive},
	}

	if _, paused := ResolvePregnancyPauseForUser(user, logs, BoundaryContext{}); paused {
		t.Fatal("a positive test from before the ending must not keep predictions paused")
	}
	if logs[1].PregnancyTest != models.PregnancyTestPositive {
		t.Fatal("the caller's logs must not be modified")
	}

	onTheDay := append([]models.DailyLog{}, logs...)
	onTheDay = append(onTheDay, models.DailyLog{Date: ended, PregnancyTest: models.PregnancyTestPositive})
	if _, paused := ResolvePregnancyPauseForUser(user, onTheDay, BoundaryContext{}); paused {
		t.Fatal("a positive test on the ending day is retired with it")
	}

	later := append([]models.DailyLog{}, logs...)
	later = append(later, models.DailyLog{Date: ppDay(2026, time.June, 2), PregnancyTest: models.PregnancyTestPositive})
	if _, paused := ResolvePregnancyPauseForUser(user, later, BoundaryContext{}); !paused {
		t.Fatal("a positive test after the ending starts a new pause")
	}
}

// TestBuildCycleStatsFromLogsPausesForADeclaredPregnancy proves the declared
// pregnancy reaches the shared stats derivation, which every surface reads.
func TestBuildCycleStatsFromLogsPausesForADeclaredPregnancy(t *testing.T) {
	logs := webhookReminderCycleLogs(t, "2026-03-01")
	now := mustParseWebhookReminderDay(t, "2026-03-10", time.UTC)

	if BuildCycleStatsFromLogs(regularWebhookUser(), logs, now, time.UTC).PregnancyPaused {
		t.Fatal("fixture: an owner with no pregnancy must not be paused")
	}
	if !BuildCycleStatsFromLogs(pregnancyUser(ppDay(2026, time.October, 8)), logs, now, time.UTC).PregnancyPaused {
		t.Fatal("a declared pregnancy must pause the stats")
	}
}

// TestWebhookRemindersAreWithheldForADeclaredPregnancy is the egress check: the
// reminder pass reads the same derivation, so no period or ovulation reminder
// leaves the instance for an owner who declared a pregnancy.
func TestWebhookRemindersAreWithheldForADeclaredPregnancy(t *testing.T) {
	logs := webhookReminderCycleLogs(t, "2026-03-01")
	// Cycle day 26 of 28: the next period is two days out, inside a 3-day lead.
	now := mustParseWebhookReminderDay(t, "2026-03-26", time.UTC)

	reminders := DecideDueReminders(regularWebhookUser(), enabledWebhookSettings(3), logs, now, time.UTC)
	if _, ok := findDueReminder(reminders, DueReminderTypePeriod); !ok {
		t.Fatalf("fixture: expected a period reminder without a pregnancy, got %#v", reminders)
	}

	reminders = DecideDueReminders(pregnancyUser(ppDay(2026, time.October, 8)), enabledWebhookSettings(3), logs, now, time.UTC)
	if len(reminders) != 0 {
		t.Fatalf("a declared pregnancy must withhold every reminder, got %#v", reminders)
	}
}

// TestNotifyRecordCarriesThePregnancyRecord pins that the narrow notify
// projection reaches the pass's user: without it the pass would build its stats
// for an owner with no pregnancy and send a reminder the app has paused.
func TestNotifyRecordCarriesThePregnancyRecord(t *testing.T) {
	due := ppDay(2026, time.October, 8)
	ended := ppDay(2026, time.June, 1)
	user := userFromNotifyRecord(models.WebhookNotifyRecord{ID: 7, PregnancyDueDate: &due, PregnancyEndedOn: &ended})
	if user.PregnancyDueDate == nil || !user.PregnancyDueDate.Equal(due) || user.PregnancyEndedOn == nil || !user.PregnancyEndedOn.Equal(ended) {
		t.Fatalf("the notify user lost the pregnancy record: %+v", user)
	}
}

type fakePregnancyRepository struct {
	startedDue time.Time
	startedFor uint
	endedOn    time.Time
	endActive  bool
	err        error
}

func (repo *fakePregnancyRepository) StartPregnancy(_ context.Context, userID uint, dueDate time.Time) error {
	repo.startedFor, repo.startedDue = userID, dueDate
	return repo.err
}

func (repo *fakePregnancyRepository) EndPregnancy(_ context.Context, _ uint, endedOn time.Time) (bool, error) {
	repo.endedOn = endedOn
	return repo.endActive, repo.err
}

func TestPregnancyServiceStartStoresTheDerivedDueDate(t *testing.T) {
	repo := &fakePregnancyRepository{}
	today := ppDay(2026, time.March, 10)

	due, err := NewPregnancyService(repo).Start(context.Background(), 9, "", "2026-01-01", today)
	if err != nil || !due.Equal(ppDay(2026, time.October, 8)) || repo.startedFor != 9 || !repo.startedDue.Equal(due) {
		t.Fatalf("got %s, %v; repo %+v", due, err, repo)
	}
}

func TestPregnancyServiceStartRefusesBadInputWithoutWriting(t *testing.T) {
	repo := &fakePregnancyRepository{}
	if _, err := NewPregnancyService(repo).Start(context.Background(), 9, "", "", ppDay(2026, time.March, 10)); !errors.Is(err, ErrPregnancyDateRequired) {
		t.Fatalf("got %v", err)
	}
	if repo.startedFor != 0 {
		t.Fatal("a refused declaration must not reach the repository")
	}
}

func TestPregnancyServiceStartSurfacesAStorageFailure(t *testing.T) {
	repo := &fakePregnancyRepository{err: errors.New("disk")}
	if _, err := NewPregnancyService(repo).Start(context.Background(), 9, "2026-09-01", "", ppDay(2026, time.March, 10)); !errors.Is(err, repo.err) {
		t.Fatalf("got %v", err)
	}
}

func TestPregnancyServiceEnd(t *testing.T) {
	today := time.Date(2026, time.March, 10, 0, 0, 0, 0, time.UTC)

	active := &fakePregnancyRepository{endActive: true}
	if err := NewPregnancyService(active).End(context.Background(), 9, today); err != nil || !active.endedOn.Equal(today) {
		t.Fatalf("got %v, endedOn %s", err, active.endedOn)
	}
	if err := NewPregnancyService(&fakePregnancyRepository{}).End(context.Background(), 9, today); !errors.Is(err, ErrPregnancyNotActive) {
		t.Fatalf("an end with nothing active must report ErrPregnancyNotActive, got %v", err)
	}
	failing := &fakePregnancyRepository{err: errors.New("disk")}
	if err := NewPregnancyService(failing).End(context.Background(), 9, today); !errors.Is(err, failing.err) {
		t.Fatalf("got %v", err)
	}
}
