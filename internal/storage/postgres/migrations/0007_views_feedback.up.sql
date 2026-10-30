CREATE TABLE views (
    id         text PRIMARY KEY,
    user_id    text        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name       text        NOT NULL,
    filter     jsonb       NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);
CREATE UNIQUE INDEX views_user_name_idx ON views (user_id, lower(name));

CREATE TABLE feedback (
    user_id     text        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    target_type text        NOT NULL CHECK (target_type IN ('feed_item', 'alert_fire')),
    target_id   text        NOT NULL,
    rating      text        NOT NULL CHECK (rating IN ('useful', 'not_useful')),
    note        text        NOT NULL DEFAULT '',
    grp         text        NOT NULL DEFAULT '',
    label       text        NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL,
    updated_at  timestamptz NOT NULL,
    PRIMARY KEY (user_id, target_type, target_id)
);
CREATE INDEX feedback_updated_idx ON feedback (updated_at);

-- SECURITY: see 0001.
ALTER TABLE views    ENABLE ROW LEVEL SECURITY;
ALTER TABLE feedback ENABLE ROW LEVEL SECURITY;

DO $$
DECLARE r text;
BEGIN
  FOREACH r IN ARRAY ARRAY['anon', 'authenticated'] LOOP
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = r) THEN
      EXECUTE format('REVOKE ALL ON TABLE views, feedback FROM %I', r);
    END IF;
  END LOOP;
END $$;
