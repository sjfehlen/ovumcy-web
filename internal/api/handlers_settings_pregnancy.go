package api

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/ovumcy/ovumcy-web/internal/services"
)

// Declaring or ending a pregnancy changes which cycle predictions are published
// and, through the webhook, which reminders leave the host, so both actions are
// health-data mutations (the same effect-based reading as the reminder window).
var (
	pregnancyStartMutation = healthMutationKind{action: "settings.pregnancy_start", target: "pregnancy"}
	pregnancyEndMutation   = healthMutationKind{action: "settings.pregnancy_end", target: "pregnancy"}
)

type pregnancyStartInput struct {
	DueDate         string `json:"due_date" form:"due_date"`
	LastPeriodStart string `json:"last_period_start" form:"last_period_start"`
}

func pregnancyInvalidDateErrorSpec() APIErrorSpec {
	return settingsValidationErrorSpec("invalid pregnancy date")
}

func pregnancyUpdateErrorSpec() APIErrorSpec {
	return globalErrorSpec(fiber.StatusInternalServerError, APIErrorCategoryInternal, "failed to update pregnancy")
}

func pregnancyNotActiveErrorSpec() APIErrorSpec {
	return globalErrorSpec(fiber.StatusConflict, APIErrorCategoryConflict, "no active pregnancy")
}

// StartPregnancy declares the authenticated owner's pregnancy (or re-dates the
// active one) from an estimated due date or the first day of the last period.
// Exactly one anchor is stored: the due date. An explicit due date wins when both
// are sent. The write is scoped to the session user_id, never an id from the
// request, and does not bump auth_session_version.
func (handler *Handler) StartPregnancy(c fiber.Ctx) error {
	user, ok := currentUser(c)
	if !ok {
		return handler.failMutation(c, pregnancyStartMutation, unauthorizedErrorSpec())
	}

	input, err := parsePregnancyStartInput(c)
	if err != nil {
		return handler.failMutation(c, pregnancyStartMutation, settingsInvalidInputErrorSpec())
	}

	location := handler.requestLocation(c)
	today := services.DateAtLocation(handler.clockNow(), location)
	due, err := handler.pregnancyService.Start(c.Context(), user.ID, input.DueDate, input.LastPeriodStart, today)
	if err != nil {
		if errors.Is(err, services.ErrPregnancyDateRequired) || errors.Is(err, services.ErrPregnancyDateInvalid) {
			return handler.failMutation(c, pregnancyStartMutation, pregnancyInvalidDateErrorSpec())
		}
		return handler.failMutation(c, pregnancyStartMutation, pregnancyUpdateErrorSpec())
	}
	user.PregnancyDueDate = &due
	user.PregnancyEndedOn = nil
	handler.logMutationSuccess(c, pregnancyStartMutation)

	status := services.SettingsPregnancyStartedStatus
	if acceptsJSON(c) {
		return c.JSON(fiber.Map{
			"ok":        true,
			"status":    status,
			"pregnancy": newPregnancyDTO(services.ResolvePregnancyStatus(user, today)),
		})
	}
	if isHTMX(c) {
		return sendHTMLFragment(c, htmxSettingsSuccessMarkup(c, status, "Pregnancy saved."))
	}

	handler.setFlashCookie(c, FlashPayload{SettingsSuccess: status})
	return redirectOrJSON(c, "/settings")
}

// EndPregnancy closes the owner's active pregnancy as of today. It answers 409
// when none is active, so a double submit is not mistaken for a first success.
func (handler *Handler) EndPregnancy(c fiber.Ctx) error {
	user, ok := currentUser(c)
	if !ok {
		return handler.failMutation(c, pregnancyEndMutation, unauthorizedErrorSpec())
	}

	location := handler.requestLocation(c)
	today := services.DateAtLocation(handler.clockNow(), location)
	if err := handler.pregnancyService.End(c.Context(), user.ID, today); err != nil {
		if errors.Is(err, services.ErrPregnancyNotActive) {
			return handler.failMutation(c, pregnancyEndMutation, pregnancyNotActiveErrorSpec())
		}
		return handler.failMutation(c, pregnancyEndMutation, pregnancyUpdateErrorSpec())
	}
	user.PregnancyDueDate = nil
	handler.logMutationSuccess(c, pregnancyEndMutation)

	status := services.SettingsPregnancyEndedStatus
	if acceptsJSON(c) {
		return c.JSON(fiber.Map{"ok": true, "status": status, "pregnancy": newPregnancyDTO(services.PregnancyStatus{})})
	}
	if isHTMX(c) {
		return sendHTMLFragment(c, htmxSettingsSuccessMarkup(c, status, "Pregnancy ended."))
	}

	handler.setFlashCookie(c, FlashPayload{SettingsSuccess: status})
	return redirectOrJSON(c, "/settings")
}

func parsePregnancyStartInput(c fiber.Ctx) (pregnancyStartInput, error) {
	input := pregnancyStartInput{}
	if hasJSONBody(c) {
		if err := c.Bind().Body(&input); err != nil {
			return pregnancyStartInput{}, err
		}
	} else {
		input.DueDate = c.FormValue("due_date")
		input.LastPeriodStart = c.FormValue("last_period_start")
	}
	input.DueDate = strings.TrimSpace(input.DueDate)
	input.LastPeriodStart = strings.TrimSpace(input.LastPeriodStart)
	return input, nil
}

// pregnancyDTO is the transport shape of a derived pregnancy status. Dates are
// calendar days (format: date). It is the one place the status is serialized, so
// the stats overview and the pregnancy write responses cannot disagree.
//
// Every field but Active is a pointer so that an ACTIVE pregnancy always carries
// all of them — zero is a real answer (the first week, the due day) and must not
// be dropped by omitempty — while an inactive one carries Active alone.
type pregnancyDTO struct {
	Active          bool    `json:"active"`
	DueDate         *string `json:"due_date,omitempty"`
	GestationalDays *int    `json:"gestational_days,omitempty"`
	Weeks           *int    `json:"weeks,omitempty"`
	DaysIntoWeek    *int    `json:"days_into_week,omitempty"`
	Trimester       *int    `json:"trimester,omitempty"`
	DaysUntilDue    *int    `json:"days_until_due,omitempty"`
	Overdue         *bool   `json:"overdue,omitempty"`
	ProgressPercent *int    `json:"progress_percent,omitempty"`
}

func newPregnancyDTO(status services.PregnancyStatus) pregnancyDTO {
	if !status.Active {
		return pregnancyDTO{}
	}
	dueDate := status.DueDate.Format("2006-01-02")
	return pregnancyDTO{
		Active:          true,
		DueDate:         &dueDate,
		GestationalDays: &status.GestationalDays,
		Weeks:           &status.Weeks,
		DaysIntoWeek:    &status.DaysIntoWeek,
		Trimester:       &status.Trimester,
		DaysUntilDue:    &status.DaysUntilDue,
		Overdue:         &status.Overdue,
		ProgressPercent: &status.ProgressPercent,
	}
}
