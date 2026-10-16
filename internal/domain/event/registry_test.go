package event

import (
	"strings"
	"testing"
	"time"
)

func TestRegisterValidation(t *testing.T) {
	r := NewRegistry()
	bad := []TypeDef{
		{ID: "Bad-ID"},
		{ID: "t1", Fields: []FieldDef{{Name: "title", Kind: KindString}}},                       // built-in collision
		{ID: "t2", Fields: []FieldDef{{Name: "x", Kind: "weird"}}},                              // bad kind
		{ID: "t3", Title: "{missing}"},                                                          // unknown template field
		{ID: "t4", Severity: "critical"},                                                        // bad severity
		{ID: "t5", Fields: []FieldDef{{Name: "author", Kind: KindNumber}}},                      // kind conflict with content_item
		{ID: "content_item"},                                                                    // duplicate
	}
	for _, td := range bad {
		if err := r.Register(td); err == nil {
			t.Errorf("expected error for %+v", td)
		}
	}
}

func TestRenderAndValidate(t *testing.T) {
	r := NewRegistry()
	err := r.Register(TypeDef{
		ID: "service_status", Severity: "notable",
		Fields: []FieldDef{{Name: "status", Kind: KindString}, {Name: "latency_ms", Kind: KindNumber}},
		Title:  "{entity} is {status}", Body: "Latency {latency_ms:%.0f} ms",
	})
	if err != nil {
		t.Fatal(err)
	}
	e := New("service_status", "S", []string{"checkout"}, time.Now(), map[string]any{"status": "down", "latency_ms": 950.4})
	if err := r.Validate(e); err != nil {
		t.Fatal(err)
	}
	title, body, sev := r.Render(e)
	if title != "checkout is down" || body != "Latency 950 ms" || sev != "notable" {
		t.Fatalf("got %q / %q / %q", title, body, sev)
	}
	e.Payload["latency_ms"] = "slow"
	if err := r.Validate(e); err == nil || !strings.Contains(err.Error(), "latency_ms") {
		t.Fatalf("expected kind error, got %v", err)
	}
	if err := r.Validate(New("nope", "S", nil, time.Now(), nil)); err == nil {
		t.Fatal("unknown type accepted")
	}
}

func TestKindOf(t *testing.T) {
	r := NewRegistry()
	if k, ok := r.KindOf("title"); !ok || k != KindString {
		t.Fatal("builtin field missing")
	}
	if _, ok := r.KindOf("nonexistent"); ok {
		t.Fatal("unknown field accepted")
	}
}
