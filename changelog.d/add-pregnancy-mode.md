### Added

- **A pregnancy mode.** An owner can declare a pregnancy under Settings → Pregnancy, from an
  estimated due date or from the first day of the last period (the due date is that day plus 280).
  While it is active the dashboard shows the gestational week and day, the trimester, the days left
  and a progress bar in place of the cycle countdown, and cycle predictions are paused on every
  surface — the dashboard, calendar, stats page, JSON API, `.ics` feed and webhook reminders — until
  the owner ends it. Ending the pregnancy resumes predictions and retires any positive pregnancy
  test dated on or before that day, which would otherwise have kept them paused until the next
  period. No outcome is recorded. `PUT` and `DELETE /api/v1/users/current/pregnancy` declare and end
  it, `GET /api/v1/stats/overview` gains a derived `pregnancy` object, and a declaration or an
  ending advances the webhook revocation epoch so an in-flight reminder pass cannot still deliver.
  Migration `043` adds two nullable date columns to `users`, both cleared by a data wipe.
