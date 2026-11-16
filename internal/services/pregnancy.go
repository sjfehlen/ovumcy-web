package services

import (
	"context"
	"errors"
	"time"

	"github.com/ovumcy/ovumcy-web/internal/models"
)

const (
	// PregnancyTermDays is the length of a full-term pregnancy counted from the
	// first day of the last menstrual period (40 weeks), the convention every
	// due-date and gestational-age figure uses.
	PregnancyTermDays = 280
	// pregnancyFirstTrimesterEndDay and pregnancySecondTrimesterEndDay close the
	// first (13w6d) and second (27w6d) trimesters, counted in gestational days.
	pregnancyFirstTrimesterEndDay  = 13*7 + 6
	pregnancySecondTrimesterEndDay = 27*7 + 6
	// pregnancyMaxOverdueDays bounds how far past the due date a pregnancy may be
	// dated: a due date further back is an entry mistake, not a pregnancy.
	pregnancyMaxOverdueDays = 14

	SettingsPregnancyStartedStatus = "pregnancy_started"
	SettingsPregnancyEndedStatus   = "pregnancy_ended"
)

var (
	// ErrPregnancyDateRequired is returned when neither a due date nor a
	// last-period date is supplied.
	ErrPregnancyDateRequired = errors.New("pregnancy date required")
	// ErrPregnancyDateInvalid is returned for a date that cannot belong to a
	// current pregnancy: unparseable, or giving a gestational age outside the term
	// window.
	ErrPregnancyDateInvalid = errors.New("pregnancy date invalid")
	// ErrPregnancyNotActive is returned by End when no pregnancy is declared.
	ErrPregnancyNotActive = errors.New("pregnancy not active")
)

// PregnancyStatus is the derived view of an active pregnancy. It carries only
// values computed from the due date and today.
type PregnancyStatus struct {
	Active bool
	// DueDate is the estimated due date (calendar day).
	DueDate time.Time
	// LastPeriodStart is the implied first day of the last menstrual period.
	LastPeriodStart time.Time
	// GestationalDays counts from LastPeriodStart to today, 0 on the first day.
	GestationalDays int
	Weeks           int
	// DaysIntoWeek is the 0-6 remainder after Weeks.
	DaysIntoWeek int
	// Trimester is 1, 2 or 3.
	Trimester int
	// DaysUntilDue is negative once the due date has passed.
	DaysUntilDue int
	// Overdue reports that today is after the due date.
	Overdue bool
	// ProgressPercent is GestationalDays over the term, clamped to 0-100.
	ProgressPercent int
}

// PregnancyDueDateFromLastPeriod derives the estimated due date from the first
// day of the last menstrual period (Naegele's rule: 280 days).
func PregnancyDueDateFromLastPeriod(lastPeriodStart time.Time) time.Time {
	return AddCalendarDays(dateOnly(lastPeriodStart), PregnancyTermDays, time.UTC)
}

// ResolvePregnancyStatus derives the pregnancy view from the owner's stored due
// date and today. Inactive (zero value) when no pregnancy is declared.
//
// today is an owner-local calendar day; the stored due date is a UTC-midnight
// calendar day. Both sides are compared as calendar days.
func ResolvePregnancyStatus(user *models.User, today time.Time) PregnancyStatus {
	if user == nil || user.PregnancyDueDate == nil || user.PregnancyDueDate.IsZero() {
		return PregnancyStatus{}
	}
	due := dateOnly(*user.PregnancyDueDate)
	daysUntilDue := CalendarDaysBetween(today, due)
	gestationalDays := PregnancyTermDays - daysUntilDue
	if gestationalDays < 0 {
		gestationalDays = 0
	}
	trimester := 3
	switch {
	case gestationalDays <= pregnancyFirstTrimesterEndDay:
		trimester = 1
	case gestationalDays <= pregnancySecondTrimesterEndDay:
		trimester = 2
	}
	progress := gestationalDays * 100 / PregnancyTermDays
	if progress > 100 {
		progress = 100
	}
	return PregnancyStatus{
		Active:          true,
		DueDate:         due,
		LastPeriodStart: AddCalendarDays(due, -PregnancyTermDays, time.UTC),
		GestationalDays: gestationalDays,
		Weeks:           gestationalDays / 7,
		DaysIntoWeek:    gestationalDays % 7,
		Trimester:       trimester,
		DaysUntilDue:    daysUntilDue,
		Overdue:         daysUntilDue < 0,
		ProgressPercent: progress,
	}
}

// PregnancyDueDateFromInput resolves the owner's submitted dates to a due date.
// Exactly one anchor is used: an explicit due date wins over a last-period date.
// The result must put today inside the pregnancy window — not before the
// implied last period, and not more than pregnancyMaxOverdueDays past the due
// date — so a mistyped year is refused rather than saved.
func PregnancyDueDateFromInput(rawDueDate string, rawLastPeriod string, today time.Time) (time.Time, error) {
	var due time.Time
	switch {
	case rawDueDate != "":
		parsed, err := ParseDayDate(rawDueDate, time.UTC)
		if err != nil {
			return time.Time{}, ErrPregnancyDateInvalid
		}
		due = CalendarDay(parsed, time.UTC)
	case rawLastPeriod != "":
		parsed, err := ParseDayDate(rawLastPeriod, time.UTC)
		if err != nil {
			return time.Time{}, ErrPregnancyDateInvalid
		}
		due = PregnancyDueDateFromLastPeriod(parsed)
	default:
		return time.Time{}, ErrPregnancyDateRequired
	}

	daysUntilDue := CalendarDaysBetween(today, due)
	if daysUntilDue > PregnancyTermDays || daysUntilDue < -pregnancyMaxOverdueDays {
		return time.Time{}, ErrPregnancyDateInvalid
	}
	return due, nil
}

// PregnancyUserRepository is the persistence the pregnancy writes need.
type PregnancyUserRepository interface {
	StartPregnancy(ctx context.Context, userID uint, dueDate time.Time) error
	EndPregnancy(ctx context.Context, userID uint, endedOn time.Time) (bool, error)
}

// PregnancyService declares and ends an owner's pregnancy.
type PregnancyService struct {
	users PregnancyUserRepository
}

func NewPregnancyService(users PregnancyUserRepository) *PregnancyService {
	return &PregnancyService{users: users}
}

// Start declares (or re-dates) the owner's pregnancy. Returns the stored due date.
func (service *PregnancyService) Start(ctx context.Context, userID uint, rawDueDate string, rawLastPeriod string, today time.Time) (time.Time, error) {
	due, err := PregnancyDueDateFromInput(rawDueDate, rawLastPeriod, today)
	if err != nil {
		return time.Time{}, err
	}
	if err := service.users.StartPregnancy(ctx, userID, due); err != nil {
		return time.Time{}, err
	}
	return due, nil
}

// End closes the owner's active pregnancy as of today. ErrPregnancyNotActive
// when none was declared, so a double submit is visible rather than silent.
func (service *PregnancyService) End(ctx context.Context, userID uint, today time.Time) error {
	ended, err := service.users.EndPregnancy(ctx, userID, CalendarDay(today, time.UTC))
	if err != nil {
		return err
	}
	if !ended {
		return ErrPregnancyNotActive
	}
	return nil
}
