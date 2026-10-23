// Package metrics is a tiny Prometheus-text registry (counters, histograms, callback
// gauges). Label values must come from code (status class, result), never from user
// input, to keep cardinality bounded. All methods are no-ops on a nil *Registry.
package metrics

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
)

var buckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

type hist struct {
	counts []uint64
	sum    float64
	n      uint64
}

type Sample struct {
	Labels []string // key, value, key, value...
	Value  float64
}

type Registry struct {
	mu       sync.Mutex
	help     map[string]string
	kind     map[string]string
	counters map[string]map[string]float64
	hists    map[string]map[string]*hist
	gauges   map[string]func() []Sample
}

func New() *Registry {
	return &Registry{help: map[string]string{}, kind: map[string]string{}, counters: map[string]map[string]float64{},
		hists: map[string]map[string]*hist{}, gauges: map[string]func() []Sample{}}
}

// Describe sets the help text and type ("counter" or "histogram") of a metric.
func (r *Registry) Describe(name, typ, help string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.help[name], r.kind[name] = help, typ
	r.mu.Unlock()
}

func labelString(labels []string) string {
	if len(labels) < 2 {
		return ""
	}
	type kv struct{ k, v string }
	var ps []kv
	for i := 0; i+1 < len(labels); i += 2 {
		ps = append(ps, kv{labels[i], labels[i+1]})
	}
	sort.Slice(ps, func(i, j int) bool { return ps[i].k < ps[j].k })
	var b strings.Builder
	for i, p := range ps {
		if i > 0 {
			b.WriteByte(',')
		}
		v := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(p.v)
		fmt.Fprintf(&b, `%s="%s"`, p.k, v)
	}
	return b.String()
}

func (r *Registry) Inc(name string, labels ...string) { r.Add(name, 1, labels...) }

func (r *Registry) Add(name string, v float64, labels ...string) {
	if r == nil {
		return
	}
	k := labelString(labels)
	r.mu.Lock()
	if r.counters[name] == nil {
		r.counters[name] = map[string]float64{}
	}
	r.counters[name][k] += v
	r.mu.Unlock()
}

// Observe records a value (seconds) in a histogram.
func (r *Registry) Observe(name string, v float64, labels ...string) {
	if r == nil {
		return
	}
	k := labelString(labels)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.hists[name] == nil {
		r.hists[name] = map[string]*hist{}
	}
	h := r.hists[name][k]
	if h == nil {
		h = &hist{counts: make([]uint64, len(buckets))}
		r.hists[name][k] = h
	}
	for i, b := range buckets {
		if v <= b {
			h.counts[i]++
		}
	}
	h.sum += v
	h.n++
}

// GaugeFunc registers a gauge computed at scrape time.
func (r *Registry) GaugeFunc(name, help string, f func() []Sample) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.help[name], r.kind[name], r.gauges[name] = help, "gauge", f
	r.mu.Unlock()
}

func series(name, labels, extra string) string {
	all := labels
	if extra != "" {
		if all != "" {
			all += ","
		}
		all += extra
	}
	if all == "" {
		return name
	}
	return name + "{" + all + "}"
}

// WriteTo renders the Prometheus text exposition format (deterministic order).
func (r *Registry) WriteTo(w io.Writer) (int64, error) {
	if r == nil {
		return 0, nil
	}
	r.mu.Lock()
	gauges := map[string]func() []Sample{}
	for k, f := range r.gauges {
		gauges[k] = f
	}
	var names []string
	seen := map[string]bool{}
	for n := range r.counters {
		names, seen[n] = append(names, n), true
	}
	for n := range r.hists {
		if !seen[n] {
			names, seen[n] = append(names, n), true
		}
	}
	for n := range gauges {
		if !seen[n] {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	var b strings.Builder
	for _, n := range names {
		if h := r.help[n]; h != "" {
			fmt.Fprintf(&b, "# HELP %s %s\n", n, h)
		}
		switch {
		case r.counters[n] != nil:
			fmt.Fprintf(&b, "# TYPE %s counter\n", n)
			for _, k := range sortedKeys(r.counters[n]) {
				fmt.Fprintf(&b, "%s %g\n", series(n, k, ""), r.counters[n][k])
			}
		case r.hists[n] != nil:
			fmt.Fprintf(&b, "# TYPE %s histogram\n", n)
			keys := make([]string, 0, len(r.hists[n]))
			for k := range r.hists[n] {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				h := r.hists[n][k]
				for i, bk := range buckets {
					fmt.Fprintf(&b, "%s %d\n", series(n+"_bucket", k, fmt.Sprintf(`le="%g"`, bk)), h.counts[i])
				}
				fmt.Fprintf(&b, "%s %d\n", series(n+"_bucket", k, `le="+Inf"`), h.n)
				fmt.Fprintf(&b, "%s %g\n%s %d\n", series(n+"_sum", k, ""), h.sum, series(n+"_count", k, ""), h.n)
			}
		}
	}
	r.mu.Unlock()
	// Gauges run outside the lock: callbacks may take their own locks or do I/O.
	for _, n := range names {
		f, ok := gauges[n]
		if !ok {
			continue
		}
		if h := r.helpFor(n); h != "" {
			fmt.Fprintf(&b, "# HELP %s %s\n", n, h)
		}
		fmt.Fprintf(&b, "# TYPE %s gauge\n", n)
		for _, s := range f() {
			fmt.Fprintf(&b, "%s %g\n", series(n, labelString(s.Labels), ""), s.Value)
		}
	}
	n, err := io.WriteString(w, b.String())
	return int64(n), err
}

func (r *Registry) helpFor(name string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.help[name]
}

func sortedKeys(m map[string]float64) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
