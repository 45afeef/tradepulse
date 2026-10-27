package page

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestCursorRoundTripAndRejectsGarbage(t *testing.T) {
	q, err := Parse(10, 200, Cursor(42), "")
	if err != nil || q.Before != 42 || q.After != 0 || q.Asc() {
		t.Fatalf("before: %+v %v", q, err)
	}
	q, err = Parse(10, 200, "", Cursor(7))
	if err != nil || q.After != 7 || !q.Asc() {
		t.Fatalf("after: %+v %v", q, err)
	}
	enc := func(s string) string { return prefix + base64.RawURLEncoding.EncodeToString([]byte(s)) }
	for name, bad := range map[string]string{
		"plain text": "garbage", "prefix only": prefix, "not base64": prefix + "!!!", "not a number": enc("abc"), "negative": enc("-5"),
		"zero": enc("0"), "float": enc("1.5"), "other version": "c2_" + base64.RawURLEncoding.EncodeToString([]byte("5")),
		"too long": prefix + strings.Repeat("A", 60), "overflow": enc("99999999999999999999"),
	} {
		if _, err := Parse(10, 200, bad, ""); err != ErrBadCursor {
			t.Errorf("%s: got %v, want ErrBadCursor", name, err)
		}
		if _, err := Parse(10, 200, "", bad); err != ErrBadCursor {
			t.Errorf("%s (after): got %v", name, err)
		}
	}
}

func TestParseClampsLimitAndRejectsBothCursors(t *testing.T) {
	for in, want := range map[int]int{0: 50, -3: 50, 1: 1, 30: 30, 200: 200, 1000: 200} {
		if q, _ := Parse(in, 200, "", ""); q.Limit != want {
			t.Errorf("limit %d -> %d, want %d", in, q.Limit, want)
		}
	}
	if _, err := Parse(10, 200, Cursor(1), Cursor(2)); err == nil {
		t.Fatal("before and after together must be rejected")
	}
}

func TestSelectOrdersFiltersAndLimits(t *testing.T) {
	items := make([]int64, 10) // ascending seq 1..10
	for i := range items {
		items[i] = int64(i + 1)
	}
	seq := func(x int64) int64 { return x }
	all := func(int64) bool { return true }
	even := func(x int64) bool { return x%2 == 0 }
	cases := []struct {
		name string
		q    Query
		keep func(int64) bool
		want []int64
	}{
		{"newest first", Query{Limit: 3}, all, []int64{10, 9, 8}},
		{"before", Query{Limit: 3, Before: 5}, all, []int64{4, 3, 2}},
		{"before past the start", Query{Limit: 5, Before: 3}, all, []int64{2, 1}},
		{"after is oldest first", Query{Limit: 3, After: 4}, all, []int64{5, 6, 7}},
		{"after the end", Query{Limit: 3, After: 10}, all, []int64{}},
		{"filter desc", Query{Limit: 3}, even, []int64{10, 8, 6}},
		{"filter asc", Query{Limit: 2, After: 3}, even, []int64{4, 6}},
	}
	for _, c := range cases {
		got := Select(items, seq, c.keep, c.q)
		if len(got) != len(c.want) {
			t.Errorf("%s: %v want %v", c.name, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%s: %v want %v", c.name, got, c.want)
			}
		}
	}
}

func TestNextCursor(t *testing.T) {
	q := Query{Limit: 3}
	if NextCursor(q, 2, 5) != "" || NextCursor(q, 0, 0) != "" {
		t.Fatal("a short or empty page has no next cursor")
	}
	if c := NextCursor(q, 3, 5); c != Cursor(5) {
		t.Fatalf("full page: %q", c)
	}
}
