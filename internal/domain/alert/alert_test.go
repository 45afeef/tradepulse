package alert

import (
	"strings"
	"testing"

	"tradepulse/internal/domain/event"
	"tradepulse/internal/domain/rule"
)

func testReg(t *testing.T) *event.Registry {
	t.Helper()
	r := event.NewRegistry()
	if err := r.Register(event.TypeDef{ID: "metric", Fields: []event.FieldDef{{Name: "value", Kind: event.KindNumber}}, Title: "{entity} {value}"}); err != nil {
		t.Fatal(err)
	}
	return r
}

func own(ids ...string) func(string) bool {
	return func(id string) bool {
		for _, x := range ids {
			if x == id {
				return true
			}
		}
		return false
	}
}

func TestNormalizeAppliesDefaultsAndCanonicalForms(t *testing.T) {
	sp, err := Spec{Name: "  Big   movers ", Scope: Scope{Kind: " Keyword ", Values: []string{"Repo-Rate!", "repo rate", "Oil"}},
		Types: []string{" METRIC ", "metric"}, Enabled: true}.Normalize(testReg(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	if sp.Name != "Big movers" || sp.Scope.Kind != "keyword" || len(sp.Scope.Values) != 2 || sp.Scope.Values[0] != "repo rate" || sp.Scope.Values[1] != "oil" {
		t.Fatalf("scope/name: %+v", sp)
	}
	if len(sp.Types) != 1 || sp.Types[0] != "metric" || sp.CooldownSeconds != DefaultCooldown || sp.Severity != "notable" || len(sp.Channels) != 1 || sp.Channels[0] != InApp {
		t.Fatalf("defaults: %+v", sp)
	}
}

func TestNormalizeRejectsInvalidSpecs(t *testing.T) {
	reg := testReg(t)
	ok := func() Spec { return Spec{Name: "r", Scope: Scope{Kind: "entity", Values: []string{"tcs"}}, Enabled: true} }
	mut := map[string]func(*Spec){
		"empty name":      func(s *Spec) { s.Name = "   " },
		"long name":       func(s *Spec) { s.Name = strings.Repeat("a", 81) },
		"control in name": func(s *Spec) { s.Name = "a\x00b" },
		"bad kind":        func(s *Spec) { s.Scope.Kind = "weird" },
		"no values":       func(s *Spec) { s.Scope.Values = nil },
		"too many values": func(s *Spec) { s.Scope.Values = make([]string, 21) },
		"empty value":     func(s *Spec) { s.Scope.Values = []string{"  "} },
		"long value":      func(s *Spec) { s.Scope.Values = []string{strings.Repeat("a", 101)} },
		"control value":   func(s *Spec) { s.Scope.Values = []string{"a\x07"} },
		"unknown type":    func(s *Spec) { s.Types = []string{"nope"} },
		"unknown field":   func(s *Spec) { s.Conditions = []rule.Condition{{Field: "nope", Op: rule.Eq, Value: "x"}} },
		"cooldown low":    func(s *Spec) { s.CooldownSeconds = 5 },
		"cooldown high":   func(s *Spec) { s.CooldownSeconds = MaxCooldown + 1 },
		"bad severity":    func(s *Spec) { s.Severity = "critical" },
		"unknown channel": func(s *Spec) { s.Channels = []string{"someone-elses-channel"} },
		"many channels":   func(s *Spec) { s.Channels = make([]string, 11) },
	}
	for name, m := range mut {
		s := ok()
		m(&s)
		if _, err := s.Normalize(reg, own("mine")); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	good := ok()
	good.Channels = []string{"mine", InApp, "mine"}
	if sp, err := good.Normalize(reg, own("mine")); err != nil || len(sp.Channels) != 2 {
		t.Fatalf("own channel must be accepted and de-duplicated: %+v %v", sp.Channels, err)
	}
}

func TestTemplates(t *testing.T) {
	ts := NewTemplates()
	if len(ts.List()) < 3 {
		t.Fatal("default templates missing")
	}
	x, ok := ts.Get("keyword-mention")
	if !ok {
		t.Fatal("keyword-mention missing")
	}
	sp := x.Spec([]string{"oil"})
	if sp.Scope.Kind != "keyword" || !sp.Enabled || sp.Scope.Values[0] != "oil" {
		t.Fatalf("spec from template: %+v", sp)
	}
	if _, err := sp.Normalize(event.NewRegistry(), nil); err != nil {
		t.Fatalf("every default template must normalize: %v", err)
	}
	for _, bad := range []Template{{}, {ID: "x", Name: "n", ScopeKind: "weird"}, {ID: "keyword-mention", Name: "dup", ScopeKind: "keyword"}} {
		if err := ts.Register(bad); err == nil {
			t.Errorf("invalid/duplicate template accepted: %+v", bad)
		}
	}
	list := ts.List()
	for i := 1; i < len(list); i++ {
		if list[i-1].ID > list[i].ID {
			t.Fatal("list must be sorted")
		}
	}
}
