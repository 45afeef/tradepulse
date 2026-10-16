package user

import (
	"strings"
	"testing"

	"tradepulse/internal/domain/event"
	"tradepulse/internal/domain/rule"
)

func TestNormalizeProfile(t *testing.T) {
	reg := event.NewRegistry()
	p := Profile{
		Subscriptions: []Subscription{
			{Kind: " Entity ", Value: "  HDFC   Bank "},
			{Kind: "entity", Value: "hdfc bank"}, // duplicate after normalization
			{Kind: "keyword", Value: "Repo-Rate!!"},
		},
		Muted: []string{" Spam  Topic ", "spam topic"},
	}
	out, err := p.Normalize(reg)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Subscriptions) != 2 || out.Subscriptions[0].Value != "hdfc bank" || out.Subscriptions[1].Value != "repo rate" {
		t.Fatalf("unexpected subscriptions: %+v", out.Subscriptions)
	}
	if len(out.Muted) != 1 || out.Muted[0] != "spam topic" {
		t.Fatalf("unexpected muted: %+v", out.Muted)
	}
}

func TestNormalizeProfileRejects(t *testing.T) {
	reg := event.NewRegistry()
	tooMany := make([]Subscription, MaxSubscriptions+1)
	for i := range tooMany {
		tooMany[i] = Subscription{Kind: "entity", Value: "x"}
	}
	cases := map[string]Profile{
		"bad kind":          {Subscriptions: []Subscription{{Kind: "weird", Value: "x"}}},
		"empty value":       {Subscriptions: []Subscription{{Kind: "entity", Value: "   "}}},
		"value too long":    {Subscriptions: []Subscription{{Kind: "entity", Value: strings.Repeat("a", 101)}}},
		"control char":      {Subscriptions: []Subscription{{Kind: "entity", Value: "a\x00b"}}},
		"unknown type":      {Subscriptions: []Subscription{{Kind: "entity", Value: "x", Types: []string{"nope"}}}},
		"unknown field":     {Subscriptions: []Subscription{{Kind: "entity", Value: "x", Conditions: []rule.Condition{{Field: "nope", Op: rule.Eq, Value: "a"}}}}},
		"too many subs":     {Subscriptions: tooMany},
		"empty muted":       {Muted: []string{"  "}},
		"muted control":     {Muted: []string{"a\x07"}},
		"keyword no tokens": {Subscriptions: []Subscription{{Kind: "keyword", Value: "!!!"}}},
	}
	for name, p := range cases {
		if _, err := p.Normalize(reg); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}
