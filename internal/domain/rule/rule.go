// Package rule is a small, safe condition engine: `field op value` over event
// fields. Fields are whitelisted by the event type registry; there is no
// expression language and nothing user-supplied is ever executed.
package rule

import (
	"errors"
	"fmt"
	"strings"

	"tradepulse/internal/domain/event"
)

const (
	MaxConditions = 10
	MaxListLen    = 50
	MaxStringLen  = 200
)

type Op string

const (
	Gt       Op = ">"
	Gte      Op = ">="
	Lt       Op = "<"
	Lte      Op = "<="
	Eq       Op = "=="
	Neq      Op = "!="
	In       Op = "in"
	Contains Op = "contains"
	AnyOf    Op = "any_of"
)

type Condition struct {
	Field string `json:"field" maxLength:"40" doc:"Built-in (type, source, title, summary, url, entity) or a payload field; see GET /event-types"`
	Op    Op     `json:"op" enum:">,>=,<,<=,==,!=,in,contains,any_of"`
	Value any    `json:"value" doc:"number, string or boolean; an array for in / any_of"`
}

// KindLookup resolves a field's kind (the field whitelist). Use Registry.KindOf.
type KindLookup func(field string) (event.FieldKind, bool)

var opsByKind = map[event.FieldKind]map[Op]bool{
	event.KindNumber: {Gt: true, Gte: true, Lt: true, Lte: true, Eq: true, Neq: true, In: true},
	event.KindString: {Eq: true, Neq: true, In: true, Contains: true, AnyOf: true},
	event.KindBool:   {Eq: true, Neq: true},
}

// Normalize validates conditions against the field whitelist and canonicalizes values.
func Normalize(conds []Condition, kindOf KindLookup) ([]Condition, error) {
	if len(conds) > MaxConditions {
		return nil, fmt.Errorf("at most %d conditions", MaxConditions)
	}
	out := make([]Condition, 0, len(conds))
	for i, c := range conds {
		n, err := normalizeOne(c, kindOf)
		if err != nil {
			return nil, fmt.Errorf("conditions[%d]: %w", i, err)
		}
		out = append(out, n)
	}
	return out, nil
}

func normalizeOne(c Condition, kindOf KindLookup) (Condition, error) {
	c.Field = strings.ToLower(strings.TrimSpace(c.Field))
	kind, ok := kindOf(c.Field)
	if !ok {
		return c, fmt.Errorf("unknown field %q", c.Field)
	}
	if !opsByKind[kind][c.Op] {
		return c, fmt.Errorf("operator %q is not valid for %s field %q", c.Op, kind, c.Field)
	}
	v, err := normValue(c.Op, kind, c.Value)
	if err != nil {
		return c, err
	}
	c.Value = v
	return c, nil
}

func toList(v any) ([]any, bool) {
	switch l := v.(type) {
	case []any:
		return l, true
	case []string:
		out := make([]any, len(l))
		for i, s := range l {
			out[i] = s
		}
		return out, true
	}
	return nil, false
}

func normValue(op Op, kind event.FieldKind, v any) (any, error) {
	if op == In || op == AnyOf {
		list, ok := toList(v)
		if !ok {
			return nil, errors.New("value must be an array")
		}
		if len(list) == 0 || len(list) > MaxListLen {
			return nil, fmt.Errorf("array must have 1-%d items", MaxListLen)
		}
		out := make([]any, 0, len(list))
		for _, it := range list {
			nv, err := scalar(kind, it)
			if err != nil {
				return nil, err
			}
			out = append(out, nv)
		}
		return out, nil
	}
	return scalar(kind, v)
}

func scalar(kind event.FieldKind, v any) (any, error) {
	switch kind {
	case event.KindNumber:
		f, ok := event.ToFloat(v)
		if !ok {
			return nil, errors.New("value must be a number")
		}
		return f, nil
	case event.KindString:
		s, ok := v.(string)
		s = strings.TrimSpace(s)
		if !ok || s == "" || len(s) > MaxStringLen {
			return nil, fmt.Errorf("value must be a non-empty string up to %d chars", MaxStringLen)
		}
		return s, nil
	case event.KindBool:
		b, ok := v.(bool)
		if !ok {
			return nil, errors.New("value must be a boolean")
		}
		return b, nil
	}
	return nil, errors.New("unsupported field kind")
}

// Match reports whether ALL conditions hold. A missing field makes a condition
// false (including !=), so conditions never match events of other types by accident.
func Match(conds []Condition, e event.Event) bool {
	for _, c := range conds {
		if !matchOne(c, e) {
			return false
		}
	}
	return true
}

func matchOne(c Condition, e event.Event) bool {
	v, ok := e.Field(c.Field)
	if !ok {
		return false
	}
	if f, isNum := event.ToFloat(v); isNum {
		return matchNumber(c, f)
	}
	switch x := v.(type) {
	case string:
		return matchString(c, x)
	case bool:
		b, ok := c.Value.(bool)
		switch c.Op {
		case Eq:
			return ok && x == b
		case Neq:
			return ok && x != b
		}
	}
	return false
}

func matchNumber(c Condition, f float64) bool {
	if c.Op == In {
		list, ok := toList(c.Value)
		if !ok {
			return false
		}
		for _, it := range list {
			if g, ok := event.ToFloat(it); ok && g == f {
				return true
			}
		}
		return false
	}
	g, ok := event.ToFloat(c.Value)
	if !ok {
		return false
	}
	switch c.Op {
	case Gt:
		return f > g
	case Gte:
		return f >= g
	case Lt:
		return f < g
	case Lte:
		return f <= g
	case Eq:
		return f == g
	case Neq:
		return f != g
	}
	return false
}

func matchString(c Condition, s string) bool {
	switch c.Op {
	case Eq, Neq:
		w, ok := c.Value.(string)
		if !ok {
			return false
		}
		eq := strings.EqualFold(s, w)
		return (c.Op == Eq) == eq
	case Contains:
		w, ok := c.Value.(string)
		return ok && strings.Contains(strings.ToLower(s), strings.ToLower(w))
	case In, AnyOf:
		list, ok := toList(c.Value)
		if !ok {
			return false
		}
		ls := strings.ToLower(s)
		for _, it := range list {
			w, ok := it.(string)
			if !ok {
				continue
			}
			if c.Op == In && strings.EqualFold(s, w) {
				return true
			}
			if c.Op == AnyOf && strings.Contains(ls, strings.ToLower(w)) {
				return true
			}
		}
	}
	return false
}
