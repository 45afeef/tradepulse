package api

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"tradepulse/internal/domain/alert"
	"tradepulse/internal/domain/view"
)

// The in-app help (web/src/help/*.md) states limits and retention to members. These must stay true:
// the numbers are derived from the code, so changing a limit without updating the help fails the build.
func TestHelpPagesStateTheRealLimitsAndRetention(t *testing.T) {
	read := func(rel ...string) string {
		b, err := os.ReadFile(filepath.Join(append([]string{"..", ".."}, rel...)...))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	help := func(name string) string { return read("web", "src", "help", name) }
	s, _ := newServer(t, nil)

	env := regexp.MustCompile(`ALERT_MAX_FIRES_PER_HOUR=(\d+)`).FindStringSubmatch(read(".env.example"))
	if env == nil {
		t.Fatal("ALERT_MAX_FIRES_PER_HOUR missing from .env.example")
	}
	want := map[string][]string{
		"alerts.md": {
			fmt.Sprintf("up to %d alerts", alert.MaxRulesPerUser),
			fmt.Sprintf("The default is %d minutes", alert.DefaultCooldown/60),
			fmt.Sprintf("more than %s alerts in an hour", env[1]),
			"stay in your history for 30 days",
		},
		"delivery.md": {fmt.Sprintf("up to %d webhooks", alert.MaxChannelsPerUser), "After 10 failed deliveries in a row"},
		"feed.md":     {fmt.Sprintf("up to %d saved views", view.MaxPerUser)},
		"account.md": {
			fmt.Sprintf("After %d failed sign-in attempts", s.throttle.max),
			fmt.Sprintf("locked for about %d minutes", int(s.throttle.lock.Minutes())),
		},
		"data-use.md": {"most recent 1,000", "kept for 30 days", "kept for one year"},
	}
	for file, phrases := range want {
		doc := help(file)
		for _, p := range phrases {
			if !strings.Contains(doc, p) {
				t.Errorf("web/src/help/%s no longer says %q: update the help (or the code) so they agree", file, p)
			}
		}
	}

	// retention described in data-use.md matches the wiring in main.go
	main := read("cmd", "server", "main.go")
	for _, code := range []string{"RunRetention(ctx, pool, log, 1000, 365*24*time.Hour)", "alerting.RunRetention(ctx, aFires, dels, 30*24*time.Hour, log)", "fbk.Prune(ctx, now.AddDate(-1, 0, 0))"} {
		if !strings.Contains(main, code) {
			t.Errorf("main.go retention changed (%s): update web/src/help/data-use.md and this test", code)
		}
	}
	// the delivery page quotes the retry schedule and disable threshold used by the sender
	sender := read("internal", "alerting", "sender.go")
	if !strings.Contains(sender, "2 * time.Second, 10 * time.Second, 60 * time.Second") || !strings.Contains(sender, "cfg.DisableAfter = 10") {
		t.Error("sender retry/disable settings changed: update web/src/help/delivery.md")
	}
	if !strings.Contains(help("delivery.md"), "(2, 10 and 60 seconds)") {
		t.Error("delivery.md must state the retry schedule")
	}
}
