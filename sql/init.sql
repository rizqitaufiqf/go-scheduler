-- ===================================================================
-- Go Scheduler - Complete Database Migration
-- Includes: Tasks, Entities, Actions, Notifications, Users
-- ===================================================================

-- ===================================================================
-- 0) Extensions & Functions
-- ===================================================================
CREATE EXTENSION IF NOT EXISTS "uuid-ossp"; -- For UUID generation
CREATE EXTENSION IF NOT EXISTS "pg_trgm";   -- For text search (optional, for notifications)

-- Auto-update 'updated_at' column
CREATE OR REPLACE FUNCTION public.update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE 'plpgsql';

-- ===================================================================
-- 1) task_entities
-- ===================================================================
CREATE TABLE IF NOT EXISTS public.task_entities (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(50) UNIQUE NOT NULL,      -- e.g. PRODUCT, ORDER, USER
    description TEXT,
    created_at TIMESTAMPTZ DEFAULT now(),
    created_by UUID, -- REFERENCES master.m_employees(id) ON DELETE SET NULL,
    updated_at TIMESTAMPTZ,
    updated_by UUID, -- REFERENCES master.m_employees(id) ON DELETE SET NULL,
    deleted_at TIMESTAMPTZ,
    deleted_by UUID -- REFERENCES master.m_employees(id) ON DELETE SET NULL
);

COMMENT ON TABLE public.task_entities IS 'Master table for task entity types (e.g., PRODUCT, ORDER)';
COMMENT ON COLUMN public.task_entities.name IS 'Unique entity name identifier';

-- ===================================================================
-- 2) task_actions
-- ===================================================================
CREATE TABLE IF NOT EXISTS public.task_actions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(50) UNIQUE NOT NULL,      -- e.g. CREATE, UPDATE, DELETE
    description TEXT,
    created_at TIMESTAMPTZ DEFAULT now(),
    created_by UUID, -- REFERENCES master.m_employees(id) ON DELETE SET NULL,
    updated_at TIMESTAMPTZ,
    updated_by UUID, -- REFERENCES master.m_employees(id) ON DELETE SET NULL,
    deleted_at TIMESTAMPTZ,
    deleted_by UUID -- REFERENCES master.m_employees(id) ON DELETE SET NULL
);

COMMENT ON TABLE public.task_actions IS 'Master table for task action types (e.g., CREATE, UPDATE, DELETE)';
COMMENT ON COLUMN public.task_actions.name IS 'Unique action name identifier';

-- ===================================================================
-- 3) users (untuk user_id di tasks dan notifications)
-- ===================================================================
-- Jika Anda sudah punya tabel users/employees, skip ini
-- Jika belum, buat simple users table
CREATE TABLE IF NOT EXISTS public.users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    username VARCHAR(100) UNIQUE NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    full_name VARCHAR(255),
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ
);

COMMENT ON TABLE public.users IS 'Users table for task creators and notification recipients';

-- ===================================================================
-- 4) task_schedulers
-- ===================================================================
CREATE TABLE IF NOT EXISTS public.task_schedulers (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    task_entity_id UUID NOT NULL REFERENCES public.task_entities(id) ON DELETE RESTRICT,
    task_action_id UUID NOT NULL REFERENCES public.task_actions(id) ON DELETE RESTRICT,
    payload JSONB NOT NULL,
    scheduled_at TIMESTAMPTZ NOT NULL,
    priority INT NOT NULL DEFAULT 0,
    retry_count INT NOT NULL DEFAULT 0,
    max_retries INT NOT NULL DEFAULT 3,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    result TEXT,
    error_message TEXT, -- untuk menyimpan error detail
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    last_error_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT now(),
    created_by UUID NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    updated_at TIMESTAMPTZ,
    updated_by UUID REFERENCES public.users(id) ON DELETE CASCADE,
    deleted_at TIMESTAMPTZ,
    deleted_by UUID REFERENCES public.users(id) ON DELETE CASCADE 
);

