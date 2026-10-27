// Package page implements the API's cursor pagination convention (see docs/API.md).
//
// A cursor is an opaque token for a position in a time-ordered list (internally a
// monotonically increasing sequence number). Clients must treat it as opaque.
//
//	before=<cursor>  items older than the position, newest first   (scrolling back)
//	after=<cursor>   items newer than the position, OLDEST first    (catch-up after a reconnect)
//
// Responses carry next_cursor: pass it as before (newest-first mode) or as after
// (oldest-first mode) to get the next page; it is empty when the list is exhausted.
package page

import (
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
)

var ErrBadCursor = errors.New("invalid cursor")

const prefix = "c1_"

// Cursor encodes a sequence number.
func Cursor(seq int64) string {
	return prefix + base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(seq, 10)))
}

func decode(s string) (int64, error) {
	if !strings.HasPrefix(s, prefix) || len(s) > 40 {
		return 0, ErrBadCursor
	}
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(s, prefix))
	if err != nil {
		return 0, ErrBadCursor
	}
	n, err := strconv.ParseInt(string(b), 10, 64)
	if err != nil || n < 1 {
		return 0, ErrBadCursor
	}
	return n, nil
}

type Query struct {
	Limit         int
	Before, After int64 // sequence numbers; 0 = unset
}

// Asc reports catch-up mode (oldest first).
func (q Query) Asc() bool { return q.After > 0 }

// Parse validates the request parameters. limit is clamped to 1..max (default 50).
func Parse(limit, max int, before, after string) (Query, error) {
	if limit < 1 {
		limit = 50
	}
	if limit > max {
		limit = max
	}
	q := Query{Limit: limit}
	if before != "" && after != "" {
		return q, errors.New("use either before or after, not both")
	}
	var err error
	if before != "" {
		if q.Before, err = decode(before); err != nil {
			return q, err
		}
	}
	if after != "" {
		if q.After, err = decode(after); err != nil {
			return q, err
		}
	}
	return q, nil
}

// NextCursor returns the cursor for the following page, or "" when this page was not full.
func NextCursor(q Query, count int, lastSeq int64) string {
	if count == 0 || count < q.Limit {
		return ""
	}
	return Cursor(lastSeq)
}

// Select applies a query to an in-memory list ordered by ascending sequence number.
func Select[T any](asc []T, seq func(T) int64, keep func(T) bool, q Query) []T {
	out := []T{}
	if q.Asc() {
		for _, it := range asc {
			if len(out) >= q.Limit {
				break
			}
			if seq(it) > q.After && keep(it) {
				out = append(out, it)
			}
		}
		return out
	}
	for i := len(asc) - 1; i >= 0 && len(out) < q.Limit; i-- {
		it := asc[i]
		if (q.Before == 0 || seq(it) < q.Before) && keep(it) {
			out = append(out, it)
		}
	}
	return out
}
