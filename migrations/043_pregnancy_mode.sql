-- Explicit pregnancy mode. The owner declares a pregnancy by its estimated due
-- date and later ends it. Both columns are calendar dates, stored UTC-midnight
-- like last_period_start.
--
--   pregnancy_due_date  -- the estimated due date. NOT NULL means a pregnancy is
--                          active, and cycle predictions are paused for it. The
--                          gestational age is derived from it (280 days minus the
--                          days left), so there is one anchor and nothing to drift.
--   pregnancy_ended_on  -- the day the owner ended the most recent pregnancy.
--                          Positive pregnancy tests on or before this day no longer
--                          pause predictions. No outcome is recorded.
--
-- The migration runner skips any ADD COLUMN whose column already exists.
--
-- NOTE: keep prose in this file free of semicolons -- the migration runner
-- splits statements on the semicolon character without stripping SQL comments.

ALTER TABLE users ADD COLUMN pregnancy_due_date DATE;
ALTER TABLE users ADD COLUMN pregnancy_ended_on DATE;