COMMENT ON TABLE public.task_schedulers IS 'Scheduled tasks with entity/action references';
COMMENT ON COLUMN public.task_schedulers.status IS 'Task status: pending, processing, completed, failed, canceled, paused, retrying';
COMMENT ON COLUMN public.task_schedulers.error_message IS 'Last error message if task failed';

-- ===================================================================
-- 5) notifications
-- ===================================================================
CREATE TABLE IF NOT EXISTS public.notifications (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    task_id UUID NOT NULL REFERENCES public.task_schedulers(id) ON DELETE CASCADE,
    type VARCHAR(50) NOT NULL,  -- e.g., 'task_completed', 'task_failed'
    title VARCHAR(255) NOT NULL,
    message TEXT NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending', -- 'pending', 'sent', 'read'
    payload JSONB,  -- Additional data (entity, action, error details, etc.)
    sent_at TIMESTAMPTZ,
    read_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ
);

COMMENT ON TABLE public.notifications IS 'Notifications for task completion/failure events';
COMMENT ON COLUMN public.notifications.type IS 'Notification type: task_completed, task_failed, etc.';
COMMENT ON COLUMN public.notifications.status IS 'Notification status: pending (not sent), sent (delivered), read (acknowledged)';
COMMENT ON COLUMN public.notifications.payload IS 'Additional metadata (entity name, action name, error info)';

-- ===================================================================
-- 6) products (application-specific table)
-- ===================================================================
CREATE TABLE IF NOT EXISTS public.products (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL,
    price NUMERIC(10, 2) NOT NULL,
    stock INT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE public.products IS 'Sample application table for product management';

-- ===================================================================
-- 7) Constraints
-- ===================================================================

-- task_schedulers status constraint
ALTER TABLE public.task_schedulers 
ADD CONSTRAINT chk_task_schedulers_status 
CHECK (status IN ('pending', 'processing', 'completed', 'failed', 'canceled', 'paused', 'retrying'));

-- notifications type constraint
ALTER TABLE public.notifications 
ADD CONSTRAINT chk_notifications_type 
CHECK (type IN ('task_completed', 'task_failed'));

-- notifications status constraint
ALTER TABLE public.notifications 
ADD CONSTRAINT chk_notifications_status 
CHECK (status IN ('pending', 'sent', 'read'));

-- priority constraint (0-10)
ALTER TABLE public.task_schedulers 
ADD CONSTRAINT chk_task_schedulers_priority 
CHECK (priority >= 0 AND priority <= 10);

-- retry_count should not exceed max_retries
ALTER TABLE public.task_schedulers 
ADD CONSTRAINT chk_task_schedulers_retry_count 
CHECK (retry_count <= max_retries);

-- ===================================================================
-- 8) Triggers
-- ===================================================================

-- Auto-update updated_at on task_entities
CREATE TRIGGER trg_set_updated_at_entities 
BEFORE UPDATE ON public.task_entities 
FOR EACH ROW EXECUTE FUNCTION public.update_updated_at_column();

-- Auto-update updated_at on task_actions
CREATE TRIGGER trg_set_updated_at_actions 
BEFORE UPDATE ON public.task_actions 
FOR EACH ROW EXECUTE FUNCTION public.update_updated_at_column();

-- Auto-update updated_at on task_schedulers
CREATE TRIGGER trg_set_updated_at_scheduled 
BEFORE UPDATE ON public.task_schedulers 
FOR EACH ROW EXECUTE FUNCTION public.update_updated_at_column();

-- Auto-update updated_at on users
CREATE TRIGGER trg_set_updated_at_users 
BEFORE UPDATE ON public.users 
FOR EACH ROW EXECUTE FUNCTION public.update_updated_at_column();

-- Auto-update updated_at on notifications
CREATE TRIGGER trg_set_updated_at_notifications 
BEFORE UPDATE ON public.notifications 
FOR EACH ROW EXECUTE FUNCTION public.update_updated_at_column();

