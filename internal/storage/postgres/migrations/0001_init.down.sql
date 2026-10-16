-- Destructive: drops all application data. Used by tests and emergency rollback only.
DROP TABLE IF EXISTS feed_items;
DROP TABLE IF EXISTS audit_log;
DROP TABLE IF EXISTS invites;
DROP TABLE IF EXISTS users;
