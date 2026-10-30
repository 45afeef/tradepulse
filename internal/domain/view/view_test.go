package view

import (
	"strings"
	"testing"
	"time"

	"tradepulse/internal/domain/event"
	"tradepulse/internal/domain/feed"
)

func reg(t *testing.T) *event.Registry {
	t.Helper()
	r := event.NewRegistry()
	if err := r.Register(event.TypeDef{ID: "metric", Fields: []event.FieldDef{{Name: "value", Kind: event.KindNumber}}, Title: "{entity} {value}"}); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestNormalizeCanonicalizesAndRejectsInvalidViews(t *testing.T) {
	r := reg(t)
	got, err := Spec{Name: "  Market   movers ", Filter: Filter{Sources: []string{" CNBC ", "cnbc"}, Entities: []string{"Markets"}, Keywords: []string{"Repo-Rate!"}, Types: []string{" METRIC "}, WithinHours: 24}}.Normalize(r)
	if err != nil || got.Name != "Market movers" || len(got.Filter.Sources) != 1 || got.Filter.Sources[0] != "cnbc" || got.Filter.Keywords[0] != "repo rate" || got.Filter.Types[0] != "metric" {
		t.Fatalf("normalize: %+v %v", got, err)
	}
	bad := map[string]Spec{
		"blank name":    {Name: " ", Filter: Filter{Sources: []string{"a"}}},
		"long name":     {Name: strings.Repeat("a", 61), Filter: Filter{Sources: []string{"a"}}},
		"control name":  {Name: "a\x00", Filter: Filter{Sources: []string{"a"}}},
		"empty filter":  {Name: "x"},
		"unknown type":  {Name: "x", Filter: Filter{Types: []string{"nope"}}},
		"too many":      {Name: "x", Filter: Filter{Sources: make([]string, 21)}},
		"blank value":   {Name: "x", Filter: Filter{Keywords: []string{"  "}}},
		"long value":    {Name: "x", Filter: Filter{Entities: []string{strings.Repeat("a", 101)}}},
		"control value": {Name: "x", Filter: Filter{Sources: []string{"a\x07"}}},
		"hours too big": {Name: "x", Filter: Filter{WithinHours: 721}},
		"negative hrs":  {Name: "x", Filter: Filter{Sources: []string{"a"}, WithinHours: -1}},
	}
	for name, sp := range bad {
		if _, err := sp.Normalize(r); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestFilterMatchSemantics(t *testing.T) {
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	it := feed.FeedItem{Source: "CNBC", EventType: "content_item", Entities: []string{"Markets", "Oil"}, Title: "Oil jumps on supply fears", Body: "Brent rose", CreatedAt: now.Add(-2 * time.Hour)}
	norm := func(f Filter) Filter {
		s, err := Spec{Name: "x", Filter: f}.Normalize(func() *event.Registry { r := event.NewRegistry(); return r }())
		if err != nil {
			t.Fatal(err)
		}
		return s.Filter
	}
	cases := []struct {
		name string
		f    Filter
		want bool
	}{
		{"source hit", Filter{Sources: []string{"cnbc"}}, true},
		{"source miss", Filter{Sources: []string{"reuters"}}, false},
		{"any entity", Filter{Entities: []string{"tech", "oil"}}, true},
		{"entity miss", Filter{Entities: []string{"tech"}}, false},
		{"keyword phrase", Filter{Keywords: []string{"supply fears"}}, true},
		{"keyword is a whole word", Filter{Keywords: []string{"oi"}}, false},
		{"keyword in body", Filter{Keywords: []string{"brent"}}, true},
		{"type hit", Filter{Types: []string{"content_item"}}, true},
		{"type miss", Filter{Types: []string{"content_item_x"}}, false},
		{"within window", Filter{Sources: []string{"cnbc"}, WithinHours: 3}, true},
		{"outside window", Filter{Sources: []string{"cnbc"}, WithinHours: 1}, false},
		{"all criteria must hold", Filter{Sources: []string{"cnbc"}, Entities: []string{"tech"}}, false},
	}
	for _, c := range cases {
		f := c.f
		if len(f.Types) > 0 && f.Types[0] == "content_item_x" { // unknown types are rejected by Normalize; match semantics only
			if got := f.Match(it, now); got != c.want {
				t.Errorf("%s: got %v", c.name, got)
			}
			continue
		}
		if got := norm(f).Match(it, now); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}
