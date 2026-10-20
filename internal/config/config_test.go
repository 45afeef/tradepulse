package config

import (
	"fmt"
	"strings"
	"testing"
)

func baseEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"APP_ENV", "DOCS_ENABLED", "SIM_DEMO", "BOOTSTRAP_ADMIN_EMAIL", "BOOTSTRAP_ADMIN_PASSWORD", "DATABASE_URL", "DATABASE_MIGRATION_URL", "DB_MAX_CONNS", "DB_SIMPLE_PROTOCOL", "SOURCE_MIN_INTERVAL_SECONDS", "SOURCE_MAX_INTERVAL_SECONDS", "X_BEARER_TOKEN", "X_API_BASE_URL", "X_MONTHLY_BUDGET_USD", "WEBHOOK_SIGNING_KEY", "WEBHOOK_RATE_PER_MINUTE", "X_COST_PER_RESOURCE_USD", "X_MAX_POSTS_PER_FETCH", "FETCH_USER_AGENT", "ALERT_MAX_FIRES_PER_HOUR", "TRUST_PROXY", "JWT_TTL", "SIM_INTERVAL", "LOG_LEVEL"} {
		t.Setenv(k, "")
	}
	t.Setenv("JWT_SECRET", strings.Repeat("s", 40))
}

func TestSecretRequiredAndLong(t *testing.T) {
	baseEnv(t)
	t.Setenv("JWT_SECRET", "short")
	if _, err := Load(); err == nil {
		t.Fatal("short secret accepted")
	}
}

