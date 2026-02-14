CREATE TABLE IF NOT EXISTS tasks (
    id UUID PRIMARY KEY,
    method TEXT NOT NULL,
    url TEXT NOT NULL,
    status TEXT NOT NULL,
    request_headers JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ
);
