-- Tambah nilai 'canceled' ke daftar status yang diizinkan dengan teknik NOT VALID + VALIDATE
-- agar aman di production (minim lock).

BEGIN;

-- 1) Tambahkan constraint baru (belum validasi data existing)
ALTER TABLE scheduled_tasks
  ADD CONSTRAINT chk_scheduled_tasks_status_new
  CHECK (status IN ('pending','processing','completed','failed','canceled')) NOT VALID;

-- 2) Validasi constraint baru terhadap data existing (non-blocking untuk write normal)
ALTER TABLE scheduled_tasks
  VALIDATE CONSTRAINT chk_scheduled_tasks_status_new;

-- 3) Hapus constraint lama
ALTER TABLE scheduled_tasks
  DROP CONSTRAINT chk_scheduled_tasks_status;

-- 4) Ganti nama constraint baru agar konsisten
ALTER TABLE scheduled_tasks
  RENAME CONSTRAINT chk_scheduled_tasks_status_new
  TO chk_scheduled_tasks_status;

COMMIT;
