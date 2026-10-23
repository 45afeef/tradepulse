package metrics

import (
	"bytes"
	"strings"
	"sync"
	"testing"
)

func render(t *testing.T, r *Registry) string {
	t.Helper()
	var b bytes.Buffer
	if _, err := r.WriteTo(&b); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestCountersRenderWithSortedEscapedLabels(t *testing.T) {
	r := New()
	r.Describe("req_total", "counter", "Requests")
	r.Inc("req_total", "class", "2xx")
	r.Inc("req_total", "class", "2xx")
	r.Add("req_total", 3, "class", "5xx", "a", `x"y\z`+"\n")
	out := render(t, r)
	for _, want := range []string{"# HELP req_total Requests", "# TYPE req_total counter", `req_total{class="2xx"} 2`, `req_total{a="x\"y\\z\n",class="5xx"} 3`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if out != render(t, r) {
		t.Fatal("output must be deterministic")
	}
}

func TestHistogramIsCumulative(t *testing.T) {
	r := New()
	for _, v := range []float64{0.001, 0.02, 0.02, 3} {
		r.Observe("lat_seconds", v)
	}
	out := render(t, r)
	for _, want := range []string{`lat_seconds_bucket{le="0.005"} 1`, `lat_seconds_bucket{le="0.025"} 3`, `lat_seconds_bucket{le="2.5"} 3`, `lat_seconds_bucket{le="5"} 4`, `lat_seconds_bucket{le="+Inf"} 4`, "lat_seconds_count 4", "# TYPE lat_seconds histogram"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestGaugeFuncsAreEvaluatedAtScrapeTime(t *testing.T) {
	r := New()
	v := 1.0
	r.GaugeFunc("things", "Things", func() []Sample { return []Sample{{Labels: []string{"kind", "a"}, Value: v}, {Value: 9}} })
	if out := render(t, r); !strings.Contains(out, `things{kind="a"} 1`) || !strings.Contains(out, "things 9") || !strings.Contains(out, "# TYPE things gauge") {
		t.Fatalf("gauge: %s", out)
	}
	v = 5
	if !strings.Contains(render(t, r), `things{kind="a"} 5`) {
		t.Fatal("gauge must reflect the current value")
	}
}

func TestNilRegistryIsSafe(t *testing.T) {
	var r *Registry
	r.Describe("x", "counter", "x")
	r.Inc("x")
	r.Add("x", 2)
	r.Observe("y", 1)
	r.GaugeFunc("z", "z", nil)
	var b bytes.Buffer
	if n, err := r.WriteTo(&b); n != 0 || err != nil || b.Len() != 0 {
		t.Fatal("a nil registry must be a silent no-op")
	}
}

func TestConcurrentIncrementsAreExact(t *testing.T) {
	r := New()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				r.Inc("hits")
				r.Observe("d", 0.01)
			}
		}()
	}
	wg.Wait()
	out := render(t, r)
	if !strings.Contains(out, "hits 10000") || !strings.Contains(out, "d_count 10000") {
		t.Fatalf("lost updates:\n%s", out)
	}
}
