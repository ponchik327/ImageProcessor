CREATE TABLE IF NOT EXISTS images (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    original_path TEXT        NOT NULL,
    original_name TEXT        NOT NULL,
    mime_type     TEXT        NOT NULL,
    status        TEXT        NOT NULL DEFAULT 'pending',
    error_message TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
