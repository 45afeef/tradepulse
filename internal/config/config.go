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
	if c.Production() && strings.HasPrefix(c.JWTSecret, "change-me") {
		return c, errors.New("JWT_SECRET is still the placeholder value")
	}
	c.AllowedOrigins = split(get("ALLOWED_ORIGINS", ""))
	c.SimSymbols = split(get("SIM_SYMBOLS", "RELIANCE,TCS,HDFCBANK,INFY,ICICIBANK,SBIN,ITC,LT,BHARTIARTL,TATAMOTORS"))
	if err := c.LogLevel.UnmarshalText([]byte(get("LOG_LEVEL", "info"))); err != nil {
		return c, fmt.Errorf("LOG_LEVEL: %w", err)
	}
	return c, nil
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
