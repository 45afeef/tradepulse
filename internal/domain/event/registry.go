package event

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

type FieldKind string

const (
	KindNumber FieldKind = "number"
	KindString FieldKind = "string"
	KindBool   FieldKind = "bool"
)

type FieldDef struct {
	Name string    `json:"name"`
	Kind FieldKind `json:"kind"`
	Doc  string    `json:"doc,omitempty"`
}

// TypeDef describes an event type as data: payload fields (the only fields
// rules may use), title/body templates and default severity. Templates use
// {field} or {field:%.2f}; allowed verbs: %d %f %s with optional + and precision.
type TypeDef struct {
	ID          string     `json:"id"`
	Description string     `json:"description,omitempty"`
	Fields      []FieldDef `json:"fields"`
	Title       string     `json:"title_template"`
	Body        string     `json:"body_template"`
	Severity    string     `json:"severity"` // info | notable | high
}

var builtinKinds = map[string]FieldKind{
	"type": KindString, "source": KindString, "title": KindString,
	"summary": KindString, "url": KindString, "entity": KindString,
}

// BuiltinFieldDefs lists fields available on every event.
func BuiltinFieldDefs() []FieldDef {
	out := make([]FieldDef, 0, len(builtinKinds))
	for n, k := range builtinKinds {
		out = append(out, FieldDef{Name: n, Kind: k})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

var (
	typeIDRe      = regexp.MustCompile(`^[a-z][a-z0-9_]{1,39}$`)
	fieldRe       = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)
	placeholderRe = regexp.MustCompile(`\{([a-z][a-z0-9_]*)(?::(%\+?(?:\.\d+)?[fds]))?\}`)
)

type Registry struct {
	mu    sync.RWMutex
	types map[string]TypeDef
	kinds map[string]FieldKind // global payload-field namespace: one kind per name
}

// NewRegistry returns a registry containing the generic content_item type.
func NewRegistry() *Registry {
	r := &Registry{types: map[string]TypeDef{}, kinds: map[string]FieldKind{}}
	_ = r.Register(TypeDef{
		ID: "content_item", Description: "A published item (article, post, announcement)",
		Fields: []FieldDef{{Name: "author", Kind: KindString}},
		Title:  "{title}", Body: "{summary}", Severity: "info",
	})
	return r
}

func validKind(k FieldKind) bool { return k == KindNumber || k == KindString || k == KindBool }

// Register validates and adds a type. A payload field name has one kind across
// all types, so rules over that field are unambiguous.
func (r *Registry) Register(t TypeDef) error {
	if !typeIDRe.MatchString(t.ID) {
		return fmt.Errorf("invalid type id %q", t.ID)
	}
	switch t.Severity {
	case "":
		t.Severity = "info"
	case "info", "notable", "high":
	default:
		return fmt.Errorf("type %s: invalid severity %q", t.ID, t.Severity)
	}
	own := map[string]bool{}
	for _, f := range t.Fields {
		if !fieldRe.MatchString(f.Name) {
			return fmt.Errorf("type %s: invalid field name %q", t.ID, f.Name)
		}
		if _, builtin := builtinKinds[f.Name]; builtin {
			return fmt.Errorf("type %s: field %q collides with a built-in field", t.ID, f.Name)
		}
		if !validKind(f.Kind) {
			return fmt.Errorf("type %s: field %q has invalid kind %q", t.ID, f.Name, f.Kind)
		}
		if own[f.Name] {
			return fmt.Errorf("type %s: duplicate field %q", t.ID, f.Name)
		}
		own[f.Name] = true
	}
	for _, tmpl := range []string{t.Title, t.Body} {
		for _, m := range placeholderRe.FindAllStringSubmatch(tmpl, -1) {
			if _, builtin := builtinKinds[m[1]]; !own[m[1]] && !builtin {
				return fmt.Errorf("type %s: template references unknown field %q", t.ID, m[1])
			}
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.types[t.ID]; dup {
		return fmt.Errorf("type %q already registered", t.ID)
	}
	for _, f := range t.Fields {
		if k, ok := r.kinds[f.Name]; ok && k != f.Kind {
			return fmt.Errorf("type %s: field %q is %s elsewhere, not %s", t.ID, f.Name, k, f.Kind)
		}
	}
	for _, f := range t.Fields {
		r.kinds[f.Name] = f.Kind
	}
	r.types[t.ID] = t
	return nil
}

func (r *Registry) Get(id string) (TypeDef, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.types[id]
	return t, ok
}

func (r *Registry) List() []TypeDef {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]TypeDef, 0, len(r.types))
	for _, t := range r.types {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// KindOf reports the kind of a built-in or registered payload field. It is the
// whitelist rules are validated against.
func (r *Registry) KindOf(field string) (FieldKind, bool) {
	if k, ok := builtinKinds[field]; ok {
		return k, true
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	k, ok := r.kinds[field]
	return k, ok
}

// Validate checks an event against its type definition.
func (r *Registry) Validate(e Event) error {
	td, ok := r.Get(e.Type)
	if !ok {
		return fmt.Errorf("unknown event type %q", e.Type)
	}
	if len(e.Entities) > 20 {
		return fmt.Errorf("too many entities (%d)", len(e.Entities))
	}
	for _, f := range td.Fields {
		v, present := e.Payload[f.Name]
		if !present {
			continue
		}
		okKind := false
		switch f.Kind {
		case KindNumber:
			_, okKind = ToFloat(v)
		case KindString:
			_, okKind = v.(string)
		case KindBool:
			_, okKind = v.(bool)
		}
		if !okKind {
			return fmt.Errorf("field %q: expected %s", f.Name, f.Kind)
		}
	}
	return nil
}

// Render produces user-facing title/body and severity for an event.
func (r *Registry) Render(e Event) (title, body, severity string) {
	td, ok := r.Get(e.Type)
	if !ok {
		return firstNonEmpty(e.Title, e.Type), e.Summary, "info"
	}
	title = strings.TrimSpace(renderTemplate(td.Title, e))
	if title == "" {
		title = firstNonEmpty(e.Title, e.Type)
	}
	return title, strings.TrimSpace(renderTemplate(td.Body, e)), td.Severity
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func renderTemplate(tmpl string, e Event) string {
	return placeholderRe.ReplaceAllStringFunc(tmpl, func(m string) string {
		sub := placeholderRe.FindStringSubmatch(m)
		v, ok := e.Field(sub[1])
		if !ok {
			return ""
		}
		return formatValue(v, sub[2])
	})
}

func formatValue(v any, verb string) string {
	if f, ok := ToFloat(v); ok {
		switch {
		case verb == "":
			return strconv.FormatFloat(f, 'f', -1, 64)
		case strings.HasSuffix(verb, "f"):
			return fmt.Sprintf(verb, f)
		case strings.HasSuffix(verb, "d"):
			return fmt.Sprintf(verb, int64(f))
		}
		return fmt.Sprint(f)
	}
	switch x := v.(type) {
	case string:
		return x
	case bool:
		return strconv.FormatBool(x)
	}
	return fmt.Sprint(v)
}
