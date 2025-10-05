-- Rollback: revert the list of statuses to the original (without 'canceled').

BEGIN;

-- Ensure no rows still have the 'canceled' status before rolling back.
-- If they exist, change them first. Example:
-- UPDATE scheduled_tasks SET status = 'failed' WHERE status = 'canceled';

-- 1) Add the old constraint (without 'canceled') as NOT VALID.
ALTER TABLE scheduled_tasks
  ADD CONSTRAINT chk_scheduled_tasks_status_old
  CHECK (status IN ('pending','processing','completed','failed')) NOT VALID;

-- 2) Validate the old constraint.
ALTER TABLE scheduled_tasks
  VALIDATE CONSTRAINT chk_scheduled_tasks_status_old;

-- 3) Drop the currently active constraint.
ALTER TABLE scheduled_tasks
  DROP CONSTRAINT chk_scheduled_tasks_status;

-- 4) Rename the old constraint back to be consistent.
ALTER TABLE scheduled_tasks
  RENAME CONSTRAINT chk_scheduled_tasks_status_old
  TO chk_scheduled_tasks_status;

COMMIT;
