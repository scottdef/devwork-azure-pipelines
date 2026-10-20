-- migration 20261019_001: templates table
-- +migrate Up
CREATE TABLE templates (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    format TEXT NOT NULL DEFAULT 'txt',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX templates_name_idx ON templates (name);
CREATE INDEX templates_format_created_at_idx ON templates (format, created_at);

-- +migrate Down
DROP TABLE templates;
