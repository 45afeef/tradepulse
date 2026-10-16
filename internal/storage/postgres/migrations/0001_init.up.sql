CREATE TABLE users (
    id            text PRIMARY KEY,
    email         text        NOT NULL,
    password_hash text        NOT NULL,
    role          text        NOT NULL CHECK (role IN ('admin', 'member')),
    status        text        NOT NULL CHECK (status IN ('active', 'suspended')),
    profile       jsonb       NOT NULL DEFAULT '{}'::jsonb,
    created_at    timestamptz NOT NULL
);
CREATE UNIQUE INDEX users_email_lower_key ON users (lower(email));

CREATE TABLE invites (
    id         text PRIMARY KEY,
    code_hash  text        NOT NULL UNIQUE,
    email      text        NOT NULL DEFAULT '',
    note       text        NOT NULL DEFAULT '',
    created_by text        NOT NULL,
    created_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    used_at    timestamptz,
    used_by    text        NOT NULL DEFAULT '',
    revoked_at timestamptz
);
CREATE INDEX invites_created_at_idx ON invites (created_at DESC);

CREATE TABLE audit_log (
    id          text PRIMARY KEY,
    at          timestamptz NOT NULL,
    actor_id    text        NOT NULL,
    action      text        NOT NULL,
    target_type text        NOT NULL DEFAULT '',
    target_id   text        NOT NULL DEFAULT '',
    detail      text        NOT NULL DEFAULT ''
);
CREATE INDEX audit_log_at_idx ON audit_log (at DESC);

CREATE TABLE feed_items (
    seq        bigserial   NOT NULL,
    id         text PRIMARY KEY,
    user_id    text        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    event_id   text        NOT NULL,
    event_type text        NOT NULL,
    source     text        NOT NULL,
    entities   jsonb       NOT NULL DEFAULT '[]'::jsonb,
    title      text        NOT NULL,
    body       text        NOT NULL,
    url        text        NOT NULL DEFAULT '',
    severity   text        NOT NULL,
    reason     text        NOT NULL,
    created_at timestamptz NOT NULL
);
CREATE INDEX feed_items_user_seq_idx ON feed_items (user_id, seq DESC);

-- SECURITY: managed Postgres providers (e.g. Supabase) expose the public schema through an
-- HTTP API with well-known roles. Deny all access except to the table owner (the app role,
-- which bypasses RLS): enable RLS with NO policies, and revoke grants from API roles if present.
ALTER TABLE users       ENABLE ROW LEVEL SECURITY;
ALTER TABLE invites     ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_log   ENABLE ROW LEVEL SECURITY;
ALTER TABLE feed_items  ENABLE ROW LEVEL SECURITY;

DO $$
DECLARE r text;
BEGIN
  FOREACH r IN ARRAY ARRAY['anon', 'authenticated'] LOOP
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = r) THEN
      EXECUTE format('REVOKE ALL ON TABLE users, invites, audit_log, feed_items FROM %I', r);
      EXECUTE format('REVOKE ALL ON SEQUENCE feed_items_seq_seq FROM %I', r);
    END IF;
  END LOOP;
END $$;
