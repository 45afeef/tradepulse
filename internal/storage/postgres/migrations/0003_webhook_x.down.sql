-- Destructive: removes webhook and x sources and the usage ledger.
DROP TABLE IF EXISTS usage_monthly;
DELETE FROM sources WHERE type IN ('webhook', 'x');
ALTER TABLE sources DROP COLUMN IF EXISTS cursor;
ALTER TABLE sources DROP COLUMN IF EXISTS secret_version;
ALTER TABLE sources DROP COLUMN IF EXISTS xcfg;
ALTER TABLE sources DROP COLUMN IF EXISTS webhook;
ALTER TABLE sources DROP CONSTRAINT sources_type_check;
ALTER TABLE sources ADD CONSTRAINT sources_type_check CHECK (type IN ('rss', 'webmonitor', 'manual'));
