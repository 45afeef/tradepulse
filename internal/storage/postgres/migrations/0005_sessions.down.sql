-- Destructive: drops all refresh tokens and the session-revocation marker.
DROP TABLE IF EXISTS refresh_tokens;
ALTER TABLE users DROP COLUMN IF EXISTS sessions_valid_after;
