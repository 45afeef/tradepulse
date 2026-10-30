package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// docs/DEVELOPER_GUIDE.md is the onboarding bible for developers and the dev advocate.
// It must never send anyone to an endpoint or file that does not exist.
func TestDeveloperGuideOnlyMentionsRealEndpointsAndFiles(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "docs", "DEVELOPER_GUIDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	doc := string(b)
	raw, err := OpenAPISpec()
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}

	endpoint := regexp.MustCompile("`(GET|POST|PUT|DELETE) (/api/v1/[A-Za-z0-9_/{}-]+)`")
	matches := endpoint.FindAllStringSubmatch(doc, -1)
	if len(matches) < 30 {
		t.Fatalf("the guide should reference the API through `METHOD /path` mentions (found %d)", len(matches))
	}
	for _, m := range matches {
		method, path := m[1], m[2]
		if path == "/api/v1/hooks/{name}" { // inbound webhook: real, but not part of the OpenAPI document by design
			continue
		}
		if _, ok := spec.Paths[path][strings.ToLower(method)]; !ok {
			t.Errorf("DEVELOPER_GUIDE.md mentions %s %s, which is not in the API", method, path)
		}
	}

	files := regexp.MustCompile(`((?:docs|deploy)/[A-Za-z0-9_./-]+\.(?:md|js|yaml))`)
	for _, m := range files.FindAllStringSubmatch(doc, -1) {
		if _, err := os.Stat(filepath.Join("..", "..", m[1])); err != nil {
			t.Errorf("DEVELOPER_GUIDE.md references %s, which does not exist", m[1])
		}
	}

	for _, must := range []string{
		"POST /api/v1/auth/login", "POST /api/v1/auth/refresh", "GET /api/v1/me/feed", "POST /api/v1/ws-ticket", "PUT /api/v1/me/subscriptions",
		"POST /api/v1/me/alerts/preview", "POST /api/v1/me/channels", "POST /api/v1/admin/sources/test", "POST /api/v1/admin/invites",
	} {
		if !strings.Contains(doc, "`"+must+"`") {
			t.Errorf("the guide must cover %s", must)
		}
	}
	for _, track := range []string{"Track A", "Track B", "Track C", "Track D", "Track E", "Track F", "Track G", "Track H", "Dev Advocate playbook"} {
		if !strings.Contains(doc, track) {
			t.Errorf("the guide lost its %q section", track)
		}
	}
}
