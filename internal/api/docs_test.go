package api

import (
	"encoding/json"
	"strings"
	"testing"
)

// The API docs ARE the developer documentation, so completeness is enforced.
func TestEveryOperationIsDocumented(t *testing.T) {
	raw, err := OpenAPISpec()
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Info struct {
			Description string `json:"description"`
		} `json:"info"`
		Tags []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"tags"`
		Paths map[string]map[string]struct {
			Summary     string   `json:"summary"`
			Description string   `json:"description"`
			Tags        []string `json:"tags"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	declared := map[string]bool{}
	for _, tg := range spec.Tags {
		if strings.TrimSpace(tg.Description) == "" {
			t.Errorf("tag %q has no description", tg.Name)
		}
		declared[tg.Name] = true
	}
	ops := 0
	for path, methods := range spec.Paths {
		for method, op := range methods {
			ops++
			id := strings.ToUpper(method) + " " + path
			if strings.TrimSpace(op.Summary) == "" || len(strings.TrimSpace(op.Description)) < 15 {
				t.Errorf("%s: summary/description missing", id)
			}
			if len(op.Tags) == 0 {
				t.Errorf("%s: no tag", id)
			}
			for _, tg := range op.Tags {
				if !declared[tg] {
					t.Errorf("%s: tag %q not declared with a description", id, tg)
				}
			}
		}
	}
	if ops < 15 {
		t.Errorf("expected at least 15 operations, found %d", ops)
	}
	for _, must := range []string{"Quick start", "invite", "WebSocket", "Subscriptions", "Errors"} {
		if !strings.Contains(spec.Info.Description, must) {
			t.Errorf("overview is missing the %q section", must)
		}
	}
}

func TestAdminOperationsDeclareSecurity(t *testing.T) {
	raw, _ := OpenAPISpec()
	var spec struct {
		Paths map[string]map[string]struct {
			Security []map[string][]string `json:"security"`
		} `json:"paths"`
	}
	_ = json.Unmarshal(raw, &spec)
	for path, methods := range spec.Paths {
		if !strings.Contains(path, "/admin/") && !strings.Contains(path, "/me/") {
			continue
		}
		for method, op := range methods {
			if len(op.Security) == 0 {
				t.Errorf("%s %s has no security requirement in the spec", strings.ToUpper(method), path)
			}
		}
	}
}
