-- Sequence numbers back the opaque pagination cursors (feed_items and alert_fires already have one).
ALTER TABLE audit_log          ADD COLUMN seq bigserial NOT NULL;
ALTER TABLE channel_deliveries ADD COLUMN seq bigserial NOT NULL;
CREATE INDEX audit_log_seq_idx ON audit_log (seq DESC);
CREATE INDEX channel_deliveries_user_seq_idx ON channel_deliveries (user_id, seq DESC);

-- SECURITY: see 0001 (the new sequences must not be reachable through provider APIs either).
DO $$
DECLARE r text;
BEGIN
  FOREACH r IN ARRAY ARRAY['anon', 'authenticated'] LOOP
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = r) THEN
      EXECUTE format('REVOKE ALL ON SEQUENCE audit_log_seq_seq, channel_deliveries_seq_seq FROM %I', r);
    END IF;
  END LOOP;
END $$;
