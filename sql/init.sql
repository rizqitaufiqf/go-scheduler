-- This extension is needed to generate UUIDs
-- ===================================================================
-- 0) Extensions & Functions
-- ===================================================================
CREATE EXTENSION IF NOT EXISTS "uuid-ossp"; -- More robust than pgcrypto for UUIDs

-- Auto-update 'updated_at' column
CREATE OR REPLACE FUNCTION public.update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- ======================================
-- 1) task_entities
-- ======================================
CREATE TABLE IF NOT EXISTS public.task_entities (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(50) UNIQUE NOT NULL,      -- e.g. PRODUCT, ORDER, USER
    description TEXT,
    created_at TIMESTAMPTZ DEFAULT now(),
    created_by UUID, -- REFERENCES master.m_employees(id) ON DELETE SET NULL,
    updated_at TIMESTAMPTZ DEFAULT now(),
    updated_by UUID, -- REFERENCES master.m_employees(id) ON DELETE SET NULL,
    deleted_at TIMESTAMPTZ,
    deleted_by UUID -- REFERENCES master.m_employees(id) ON DELETE SET NULL
);

-- ======================================
-- 2) task_actions
-- ======================================
CREATE TABLE IF NOT EXISTS public.task_actions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(50) UNIQUE NOT NULL,      -- e.g. CREATE, UPDATE, DELETE
    description TEXT,
    created_at TIMESTAMPTZ DEFAULT now(),
    created_by UUID, -- REFERENCES master.m_employees(id) ON DELETE SET NULL,
    updated_at TIMESTAMPTZ DEFAULT now(),
    updated_by UUID, -- REFERENCES master.m_employees(id) ON DELETE SET NULL,
    deleted_at TIMESTAMPTZ,
    deleted_by UUID -- REFERENCES master.m_employees(id) ON DELETE SET NULL
);

-- ======================================
-- 3) task_schedulers (renamed from scheduled_tasks)
-- ======================================
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
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    last_error_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT now(),
    created_by UUID, -- REFERENCES master.m_employees(id) ON DELETE SET NULL,
    updated_at TIMESTAMPTZ DEFAULT now(),
    updated_by UUID, -- REFERENCES master.m_employees(id) ON DELETE SET NULL,
    deleted_at TIMESTAMPTZ,
    deleted_by UUID -- REFERENCES master.m_employees(id) ON DELETE SET NULL
);

-- ======================================
-- 4) Constraints & Triggers
-- ======================================
ALTER TABLE public.task_schedulers ADD CONSTRAINT chk_task_schedulers_status CHECK (status IN ('pending', 'processing', 'completed', 'failed', 'canceled', 'paused', 'retrying'));

CREATE TRIGGER trg_set_updated_at_entities BEFORE UPDATE ON public.task_entities FOR EACH ROW EXECUTE FUNCTION public.update_updated_at_column();
CREATE TRIGGER trg_set_updated_at_actions BEFORE UPDATE ON public.task_actions FOR EACH ROW EXECUTE FUNCTION public.update_updated_at_column();
CREATE TRIGGER trg_set_updated_at_scheduled BEFORE UPDATE ON public.task_schedulers FOR EACH ROW EXECUTE FUNCTION public.update_updated_at_column();

-- ======================================
-- 5) Application-specific tables
-- ======================================
CREATE TABLE IF NOT EXISTS public.products (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL,
    price NUMERIC(10, 2) NOT NULL,
    stock INT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ===================================================================
-- 5) Indexes
-- ===================================================================
CREATE INDEX IF NOT EXISTS r_idx_task_entities_name ON public.task_entities (name) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS r_idx_task_actions_name ON public.task_actions (name) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS r_idx_sch_tasks_due_priority ON public.task_schedulers (scheduled_at ASC, priority DESC) WHERE deleted_at IS NULL AND status = 'pending';
CREATE INDEX IF NOT EXISTS r_idx_sch_tasks_entity_id ON public.task_schedulers (task_entity_id);
CREATE INDEX IF NOT EXISTS r_idx_sch_tasks_action_id ON public.task_schedulers (task_action_id);
CREATE INDEX IF NOT EXISTS r_idx_sch_tasks_entity_action_status ON public.task_schedulers (task_entity_id, task_action_id, status) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS r_idx_sch_tasks_status ON public.task_schedulers (status) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS r_idx_sch_tasks_scheduled_pending ON public.task_schedulers (scheduled_at) WHERE deleted_at IS NULL AND status = 'pending';
CREATE INDEX IF NOT EXISTS r_idx_sch_tasks_scheduled_retrying ON public.task_schedulers (scheduled_at) WHERE deleted_at IS NULL AND status = 'retrying';

-- ===================================================================
-- 6) Seed Data
-- ===================================================================
-- Insert master data for entities and actions
INSERT INTO public.task_entities (name, description) VALUES ('PRODUCT', 'Represents product-related tasks') ON CONFLICT (name) DO NOTHING;
INSERT INTO public.task_actions (name, description) VALUES ('CREATE', 'Task to create a new record') ON CONFLICT (name) DO NOTHING;
INSERT INTO public.task_actions (name, description) VALUES ('UPDATE', 'Task to update an existing record') ON CONFLICT (name) DO NOTHING;
INSERT INTO public.task_actions (name, description) VALUES ('DELETE', 'Task to delete a record') ON CONFLICT (name) DO NOTHING;

-- Add a sample task for testing
INSERT INTO public.task_schedulers (task_entity_id, task_action_id, payload, scheduled_at)
SELECT
    (SELECT id FROM public.task_entities WHERE name = 'PRODUCT'),
    (SELECT id FROM public.task_actions WHERE name = 'CREATE'),
    '{"name": "A Pending Product", "price": 99.99, "stock": 10}',
    NOW() + INTERVAL '2 minutes'
WHERE
    EXISTS (SELECT 1 FROM public.task_entities WHERE name = 'PRODUCT') AND
    EXISTS (SELECT 1 FROM public.task_actions WHERE name = 'CREATE');

-- Add a sample PAUSED task for testing the resume endpoint
INSERT INTO public.task_schedulers (task_entity_id, task_action_id, payload, scheduled_at, status)
SELECT
    (SELECT id FROM public.task_entities WHERE name = 'PRODUCT'),
    (SELECT id FROM public.task_actions WHERE name = 'UPDATE'),
    jsonb_build_object('id', uuid_generate_v4(), 'name', 'A Paused Update Task', 'price', 150.00),
    NOW() + INTERVAL '4 minutes',
    'paused'
WHERE
    EXISTS (SELECT 1 FROM public.task_entities WHERE name = 'PRODUCT') AND
    EXISTS (SELECT 1 FROM public.task_actions WHERE name = 'UPDATE');