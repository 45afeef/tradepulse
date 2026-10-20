CREATE TABLE alert_rules (
    id               text PRIMARY KEY,
    user_id          text        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name             text        NOT NULL,
    scope            jsonb       NOT NULL,
    types            jsonb       NOT NULL DEFAULT '[]'::jsonb,
    conditions       jsonb       NOT NULL DEFAULT '[]'::jsonb,
    cooldown_seconds integer     NOT NULL,
    severity         text        NOT NULL,
    channels         jsonb       NOT NULL DEFAULT '["in_app"]'::jsonb,
    enabled          boolean     NOT NULL,
    paused_reason    text        NOT NULL DEFAULT '',
    created_at       timestamptz NOT NULL,
    updated_at       timestamptz NOT NULL,
    last_fired_at    timestamptz
);
CREATE INDEX alert_rules_user_idx ON alert_rules (user_id, created_at);

CREATE TABLE alert_fires (
    seq       bigserial   NOT NULL,
    id        text PRIMARY KEY,
    rule_id   text        NOT NULL,
    user_id   text        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    event_id  text        NOT NULL,
    rule_name text        NOT NULL,
    severity  text        NOT NULL,
    title     text        NOT NULL,
    body      text        NOT NULL,
    url       text        NOT NULL DEFAULT '',
    source    text        NOT NULL,
    entities  jsonb       NOT NULL DEFAULT '[]'::jsonb,
    reason    text        NOT NULL,
    fired_at  timestamptz NOT NULL
);
CREATE INDEX alert_fires_user_seq_idx ON alert_fires (user_id, seq DESC);
CREATE INDEX alert_fires_fired_idx ON alert_fires (fired_at);

CREATE TABLE notification_channels (
    id                   text PRIMARY KEY,
    user_id              text        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    type                 text        NOT NULL CHECK (type IN ('webhook')),
    name                 text        NOT NULL,
    url                  text        NOT NULL,
    enabled              boolean     NOT NULL DEFAULT true,
    secret_version       integer     NOT NULL DEFAULT 1,
    consecutive_failures integer     NOT NULL DEFAULT 0,
    disabled_reason      text        NOT NULL DEFAULT '',
    last_delivery_at     timestamptz,
    created_at           timestamptz NOT NULL
);
CREATE INDEX notification_channels_user_idx ON notification_channels (user_id, created_at);

CREATE TABLE channel_deliveries (
    id         text PRIMARY KEY,
    channel_id text        NOT NULL,
    user_id    text        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    fire_id    text        NOT NULL,
    at         timestamptz NOT NULL,
    ok         boolean     NOT NULL,
    status     integer     NOT NULL DEFAULT 0,
    attempts   integer     NOT NULL DEFAULT 0,
    error      text        NOT NULL DEFAULT ''
);
CREATE INDEX channel_deliveries_user_idx ON channel_deliveries (user_id, at DESC);
CREATE INDEX channel_deliveries_at_idx ON channel_deliveries (at);

-- SECURITY: see 0001.
ALTER TABLE alert_rules           ENABLE ROW LEVEL SECURITY;
ALTER TABLE alert_fires           ENABLE ROW LEVEL SECURITY;
ALTER TABLE notification_channels ENABLE ROW LEVEL SECURITY;
ALTER TABLE channel_deliveries    ENABLE ROW LEVEL SECURITY;

DO $$
DECLARE r text;
BEGIN
  FOREACH r IN ARRAY ARRAY['anon', 'authenticated'] LOOP
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = r) THEN
      EXECUTE format('REVOKE ALL ON TABLE alert_rules, alert_fires, notification_channels, channel_deliveries FROM %I', r);
      EXECUTE format('REVOKE ALL ON SEQUENCE alert_fires_seq_seq FROM %I', r);
    END IF;
  END LOOP;
END $$;
