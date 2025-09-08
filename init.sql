-- This extension is needed to generate UUIDs
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE scheduled_tasks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_type VARCHAR(50) NOT NULL, -- e.g., 'PRODUCT_CREATE', 'PRODUCT_UPDATE', 'PRODUCT_DELETE'
    payload JSONB NOT NULL,
    scheduled_at TIMESTAMPTZ NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending', -- pending, processing, completed, failed
    result TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE products (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    price NUMERIC(10, 2) NOT NULL,
    stock INT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- This index is crucial for the worker to efficiently find pending tasks
CREATE INDEX idx_scheduled_tasks_pending ON scheduled_tasks (scheduled_at) WHERE status = 'pending';

-- Add a sample task to run 30 seconds after startup for testing
INSERT INTO scheduled_tasks (task_type, payload, scheduled_at) VALUES
('PRODUCT_CREATE', '{"name": "Pre-scheduled Product", "price": 99.99, "stock": 10}', NOW() + INTERVAL '2 minutes');