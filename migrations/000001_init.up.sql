CREATE TABLE IF NOT EXISTS tasks (
    id UUID PRIMARY KEY,
    method TEXT NOT NULL,
    url TEXT NOT NULL,
    status TEXT NOT NULL,
    request_headers JSONB NOT NULL,
    http_status_code INTEGER NOT NULL DEFAULT 0,
    response_headers JSONB NOT NULL DEFAULT '{}'::jsonb,
    length BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ
);
