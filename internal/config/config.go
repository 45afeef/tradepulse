// Package config loads configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Env            string
	Addr           string
	JWTSecret      string
	JWTTTL         time.Duration
	AllowedOrigins []string
	TrustProxy     bool
	LogLevel       slog.Level
	RateRPS        float64
	RateBurst      int
	MaxWSPerUser   int
	DocsEnabled    bool

	// Database (managed Postgres: change provider by changing only these variables).
	DatabaseURL          string
	DatabaseMigrationURL string // direct / session-mode URL for migrations (defaults to DatabaseURL)
	DBMaxConns           int
	DBSimpleProtocol     string // auto | true | false

	// Sources (used from M3/M4). Intervals are bounds; each source sets its own.
	SourceMinInterval time.Duration
	SourceMaxInterval time.Duration
	FetchUserAgent    string // optional override for outbound requests
	XBearerToken      string // secret
	XAPIBaseURL       string
	XMonthlyBudgetUSD float64

	// Optional first-admin bootstrap (idempotent; remove from env after first start).
	BootstrapAdminEmail    string
	BootstrapAdminPassword string

	SimDemo     string // market | status | none
	SimSymbols  []string
	SimInterval time.Duration
}

func Load() (Config, error) {
	c := Config{
		Env:          get("APP_ENV", "development"),
		Addr:         get("ADDR", ":8080"),
		JWTSecret:    os.Getenv("JWT_SECRET"),
		MaxWSPerUser: 5,
	}
	var err error
	if len(c.JWTSecret) < 32 {
		return c, errors.New("JWT_SECRET is required and must be at least 32 characters")
	}
	if c.JWTTTL, err = time.ParseDuration(get("JWT_TTL", "12h")); err != nil {
		return c, fmt.Errorf("JWT_TTL: %w", err)
	}
	if c.SimInterval, err = time.ParseDuration(get("SIM_INTERVAL", "2s")); err != nil || c.SimInterval < 100*time.Millisecond {
		return c, errors.New("SIM_INTERVAL must be a duration >= 100ms")
	}
	if c.RateRPS, err = strconv.ParseFloat(get("RATE_LIMIT_RPS", "20"), 64); err != nil {
		return c, fmt.Errorf("RATE_LIMIT_RPS: %w", err)
	}
	if c.RateBurst, err = strconv.Atoi(get("RATE_LIMIT_BURST", "40")); err != nil {
		return c, fmt.Errorf("RATE_LIMIT_BURST: %w", err)
	}
	c.TrustProxy = get("TRUST_PROXY", "false") == "true"
	demoDefault := "market"
	if c.Production() {
		demoDefault = "none"
	}
	c.SimDemo = get("SIM_DEMO", demoDefault)
	if c.SimDemo != "market" && c.SimDemo != "status" && c.SimDemo != "none" {
		return c, errors.New("SIM_DEMO must be market, status or none")
	}
	// Docs (/docs, /openapi.json) default ON in dev and OFF in production.
	c.DocsEnabled = get("DOCS_ENABLED", strconv.FormatBool(!c.Production())) == "true"
	c.BootstrapAdminEmail = strings.TrimSpace(os.Getenv("BOOTSTRAP_ADMIN_EMAIL"))
	c.BootstrapAdminPassword = os.Getenv("BOOTSTRAP_ADMIN_PASSWORD")
	if (c.BootstrapAdminEmail == "") != (c.BootstrapAdminPassword == "") {
		return c, errors.New("BOOTSTRAP_ADMIN_EMAIL and BOOTSTRAP_ADMIN_PASSWORD must be set together")
	}
	if c.BootstrapAdminPassword != "" && len(c.BootstrapAdminPassword) < 12 {
		return c, errors.New("BOOTSTRAP_ADMIN_PASSWORD must be at least 12 characters")
	}
	if c.Production() && strings.HasPrefix(c.JWTSecret, "change-me") {
		return c, errors.New("JWT_SECRET is still the placeholder value")
	}
	c.AllowedOrigins = split(get("ALLOWED_ORIGINS", ""))
	c.SimSymbols = split(get("SIM_SYMBOLS", "RELIANCE,TCS,HDFCBANK,INFY,ICICIBANK,SBIN,ITC,LT,BHARTIARTL,TATAMOTORS"))
	if err := c.LogLevel.UnmarshalText([]byte(get("LOG_LEVEL", "info"))); err != nil {
		return c, fmt.Errorf("LOG_LEVEL: %w", err)
	}
	c.DatabaseURL = strings.TrimSpace(os.Getenv("DATABASE_URL"))
	c.DatabaseMigrationURL = strings.TrimSpace(os.Getenv("DATABASE_MIGRATION_URL"))
	if c.DatabaseMigrationURL == "" {
		c.DatabaseMigrationURL = c.DatabaseURL
	}
	if c.Production() && c.DatabaseURL == "" {
		return c, errors.New("DATABASE_URL is required in production")
	}
	if c.DBMaxConns, err = strconv.Atoi(get("DB_MAX_CONNS", "10")); err != nil || c.DBMaxConns < 1 || c.DBMaxConns > 100 {
		return c, errors.New("DB_MAX_CONNS must be an integer 1-100")
	}
	c.DBSimpleProtocol = strings.ToLower(get("DB_SIMPLE_PROTOCOL", "auto"))
	if c.DBSimpleProtocol != "auto" && c.DBSimpleProtocol != "true" && c.DBSimpleProtocol != "false" {
		return c, errors.New("DB_SIMPLE_PROTOCOL must be auto, true or false")
	}
	minS, e1 := strconv.Atoi(get("SOURCE_MIN_INTERVAL_SECONDS", "60"))
	maxS, e2 := strconv.Atoi(get("SOURCE_MAX_INTERVAL_SECONDS", "86400"))
	if e1 != nil || e2 != nil || minS < 5 || maxS < minS {
		return c, errors.New("SOURCE_MIN_INTERVAL_SECONDS must be >= 5 and SOURCE_MAX_INTERVAL_SECONDS >= the minimum")
	}
	c.SourceMinInterval = time.Duration(minS) * time.Second
	c.SourceMaxInterval = time.Duration(maxS) * time.Second
	c.FetchUserAgent = strings.TrimSpace(os.Getenv("FETCH_USER_AGENT"))
	if len(c.FetchUserAgent) > 200 {
		return c, errors.New("FETCH_USER_AGENT is too long")
	}
	c.XBearerToken = strings.TrimSpace(os.Getenv("X_BEARER_TOKEN"))
	c.XAPIBaseURL = get("X_API_BASE_URL", "https://api.x.com")
	if c.XMonthlyBudgetUSD, err = strconv.ParseFloat(get("X_MONTHLY_BUDGET_USD", "20"), 64); err != nil || c.XMonthlyBudgetUSD < 0 {
		return c, errors.New("X_MONTHLY_BUDGET_USD must be a number >= 0")
	}
	return c, nil
}

// LogValue makes Config safe to log: secrets and credentials are redacted.
func (c Config) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("env", c.Env), slog.String("addr", c.Addr), slog.Bool("docs", c.DocsEnabled),
		slog.String("demo", c.SimDemo), slog.Bool("database", c.DatabaseURL != ""),
		slog.Bool("x_configured", c.XBearerToken != ""), slog.Float64("x_budget_usd", c.XMonthlyBudgetUSD),
	)
}

func (c Config) Production() bool { return c.Env == "production" }

func get(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func split(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
