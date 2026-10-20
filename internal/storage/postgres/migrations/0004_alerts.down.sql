-- Destructive: drops alert rules, fires, channels and delivery history.
DROP TABLE IF EXISTS channel_deliveries;
DROP TABLE IF EXISTS notification_channels;
DROP TABLE IF EXISTS alert_fires;
DROP TABLE IF EXISTS alert_rules;
