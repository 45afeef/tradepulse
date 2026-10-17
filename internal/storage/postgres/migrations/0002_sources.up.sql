CREATE TABLE sources (
    id                   text PRIMARY KEY,
    name                 text        NOT NULL UNIQUE,
    type                 text        NOT NULL CHECK (type IN ('rss', 'webmonitor', 'manual')),
    url                  text        NOT NULL DEFAULT '',
    enabled              boolean     NOT NULL DEFAULT false,
    interval_seconds     integer     NOT NULL DEFAULT 0,
    entities             jsonb       NOT NULL DEFAULT '[]'::jsonb,
    web                  jsonb,
    created_at           timestamptz NOT NULL,
    updated_at           timestamptz NOT NULL,
    last_run_at          timestamptz,
    last_success_at      timestamptz,
    next_run_at          timestamptz,
    last_item_at         timestamptz,
    last_error           text        NOT NULL DEFAULT '',
    consecutive_failures integer     NOT NULL DEFAULT 0,
    unchanged_streak     integer     NOT NULL DEFAULT 0,
    etag                 text        NOT NULL DEFAULT '',
    last_modified        text        NOT NULL DEFAULT '',
    items_total          bigint      NOT NULL DEFAULT 0
);

CREATE TABLE source_runs (
    id              text PRIMARY KEY,
    source_id       text        NOT NULL REFERENCES sources (id) ON DELETE CASCADE,
    started_at      timestamptz NOT NULL,
    duration_ms     integer     NOT NULL DEFAULT 0,
    ok              boolean     NOT NULL,
    http_status     integer     NOT NULL DEFAULT 0,
    items_seen      integer     NOT NULL DEFAULT 0,
    items_new       integer     NOT NULL DEFAULT 0,
    items_published integer     NOT NULL DEFAULT 0,
    error           text        NOT NULL DEFAULT ''
);
CREATE INDEX source_runs_source_started_idx ON source_runs (source_id, started_at DESC);
CREATE INDEX source_runs_started_idx ON source_runs (started_at);

-- Raw store AND dedupe: the primary key makes ingestion idempotent across restarts.
CREATE TABLE source_items (
    source_id  text        NOT NULL REFERENCES sources (id) ON DELETE CASCADE,
    dedupe_key text        NOT NULL,
    fetched_at timestamptz NOT NULL,
    raw        jsonb       NOT NULL,
    PRIMARY KEY (source_id, dedupe_key)
);
CREATE INDEX source_items_fetched_idx ON source_items (fetched_at);

-- SECURITY: see 0001. RLS with no policies + revoke from provider API roles.
ALTER TABLE sources      ENABLE ROW LEVEL SECURITY;
ALTER TABLE source_runs  ENABLE ROW LEVEL SECURITY;
ALTER TABLE source_items ENABLE ROW LEVEL SECURITY;

DO $$
DECLARE r text;
BEGIN
  FOREACH r IN ARRAY ARRAY['anon', 'authenticated'] LOOP
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = r) THEN
      EXECUTE format('REVOKE ALL ON TABLE sources, source_runs, source_items FROM %I', r);
    END IF;
  END LOOP;
END $$;
