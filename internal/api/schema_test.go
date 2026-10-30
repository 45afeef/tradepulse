package api

import (
	"strings"
	"testing"
)

// Huma panics when two Go types share a schema name (both alert.Spec and view.Spec were once "Spec").
// This makes the failure explicit and keeps the chosen names stable for generated clients.
func TestOpenAPISchemaNamesDoNotCollide(t *testing.T) {
	var raw []byte
	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("schema generation panicked (two types share a schema name; add one to schemaNames): %v", r)
			}
		}()
		raw, err = OpenAPISpec()
	}()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{`"RuleSpec"`, `"ViewSpec"`} {
		if !strings.Contains(string(raw), name) {
			t.Errorf("expected schema %s in the OpenAPI document", name)
		}
	}
}
