-- Explicit pregnancy mode. See the SQLite migration of the same number for the
-- column semantics.
--
-- NOTE: keep prose in this file free of semicolons -- the migration runner
-- splits statements on the semicolon character without stripping SQL comments.

ALTER TABLE users ADD COLUMN IF NOT EXISTS pregnancy_due_date DATE;
ALTER TABLE users ADD COLUMN IF NOT EXISTS pregnancy_ended_on DATE;
