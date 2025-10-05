-- Tambah nilai 'canceled' ke daftar status yang diizinkan dengan teknik NOT VALID + VALIDATE
-- Add the 'canceled' value to the list of allowed statuses using the NOT VALID + VALIDATE technique
-- for a safe, low-lock migration in production.

BEGIN;

-- 1) Add the new constraint without validating existing data.
ALTER TABLE scheduled_tasks
  ADD CONSTRAINT chk_scheduled_tasks_status_new
  CHECK (status IN ('pending','processing','completed','failed','canceled')) NOT VALID;

-- 2) Validate the new constraint against existing data (this is non-blocking for normal writes).
ALTER TABLE scheduled_tasks
  VALIDATE CONSTRAINT chk_scheduled_tasks_status_new;

-- 3) Drop the old constraint.
ALTER TABLE scheduled_tasks
  DROP CONSTRAINT chk_scheduled_tasks_status;

-- 4) Rename the new constraint to be consistent.
ALTER TABLE scheduled_tasks
  RENAME CONSTRAINT chk_scheduled_tasks_status_new
  TO chk_scheduled_tasks_status;

COMMIT;
