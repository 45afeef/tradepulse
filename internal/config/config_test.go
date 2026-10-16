package config

import (
	"strings"
	"testing"
)

func baseEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"APP_ENV", "DOCS_ENABLED", "SIM_DEMO", "TRUST_PROXY", "JWT_TTL", "SIM_INTERVAL", "LOG_LEVEL"} {
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
