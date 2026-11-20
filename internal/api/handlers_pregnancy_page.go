package api

import (
	"github.com/gofiber/fiber/v3"
	"github.com/ovumcy/ovumcy-web/internal/services"
)

// ShowPregnancy renders the pregnancy tab: the status, a week-by-week timeline
// and the owner's journal figures for it. Without a declared pregnancy there is
// nothing to show, so the tab's address sends the owner back to the dashboard.
func (handler *Handler) ShowPregnancy(c fiber.Ctx) error {
	user, handled, err := handler.currentUserOrRedirectToLogin(c)
	if err != nil {
		return err
	}
	if handled {
		return nil
	}

	language, messages, now := handler.currentPageViewContext(c)
	location := handler.requestLocation(c)
	today := services.DateAtLocation(now, location)
	status := services.ResolvePregnancyStatus(user, today)
	if !status.Active {
		return c.Redirect().Status(fiber.StatusSeeOther).To("/dashboard")
	}

	logs, err := handler.dayService.FetchLogsForUser(c.Context(), user.ID, status.LastPeriodStart, today, location)
	if err != nil {
		return handler.respondMappedError(c, mapStatsPageViewError(err))
	}
	page := services.BuildPregnancyPageView(status, logs, language)

	return handler.render(c, "pregnancy", fiber.Map{
		"Title":         localizedPageTitle(messages, "meta.title.pregnancy", "Ovumcy | Pregnancy"),
		"CurrentUser":   user,
		"Pregnancy":     status,
		"PregnancyPage": page,
	})
}