-- Auto-update updated_at on products
CREATE TRIGGER trg_set_updated_at_products 
BEFORE UPDATE ON public.products 
FOR EACH ROW EXECUTE FUNCTION public.update_updated_at_column();

-- ===================================================================
-- 9) Indexes - task_entities
-- ===================================================================
CREATE INDEX IF NOT EXISTS idx_task_entities_name 
ON public.task_entities (name) 
WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_task_entities_deleted_at 
ON public.task_entities (deleted_at);

-- ===================================================================
-- 10) Indexes - task_actions
-- ===================================================================
CREATE INDEX IF NOT EXISTS idx_task_actions_name 
ON public.task_actions (name) 
WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_task_actions_deleted_at 
ON public.task_actions (deleted_at);

-- ===================================================================
-- 11) Indexes - users
-- ===================================================================
CREATE INDEX IF NOT EXISTS idx_users_username 
ON public.users (username) 
WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_users_email 
ON public.users (email) 
WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_users_is_active 
ON public.users (is_active) 
WHERE deleted_at IS NULL;

-- ===================================================================
-- 12) Indexes - task_schedulers (CRITICAL for performance)
-- ===================================================================

-- For worker polling (get pending tasks ordered by priority)
CREATE INDEX IF NOT EXISTS idx_task_schedulers_pending_priority 
ON public.task_schedulers (scheduled_at ASC, priority DESC) 
WHERE deleted_at IS NULL AND status = 'pending';

-- For retrying tasks
CREATE INDEX IF NOT EXISTS idx_task_schedulers_retrying 
ON public.task_schedulers (scheduled_at ASC) 
WHERE deleted_at IS NULL AND status = 'retrying';

-- For status queries
CREATE INDEX IF NOT EXISTS idx_task_schedulers_status 
ON public.task_schedulers (status) 
WHERE deleted_at IS NULL;

-- For entity/action lookup
CREATE INDEX IF NOT EXISTS idx_task_schedulers_entity_id 
ON public.task_schedulers (task_entity_id);

CREATE INDEX IF NOT EXISTS idx_task_schedulers_action_id 
ON public.task_schedulers (task_action_id);

-- Composite index for common queries
CREATE INDEX IF NOT EXISTS idx_task_schedulers_composite 
ON public.task_schedulers (task_entity_id, task_action_id, status) 
WHERE deleted_at IS NULL;

-- For soft delete queries
CREATE INDEX IF NOT EXISTS idx_task_schedulers_deleted_at 
ON public.task_schedulers (deleted_at);

-- For scheduled_at range queries
CREATE INDEX IF NOT EXISTS idx_task_schedulers_scheduled_at 
ON public.task_schedulers (scheduled_at);

-- ===================================================================
-- 13) Indexes - notifications
-- ===================================================================

-- Most important: Get pending notifications by user
CREATE INDEX IF NOT EXISTS idx_notifications_user_pending 
ON public.notifications (user_id, created_at DESC) 
WHERE status = 'pending' AND deleted_at IS NULL;

-- Get all notifications by user (for history)
CREATE INDEX IF NOT EXISTS idx_notifications_user_id 
ON public.notifications (user_id, created_at DESC) 
WHERE deleted_at IS NULL;

-- For status queries
CREATE INDEX IF NOT EXISTS idx_notifications_status 
ON public.notifications (status) 
WHERE deleted_at IS NULL;

-- For task-related notifications
CREATE INDEX IF NOT EXISTS idx_notifications_task_id 
ON public.notifications (task_id);

-- For notification type filtering
CREATE INDEX IF NOT EXISTS idx_notifications_type 
ON public.notifications (type) 
WHERE deleted_at IS NULL;

-- Composite index for common queries
CREATE INDEX IF NOT EXISTS idx_notifications_user_status_type 
ON public.notifications (user_id, status, type, created_at DESC) 
WHERE deleted_at IS NULL;

