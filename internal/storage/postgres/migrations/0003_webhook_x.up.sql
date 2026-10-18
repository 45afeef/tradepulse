ALTER TABLE sources DROP CONSTRAINT sources_type_check;
ALTER TABLE sources ADD CONSTRAINT sources_type_check CHECK (type IN ('rss', 'webmonitor', 'manual', 'webhook', 'x'));
ALTER TABLE sources ADD COLUMN webhook        jsonb;
ALTER TABLE sources ADD COLUMN xcfg           jsonb;
ALTER TABLE sources ADD COLUMN secret_version integer NOT NULL DEFAULT 1;
ALTER TABLE sources ADD COLUMN cursor         text    NOT NULL DEFAULT '';

-- Monthly paid-API spend (micro-USD), used to enforce X_MONTHLY_BUDGET_USD across restarts and instances.
CREATE TABLE usage_monthly (
    period    text   NOT NULL,
    key       text   NOT NULL,
    micro_usd bigint NOT NULL DEFAULT 0,
    PRIMARY KEY (period, key)
);

-- SECURITY: see 0001.
ALTER TABLE usage_monthly ENABLE ROW LEVEL SECURITY;

DO $$
DECLARE r text;
BEGIN
  FOREACH r IN ARRAY ARRAY['anon', 'authenticated'] LOOP
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = r) THEN
      EXECUTE format('REVOKE ALL ON TABLE usage_monthly FROM %I', r);
    END IF;
  END LOOP;
END $$;
