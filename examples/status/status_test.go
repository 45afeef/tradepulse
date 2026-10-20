package status

import (
	"testing"
	"time"

	"tradepulse/internal/domain/alert"
	"tradepulse/internal/domain/event"
	"tradepulse/internal/source"
)

func TestStatusProcessAndRender(t *testing.T) {
	reg := event.NewRegistry()
	if err := RegisterTypes(reg); err != nil {
		t.Fatal(err)
	}
	evs, err := NewProcessor().Process(source.RawMessage{ReceivedAt: time.Now(),
		Payload: []byte(`{"service":"checkout","status":"DOWN","latency_ms":950}`)})
	if err != nil || len(evs) != 1 {
		t.Fatalf("got %d events, err %v", len(evs), err)
	}
	if err := reg.Validate(evs[0]); err != nil {
		t.Fatal(err)
	}
	if title, _, _ := reg.Render(evs[0]); title != "checkout is down" {
		t.Fatalf("title %q", title)
	}
}

func TestStatusRejectsInvalid(t *testing.T) {
	for _, b := range []string{`x`, `{"service":"","status":"up"}`, `{"service":"a","status":"exploded"}`, `{"service":"a","status":"up","latency_ms":-1}`} {
		if _, err := NewProcessor().Process(source.RawMessage{Payload: []byte(b)}); err == nil {
			t.Errorf("expected error for %s", b)
		}
	}
}

func TestAlertTemplatesAreValid(t *testing.T) {
	reg := event.NewRegistry()
	if err := RegisterTypes(reg); err != nil {
		t.Fatal(err)
	}
	ts := alert.NewTemplates()
	if err := RegisterAlertTemplates(ts); err != nil {
		t.Fatal(err)
	}
	for _, x := range ts.List() {
		if _, err := x.Spec([]string{"checkout"}).Normalize(reg, nil); err != nil {
			t.Errorf("template %s does not normalize against the registry: %v", x.ID, err)
		}
	}
	if _, ok := ts.Get("service-not-up"); !ok {
		t.Fatal("service-not-up missing")
	}
}
