package connector

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	robotsToken    = "tradepulse"
	maxRobotsRules = 500
)

type robotRule struct {
	allow bool
	pat   string
}

type robotsEntry struct {
	rules   []robotRule
	expires time.Time
}

// Robots enforces robots.txt (fetched through the same SSRF-safe HTTP layer, cached).
type Robots struct {
	http HTTP
	ttl  time.Duration
	now  func() time.Time
	mu   sync.Mutex
	m    map[string]robotsEntry
}

func NewRobots(h HTTP, ttl time.Duration, now func() time.Time) *Robots {
	if now == nil {
		now = time.Now
	}
	return &Robots{http: h, ttl: ttl, now: now, m: map[string]robotsEntry{}}
}

// Allowed reports whether our crawler may fetch rawURL. 4xx on robots.txt means
// "no restrictions"; network errors and 5xx fail closed (an error is returned).
func (r *Robots) Allowed(ctx context.Context, rawURL string) (bool, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return false, fmt.Errorf("invalid URL")
	}
	origin := u.Scheme + "://" + u.Host
	r.mu.Lock()
	e, ok := r.m[origin]
	r.mu.Unlock()
	if !ok || r.now().After(e.expires) {
		resp, err := r.http.Get(ctx, origin+"/robots.txt", nil)
		if err != nil {
			return false, fmt.Errorf("robots.txt unavailable: %w", err)
		}
		switch {
		case resp.Status >= 500:
			return false, fmt.Errorf("robots.txt unavailable (HTTP %d)", resp.Status)
		case resp.Status >= 400:
			e = robotsEntry{expires: r.now().Add(r.ttl)} // no restrictions
		default:
			e = robotsEntry{rules: parseRobots(string(resp.Body), robotsToken), expires: r.now().Add(r.ttl)}
		}
		r.mu.Lock()
		r.m[origin] = e
		r.mu.Unlock()
	}
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	if u.RawQuery != "" {
		path += "?" + u.RawQuery
	}
	return robotsAllowed(e.rules, path), nil
}

// parseRobots returns the rules of the group that names our token, else of "*".
func parseRobots(body, token string) []robotRule {
	type group struct {
		agents []string
		rules  []robotRule
	}
	var groups []*group
	var cur *group
	lastWasRule := false
	for _, line := range strings.Split(body, "\n") {
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key, val := strings.ToLower(strings.TrimSpace(k)), strings.TrimSpace(v)
		switch key {
		case "user-agent":
			if cur == nil || lastWasRule {
				cur = &group{}
				groups = append(groups, cur)
			}
			cur.agents = append(cur.agents, strings.ToLower(val))
			lastWasRule = false
		case "allow", "disallow":
			if cur == nil {
				continue
			}
			lastWasRule = true
			if val == "" || len(val) > 200 || len(cur.rules) >= maxRobotsRules {
				continue // empty Disallow = allow all
			}
			cur.rules = append(cur.rules, robotRule{allow: key == "allow", pat: val})
		}
	}
	var specific, star []robotRule
	for _, g := range groups {
		for _, a := range g.agents {
			switch {
			case a == "*":
				star = append(star, g.rules...)
			case a != "" && strings.Contains(token, a):
				specific = append(specific, g.rules...)
			}
		}
	}
	if len(specific) > 0 {
		return specific
	}
	return star
}

func robotsAllowed(rules []robotRule, path string) bool {
	var best *robotRule
	for i := range rules {
		rl := &rules[i]
		if !robotMatch(rl.pat, path) {
			continue
		}
		if best == nil || len(rl.pat) > len(best.pat) || (len(rl.pat) == len(best.pat) && rl.allow) {
			best = rl
		}
	}
	return best == nil || best.allow
}

func robotMatch(pat, path string) bool {
	re := regexp.QuoteMeta(pat)
	re = strings.ReplaceAll(re, `\*`, ".*")
	if strings.HasSuffix(re, `\$`) {
		re = strings.TrimSuffix(re, `\$`) + "$"
	}
	ok, err := regexp.MatchString("^"+re, path)
	return err == nil && ok
}
