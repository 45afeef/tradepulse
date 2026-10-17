package sourcedef

import (
	"strings"
	"testing"
	"time"
)

var bounds = Bounds{Min: 60 * time.Second, Max: 24 * time.Hour}

func TestNormalizeValidSources(t *testing.T) {
	rss, err := Source{Name: " CNBC-Top ", Type: TypeRSS, URL: " https://example.com/feed.xml ", IntervalSeconds: 300,
		Entities: []string{" Markets ", "markets", "CNBC"}}.Normalize(bounds)
	if err != nil || rss.Name != "cnbc-top" || rss.URL != "https://example.com/feed.xml" || len(rss.Entities) != 2 || rss.Entities[0] != "markets" {
		t.Fatalf("rss: %+v %v", rss, err)
	}
	web, err := Source{Name: "scanx", Type: TypeWebMonitor, URL: "https://example.com/news", IntervalSeconds: 600,
		Web: &WebConfig{ItemSelector: " div.item "}}.Normalize(bounds)
	if err != nil || web.Web.ItemSelector != "div.item" || web.Web.MaxItems != 30 {
		t.Fatalf("web: %+v %v", web, err)
	}
	man, err := Source{Name: "admin-posts", Type: TypeManual, URL: "https://ignored", IntervalSeconds: 99}.Normalize(bounds)
	if err != nil || man.URL != "" || man.IntervalSeconds != 0 {
		t.Fatalf("manual: %+v %v", man, err)
	}
}

func TestNormalizeRejects(t *testing.T) {
	ok := func() Source {
		return Source{Name: "good-name", Type: TypeRSS, URL: "https://example.com/f", IntervalSeconds: 300}
	}
	mut := map[string]func(*Source){
		"short name":        func(s *Source) { s.Name = "a" },
		"bad chars":         func(s *Source) { s.Name = "Bad Name!" },
		"unknown type":      func(s *Source) { s.Type = "ftp" },
		"ftp url":           func(s *Source) { s.URL = "ftp://example.com/f" },
		"no host":           func(s *Source) { s.URL = "https:///x" },
		"control chars url": func(s *Source) { s.URL = "https://example.com/\x00" },
		"interval too low":  func(s *Source) { s.IntervalSeconds = 10 },
		"interval too high": func(s *Source) { s.IntervalSeconds = 10 * 86400 },
		"too many entities": func(s *Source) { s.Entities = make([]string, 11) },
		"empty entity":      func(s *Source) { s.Entities = []string{"  "} },
		"web needs config":  func(s *Source) { s.Type = TypeWebMonitor },
		"web empty selector": func(s *Source) {
			s.Type = TypeWebMonitor
			s.Web = &WebConfig{ItemSelector: "  "}
		},
		"web max items": func(s *Source) {
			s.Type = TypeWebMonitor
			s.Web = &WebConfig{ItemSelector: "a", MaxItems: 500}
		},
		"robots override needs reason": func(s *Source) {
			s.Type = TypeWebMonitor
			s.Web = &WebConfig{ItemSelector: "a", IgnoreRobots: true, IgnoreRobotsReason: "short"}
		},
		"selector too long": func(s *Source) {
			s.Type = TypeWebMonitor
			s.Web = &WebConfig{ItemSelector: strings.Repeat("a", 201)}
		},
	}
	for name, m := range mut {
		s := ok()
		m(&s)
		if _, err := s.Normalize(bounds); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestRobotsOverrideKeepsReasonOnlyWhenIgnoring(t *testing.T) {
	s, err := Source{Name: "site", Type: TypeWebMonitor, URL: "https://example.com", IntervalSeconds: 300,
		Web: &WebConfig{ItemSelector: "a", IgnoreRobots: true, IgnoreRobotsReason: "owner has written permission"}}.Normalize(bounds)
	if err != nil || !s.Web.IgnoreRobots || s.Web.IgnoreRobotsReason == "" {
		t.Fatalf("override lost: %+v %v", s.Web, err)
	}
	s, _ = Source{Name: "site", Type: TypeWebMonitor, URL: "https://example.com", IntervalSeconds: 300,
		Web: &WebConfig{ItemSelector: "a", IgnoreRobotsReason: "stale reason text"}}.Normalize(bounds)
	if s.Web.IgnoreRobotsReason != "" {
		t.Fatal("reason must be cleared when robots are respected")
	}
}

func TestHealth(t *testing.T) {
	now := time.Now()
	ago := func(d time.Duration) *time.Time { t := now.Add(-d); return &t }
	in := func(d time.Duration) *time.Time { t := now.Add(d); return &t }
	base := Source{Type: TypeRSS, Enabled: true, IntervalSeconds: 300}
	cases := []struct {
		name string
		mod  func(*Source)
		want string
	}{
		{"disabled", func(s *Source) { s.Enabled = false }, "disabled"},
		{"manual", func(s *Source) { s.Type = TypeManual }, "ok"},
		{"pending", func(s *Source) {}, "pending"},
		{"ok", func(s *Source) { s.State.LastRunAt = ago(time.Minute); s.State.NextRunAt = in(time.Minute) }, "ok"},
		{"degraded", func(s *Source) { s.State.LastRunAt = ago(time.Minute); s.State.ConsecutiveFailures = 1; s.State.NextRunAt = in(time.Minute) }, "degraded"},
		{"failing", func(s *Source) { s.State.LastRunAt = ago(time.Minute); s.State.ConsecutiveFailures = 3 }, "failing"},
		{"overdue", func(s *Source) { s.State.LastRunAt = ago(5 * time.Hour); s.State.NextRunAt = ago(4 * time.Hour) }, "overdue"},
	}
	for _, c := range cases {
		s := base
		c.mod(&s)
		if got := s.Health(now); got != c.want {
			t.Errorf("%s: got %s want %s", c.name, got, c.want)
		}
	}
}
