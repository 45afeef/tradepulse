DROP INDEX IF EXISTS channel_deliveries_user_seq_idx;
DROP INDEX IF EXISTS audit_log_seq_idx;
ALTER TABLE channel_deliveries DROP COLUMN IF EXISTS seq;
ALTER TABLE audit_log          DROP COLUMN IF EXISTS seq;
