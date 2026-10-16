package rule

import (
	"testing"
	"time"

	"tradepulse/internal/domain/event"
)

func testRegistry(t *testing.T) *event.Registry {
	t.Helper()
	r := event.NewRegistry()
	err := r.Register(event.TypeDef{ID: "service_status", Fields: []event.FieldDef{
		{Name: "status", Kind: event.KindString}, {Name: "latency_ms", Kind: event.KindNumber}, {Name: "degraded", Kind: event.KindBool},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestNormalizeRejects(t *testing.T) {
	r := testRegistry(t)
	bad := []Condition{
		{Field: "nope", Op: Eq, Value: "x"},
		{Field: "latency_ms", Op: Contains, Value: "x"}, // op invalid for number
		{Field: "status", Op: Gt, Value: "x"},           // op invalid for string
		{Field: "latency_ms", Op: Gt, Value: "fast"},    // wrong value type
		{Field: "status", Op: In, Value: "down"},        // in needs array
		{Field: "status", Op: Eq, Value: ""},            // empty string
		{Field: "degraded", Op: Gt, Value: true},        // op invalid for bool
	}
	for _, c := range bad {
		if _, err := Normalize([]Condition{c}, r.KindOf); err == nil {
			t.Errorf("expected error for %+v", c)
		}
	}
	many := make([]Condition, MaxConditions+1)
	if _, err := Normalize(many, r.KindOf); err == nil {
		t.Error("too many conditions accepted")
	}
}

func TestMatch(t *testing.T) {
	r := testRegistry(t)
	ev := event.New("service_status", "STATUS_SIM", []string{"checkout"}, time.Now(),
		map[string]any{"status": "Down", "latency_ms": 950.0, "degraded": true})
	cases := []struct {
		c    Condition
		want bool
	}{
		{Condition{Field: "latency_ms", Op: Gte, Value: 900.0}, true},
		{Condition{Field: "latency_ms", Op: Lt, Value: 900.0}, false},
		{Condition{Field: "status", Op: Eq, Value: "down"}, true}, // case-insensitive
		{Condition{Field: "status", Op: Neq, Value: "up"}, true},
		{Condition{Field: "status", Op: In, Value: []any{"up", "down"}}, true},
		{Condition{Field: "status", Op: AnyOf, Value: []any{"dow"}}, true},
		{Condition{Field: "entity", Op: Eq, Value: "checkout"}, true},
		{Condition{Field: "degraded", Op: Eq, Value: true}, true},
		{Condition{Field: "latency_ms", Op: In, Value: []any{1.0, 950.0}}, true},
	}
	for _, tc := range cases {
		n, err := Normalize([]Condition{tc.c}, r.KindOf)
		if err != nil {
			t.Fatalf("%+v: %v", tc.c, err)
		}
		if got := Match(n, ev); got != tc.want {
			t.Errorf("%+v: got %v want %v", tc.c, got, tc.want)
		}
	}
	// missing field -> false, even for !=
	other := event.New("content_item", "S", nil, time.Now(), nil)
	n, _ := Normalize([]Condition{{Field: "status", Op: Neq, Value: "up"}}, r.KindOf)
	if Match(n, other) {
		t.Error("missing field must not match")
	}
}
