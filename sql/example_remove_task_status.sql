-- Rollback: kembalikan daftar status ke semula (tanpa 'canceled').

BEGIN;

-- Pastikan tidak ada baris yang masih berstatus 'canceled' sebelum rollback.
-- Jika ada, ubah dulu. Contoh:
-- UPDATE scheduled_tasks SET status = 'failed' WHERE status = 'canceled';

-- 1) Tambahkan constraint lama (tanpa 'canceled') sebagai NOT VALID
ALTER TABLE scheduled_tasks
  ADD CONSTRAINT chk_scheduled_tasks_status_old
  CHECK (status IN ('pending','processing','completed','failed')) NOT VALID;

-- 2) Validasi constraint lama
ALTER TABLE scheduled_tasks
  VALIDATE CONSTRAINT chk_scheduled_tasks_status_old;

-- 3) Hapus constraint aktif
ALTER TABLE scheduled_tasks
  DROP CONSTRAINT chk_scheduled_tasks_status;

-- 4) Rename kembali agar konsisten
ALTER TABLE scheduled_tasks
  RENAME CONSTRAINT chk_scheduled_tasks_status_old
  TO chk_scheduled_tasks_status;

COMMIT;