-- For cleanup queries (old read notifications)
CREATE INDEX IF NOT EXISTS idx_notifications_read_at 
ON public.notifications (read_at) 
WHERE status = 'read' AND deleted_at IS NULL;

-- For soft delete
CREATE INDEX IF NOT EXISTS idx_notifications_deleted_at 
ON public.notifications (deleted_at);

-- Optional: Full-text search on title/message (if needed)
-- CREATE INDEX IF NOT EXISTS idx_notifications_fts 
-- ON public.notifications USING gin(to_tsvector('english', title || ' ' || message));

-- ===================================================================
-- 14) Indexes - products
-- ===================================================================
CREATE INDEX IF NOT EXISTS idx_products_name 
ON public.products (name);

-- ===================================================================
-- 15) Seed Data
-- ===================================================================

-- Insert sample user (for testing)
INSERT INTO public.users (id, username, email, full_name, is_active)
VALUES 
    ('00000000-0000-0000-0000-000000000001', 'admin', 'admin@example.com', 'Admin User', true),
    ('00000000-0000-0000-0000-000000000002', 'testuser', 'test@example.com', 'Test User', true)
ON CONFLICT (username) DO NOTHING;

-- Insert master data for entities
INSERT INTO public.task_entities (name, description) 
VALUES 
    ('PRODUCT', 'Represents product-related tasks'),
    ('ORDER', 'Represents order-related tasks'),
    ('USER', 'Represents user-related tasks'),
    ('NOTIFICATION', 'Represents notification-related tasks')
ON CONFLICT (name) DO NOTHING;

-- Insert master data for actions
INSERT INTO public.task_actions (name, description) 
VALUES 
    ('CREATE', 'Task to create a new record'),
    ('UPDATE', 'Task to update an existing record'),
    ('DELETE', 'Task to delete a record'),
    ('SEND', 'Task to send something (email, notification, etc.)')
ON CONFLICT (name) DO NOTHING;

-- Insert sample pending task for testing
INSERT INTO public.task_schedulers (task_entity_id, task_action_id, created_by, payload, scheduled_at, priority)
SELECT
    (SELECT id FROM public.task_entities WHERE name = 'PRODUCT'),
    (SELECT id FROM public.task_actions WHERE name = 'CREATE'),
    '00000000-0000-0000-0000-000000000001'::UUID, -- admin user
    '{"name": "Sample Pending Product", "price": 99.99, "stock": 10}'::JSONB,
    NOW() + INTERVAL '2 minutes',
    10
WHERE
    EXISTS (SELECT 1 FROM public.task_entities WHERE name = 'PRODUCT') AND
    EXISTS (SELECT 1 FROM public.task_actions WHERE name = 'CREATE')
ON CONFLICT DO NOTHING;

-- Insert sample paused task for testing resume endpoint
INSERT INTO public.task_schedulers (task_entity_id, task_action_id, created_by, payload, scheduled_at, status, priority)
SELECT
    (SELECT id FROM public.task_entities WHERE name = 'PRODUCT'),
    (SELECT id FROM public.task_actions WHERE name = 'UPDATE'),
    '00000000-0000-0000-0000-000000000002'::UUID, -- test user
    jsonb_build_object(
        'id', uuid_generate_v4(), 
        'name', 'Paused Update Task', 
        'price', 150.00
    ),
    NOW() + INTERVAL '3 minutes',
    'paused',
    5
WHERE
    EXISTS (SELECT 1 FROM public.task_entities WHERE name = 'PRODUCT') AND
    EXISTS (SELECT 1 FROM public.task_actions WHERE name = 'UPDATE')
ON CONFLICT DO NOTHING;

-- ===================================================================
-- 16) Useful Views (Optional)
-- ===================================================================

-- View: Active tasks with entity/action names
CREATE OR REPLACE VIEW public.v_active_tasks AS
SELECT 
    ts.id,
    u.username,
    te.name AS entity_name,
    ta.name AS action_name,
    ts.payload,
    ts.scheduled_at,
    ts.priority,
    ts.status,
    ts.retry_count,
    ts.max_retries,
    ts.error_message,
    ts.created_at,
    ts.started_at,
    ts.finished_at
