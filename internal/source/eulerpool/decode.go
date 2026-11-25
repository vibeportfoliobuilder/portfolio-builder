package eulerpool

import (
	"bytes"
	"encoding/json"
	"math"
	"slices"
	"strconv"
	"strings"
)

// Num is a JSON number the vendor may also send as a string ("5", "0.14") or null.
type Num struct {
	V  float64
	OK bool
}

// UnmarshalJSON accepts a number, a numeric string, "" or null.
func (n *Num) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	*n = Num{}
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	s := string(b)
	if b[0] == '"' {
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		s = strings.TrimSpace(s)
		if s == "" {
			return nil
		}
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return nil
	}
	*n = Num{V: v, OK: true}
	return nil
}

// Ptr is the value, or nil when absent.
func (n Num) Ptr() *float64 {
	if !n.OK {
		return nil
	}
	v := n.V
	return &v
}

// Row is one statement line as the vendor sends it: field names vary by country and route.
type Row map[string]json.RawMessage

// Str is a string field, "" when absent.
func (r Row) Str(k string) string {
	var s string
	if raw, ok := r[k]; ok && json.Unmarshal(raw, &s) == nil {
		return s
	}
	return ""
}

// Num is a numeric field, nil when absent or null.
func (r Row) Num(k string) *float64 {
	raw, ok := r[k]
	if !ok {
		return nil
	}
	var n Num
	if n.UnmarshalJSON(raw) != nil {
		return nil
	}
	return n.Ptr()
}

// first is the first of keys present with a value.
func (r Row) first(keys ...string) *float64 {
	for _, k := range keys {
		if v := r.Num(k); v != nil {
			return v
		}
	}
	return nil
}

// decodeRows reads a statement array, sorted by period ascending (the vendor does not sort).
func decodeRows(b []byte) ([]Row, error) {
	var rs []Row
	if err := json.Unmarshal(b, &rs); err != nil {
		return nil, err
	}
	slices.SortStableFunc(rs, func(a, b Row) int { return strings.Compare(a.Str("period"), b.Str("period")) })
	return rs, nil
}

// isEstimate reports a period the vendor marks as forecast with a trailing "e".
func isEstimate(period string) bool { return strings.HasSuffix(period, "e") }

// splitActual separates reported rows from the vendor's estimate rows.
func splitActual(rs []Row) (actual, est []Row) {
	for _, r := range rs {
		if p := r.Str("period"); p == "" {
			continue
		} else if isEstimate(p) {
			est = append(est, r)
		} else {
			actual = append(actual, r)
		}
	}
	return actual, est
}

// round is v to d decimals, nil-safe.
func round(v *float64, d int) *float64 {
	if v == nil || math.IsNaN(*v) || math.IsInf(*v, 0) {
		return nil
	}
	p := math.Pow(10, float64(d))
	r := math.Round(*v*p) / p
	if r == 0 {
		r = 0 // no negative zero in a written file
	}
	return &r
}

func f(v float64) *float64 { return &v }

// nonzero drops a zero the vendor sends for "not computed" (metrics pe, ps).
func nonzero(v *float64) *float64 {
	if v == nil || *v == 0 {
		return nil
	}
	return v
}

// ratio is a/b, nil when either is absent or b is zero.
func ratio(a, b *float64) *float64 {
	if a == nil || b == nil || *b == 0 {
		return nil
	}
	return f(*a / *b)
}

// pct is 100*a/b.
func pct(a, b *float64) *float64 {
	r := ratio(a, b)
	if r == nil {
		return nil
	}
	return f(*r * 100)
}

func neg(v *float64) *float64 {
	if v == nil {
		return nil
	}
	return f(-*v)
}

func sum(vs ...*float64) *float64 {
	var t float64
	seen := false
	for _, v := range vs {
		if v != nil {
			t += *v
			seen = true
		}
	}
	if !seen {
		return nil
	}
	return &t
}