func TestProductionRejectsPlaceholderSecret(t *testing.T) {
	baseEnv(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("JWT_SECRET", "change-me-"+strings.Repeat("x", 30))
	if _, err := Load(); err == nil {
		t.Fatal("placeholder secret accepted in production")
	}
}

func TestProductionSafeDefaults(t *testing.T) {
	baseEnv(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATABASE_URL", "postgres://u:p@host:5432/db")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.DocsEnabled || c.SimDemo != "none" || c.TrustProxy {
		t.Fatalf("unsafe production defaults: %+v", c)
	}
}

func TestDevelopmentDefaults(t *testing.T) {
	baseEnv(t)
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !c.DocsEnabled || c.SimDemo != "market" {
		t.Fatalf("unexpected dev defaults: %+v", c)
	}
}

func TestInvalidSimDemoRejected(t *testing.T) {
	baseEnv(t)
	t.Setenv("SIM_DEMO", "bogus")
	if _, err := Load(); err == nil {
		t.Fatal("invalid SIM_DEMO accepted")
	}
}

func TestBootstrapAdminMustBeCompleteAndStrong(t *testing.T) {
	baseEnv(t)
	t.Setenv("BOOTSTRAP_ADMIN_EMAIL", "admin@example.com")
	t.Setenv("BOOTSTRAP_ADMIN_PASSWORD", "")
	if _, err := Load(); err == nil {
		t.Fatal("email without password accepted")
	}
	t.Setenv("BOOTSTRAP_ADMIN_PASSWORD", "short")
	if _, err := Load(); err == nil {
		t.Fatal("weak bootstrap password accepted")
	}
	t.Setenv("BOOTSTRAP_ADMIN_PASSWORD", "a-strong-password-1")
	if c, err := Load(); err != nil || c.BootstrapAdminEmail != "admin@example.com" {
		t.Fatalf("valid bootstrap rejected: %v", err)
	}
}

func TestProductionRequiresDatabase(t *testing.T) {
	baseEnv(t)
	t.Setenv("APP_ENV", "production")
	if _, err := Load(); err == nil {
		t.Fatal("production must refuse to start without DATABASE_URL (in-memory data loss)")
	}
}

func TestDatabaseConfig(t *testing.T) {
	baseEnv(t)
	t.Setenv("DATABASE_URL", "postgres://u:p@pooler:6543/db")
	c, err := Load()
	if err != nil || c.DatabaseMigrationURL != c.DatabaseURL || c.DBMaxConns != 10 || c.DBSimpleProtocol != "auto" {
		t.Fatalf("defaults wrong: %+v %v", c, err)
	}
	t.Setenv("DATABASE_MIGRATION_URL", "postgres://u:p@direct:5432/db")
	if c, _ = Load(); c.DatabaseMigrationURL != "postgres://u:p@direct:5432/db" {
		t.Fatal("migration URL override ignored")
	}
	for _, bad := range []struct{ k, v string }{{"DB_MAX_CONNS", "0"}, {"DB_MAX_CONNS", "abc"}, {"DB_MAX_CONNS", "1000"}, {"DB_SIMPLE_PROTOCOL", "maybe"}} {
		baseEnv(t)
		t.Setenv(bad.k, bad.v)
		if _, err := Load(); err == nil {
			t.Errorf("%s=%s accepted", bad.k, bad.v)
		}
	}
}

func TestSourceAndXConfigValidation(t *testing.T) {
	baseEnv(t)
	c, err := Load()
	if err != nil || c.SourceMinInterval.Seconds() != 60 || c.XMonthlyBudgetUSD != 20 || c.XAPIBaseURL == "" {
		t.Fatalf("defaults wrong: %+v %v", c, err)
	}
	for _, bad := range []struct{ k, v string }{
		{"SOURCE_MIN_INTERVAL_SECONDS", "1"}, {"SOURCE_MIN_INTERVAL_SECONDS", "x"},
		{"X_MONTHLY_BUDGET_USD", "-5"}, {"X_MONTHLY_BUDGET_USD", "lots"},
	} {
		baseEnv(t)
		t.Setenv(bad.k, bad.v)
		if _, err := Load(); err == nil {
			t.Errorf("%s=%s accepted", bad.k, bad.v)
		}
	}
	baseEnv(t)
	t.Setenv("SOURCE_MIN_INTERVAL_SECONDS", "600")
	t.Setenv("SOURCE_MAX_INTERVAL_SECONDS", "60")
	if _, err := Load(); err == nil {
		t.Fatal("max below min accepted")
	}
}

func TestConfigLogValueRedactsSecrets(t *testing.T) {
	baseEnv(t)
	t.Setenv("DATABASE_URL", "postgres://admin:DbPassw0rd@host:5432/db")
	t.Setenv("X_BEARER_TOKEN", "x-secret-token-123")
	t.Setenv("BOOTSTRAP_ADMIN_EMAIL", "root@example.com")
	t.Setenv("BOOTSTRAP_ADMIN_PASSWORD", "bootstrap-secret-pw")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	out := fmt.Sprintf("%v", c.LogValue())
	for _, secret := range []string{strings.Repeat("s", 40), "DbPassw0rd", "x-secret-token-123", "bootstrap-secret-pw"} {
		if strings.Contains(out, secret) {
			t.Fatalf("log value leaks a secret: %s", out)
		}
	}
}

func TestWebhookAndXSettings(t *testing.T) {
	baseEnv(t)
	for _, k := range []string{"WEBHOOK_SIGNING_KEY", "WEBHOOK_RATE_PER_MINUTE", "X_COST_PER_RESOURCE_USD", "X_MAX_POSTS_PER_FETCH", "FETCH_USER_AGENT"} {
		t.Setenv(k, "")
	}
	c, err := Load()
	if err != nil || c.WebhookSigningKey != "" || c.WebhookRatePerMinute != 60 || c.XCostPerResourceUSD != 0.005 || c.XMaxPostsPerFetch != 20 {
		t.Fatalf("defaults: %+v %v", c, err)
	}
	t.Setenv("WEBHOOK_SIGNING_KEY", strings.Repeat("w", 40))
	t.Setenv("X_COST_PER_RESOURCE_USD", "0.01")
	t.Setenv("X_MAX_POSTS_PER_FETCH", "50")
	if c, err = Load(); err != nil || c.WebhookSigningKey == "" || c.XCostPerResourceUSD != 0.01 || c.XMaxPostsPerFetch != 50 {
		t.Fatalf("overrides: %+v %v", c, err)
	}
	for _, bad := range []struct{ k, v string }{
		{"WEBHOOK_SIGNING_KEY", "too-short"}, {"WEBHOOK_RATE_PER_MINUTE", "0"}, {"WEBHOOK_RATE_PER_MINUTE", "99999"},
		{"X_COST_PER_RESOURCE_USD", "0"}, {"X_COST_PER_RESOURCE_USD", "free"}, {"X_MAX_POSTS_PER_FETCH", "4"}, {"X_MAX_POSTS_PER_FETCH", "500"},
		{"FETCH_USER_AGENT", strings.Repeat("a", 201)},
	} {
		baseEnv(t)
		t.Setenv(bad.k, bad.v)
		if _, err := Load(); err == nil {
			t.Errorf("%s=%q accepted", bad.k, bad.v)
		}
	}
}

func TestLogValueRedactsWebhookKey(t *testing.T) {
	baseEnv(t)
	t.Setenv("WEBHOOK_SIGNING_KEY", "webhook-key-"+strings.Repeat("k", 30))
	c, _ := Load()
	if out := fmt.Sprintf("%v", c.LogValue()); strings.Contains(out, "webhook-key-") {
		t.Fatalf("webhook signing key leaked: %s", out)
	}
}

func TestAlertSettings(t *testing.T) {
	baseEnv(t)
	if cfg, err := Load(); err != nil || cfg.AlertMaxFiresPerHour != 30 {
		t.Fatalf("default: %+v %v", cfg.AlertMaxFiresPerHour, err)
	}
	t.Setenv("ALERT_MAX_FIRES_PER_HOUR", "100")
	if cfg, err := Load(); err != nil || cfg.AlertMaxFiresPerHour != 100 {
		t.Fatalf("override: %v", err)
	}
	for _, bad := range []string{"0", "-1", "5000", "many"} {
		t.Setenv("ALERT_MAX_FIRES_PER_HOUR", bad)
		if _, err := Load(); err == nil {
			t.Errorf("ALERT_MAX_FIRES_PER_HOUR=%s accepted", bad)
		}
	}
}