FROM public.task_schedulers ts
JOIN public.task_entities te ON ts.task_entity_id = te.id
JOIN public.task_actions ta ON ts.task_action_id = ta.id
JOIN public.users u ON ts.created_by = u.id
WHERE ts.deleted_at IS NULL;

COMMENT ON VIEW public.v_active_tasks IS 'Active tasks with joined entity/action/user names';

-- View: Pending notifications by user
CREATE OR REPLACE VIEW public.v_pending_notifications AS
SELECT 
    n.id,
    n.user_id,
    u.username,
    u.email,
    n.task_id,
    n.type,
    n.title,
    n.message,
    n.payload,
    n.created_at,
    EXTRACT(EPOCH FROM (NOW() - n.created_at)) / 60 AS minutes_pending
FROM public.notifications n
JOIN public.users u ON n.user_id = u.id
WHERE n.status = 'pending' 
  AND n.deleted_at IS NULL
ORDER BY n.created_at ASC;

COMMENT ON VIEW public.v_pending_notifications IS 'Pending notifications with user details';

-- ===================================================================
-- 17) Cleanup Functions (Optional - for maintenance)
-- ===================================================================

-- Function to cleanup old read notifications
CREATE OR REPLACE FUNCTION public.cleanup_old_notifications(days_old INT DEFAULT 30)
RETURNS TABLE(deleted_count BIGINT) AS $$
DECLARE
    count BIGINT;
BEGIN
    WITH deleted AS (
        UPDATE public.notifications
        SET deleted_at = NOW()
        WHERE status = 'read'
          AND read_at < NOW() - (days_old || ' days')::INTERVAL
          AND deleted_at IS NULL
        RETURNING id
    )
    SELECT COUNT(*) INTO count FROM deleted;
    
    RETURN QUERY SELECT count;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION public.cleanup_old_notifications IS 'Soft delete old read notifications (default 30 days)';

-- Function to cleanup old completed/failed tasks
-- CREATE OR REPLACE FUNCTION public.cleanup_old_tasks(days_old INT DEFAULT 90)
-- RETURNS TABLE(deleted_count BIGINT) AS $$
-- DECLARE
--     count BIGINT;
-- BEGIN
--     WITH deleted AS (
--         UPDATE public.task_schedulers
--         SET deleted_at = NOW()
--         WHERE status IN ('completed', 'failed', 'canceled')
--           AND finished_at < NOW() - (days_old || ' days')::INTERVAL
--           AND deleted_at IS NULL
--         RETURNING id
--     )
--     SELECT COUNT(*) INTO count FROM deleted;
    
--     RETURN QUERY SELECT count;
-- END;
-- $$ LANGUAGE plpgsql;

-- COMMENT ON FUNCTION public.cleanup_old_tasks IS 'Soft delete old completed/failed/canceled tasks (default 90 days)';

-- ===================================================================
-- 18) Grant Permissions (adjust as needed)
-- ===================================================================
-- GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO your_app_user;
-- GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO your_app_user;
-- GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA public TO your_app_user;

-- ===================================================================
-- Migration Complete!
-- ===================================================================
-- Tables created:
--   1. task_entities
--   2. task_actions
--   3. users
--   4. task_schedulers (with user_id)
--   5. notifications (NEW)
--   6. products
--
-- Features:
--   ✅ UUID primary keys
--   ✅ Soft deletes (deleted_at)
--   ✅ Audit fields (created_by, updated_by, deleted_by)
--   ✅ Auto-update triggers for updated_at
--   ✅ Comprehensive indexes for performance
--   ✅ Constraints for data integrity
--   ✅ Views for common queries
--   ✅ Cleanup functions for maintenance
--   ✅ Sample seed data
-- ===================================================================