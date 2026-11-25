package docfmt

import (
	"bytes"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The rendering must be stable across machines and the shortest form that still
// parses back exactly; those pull against each other, so both are asserted.
func TestFormatNumIsStableAndShortest(t *testing.T) {
	for _, tc := range []struct {
		in   float64
		want string
	}{
		{43530290.51, "43530290.51"}, // a net-assets figure, as published
		{0.0007, "0.0007"},           // a 0.07% TER, as a fraction
		{1e-7, "0.0000001"},          // no exponent, however small
		{1e12, "1000000000000"},      // no exponent, however large
		{0.1, "0.1"},                 // not 0.100000000000000006
		{100, "100"},                 // a full-weight holding, no ".0" tail
		{-2.5, "-2.5"},
		{0, "0"},
		{math.Copysign(0, -1), "0"}, // negative zero must not churn the line
		{math.NaN(), ""},            // not a measurement; not "NaN"
		{math.Inf(1), ""},
		{math.Inf(-1), ""},
	} {
		if got := FormatNum(tc.in); got != tc.want {
			t.Errorf("formatNum(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// Shortest is only acceptable because it is exact: a value that failed to
// round-trip would store a different number than the publisher sent.
func TestFormatNumRoundTrips(t *testing.T) {
	for _, v := range []float64{
		43530290.51, 0.0007, 1e-7, 1e12, 0.1, 0.2, 0.3, 1.0 / 3.0,
		2.675, 9007199254740993, -0.000123456789, 123456789.123456789,
		math.MaxFloat64, math.SmallestNonzeroFloat64, -math.MaxFloat64,
	} {
		s := FormatNum(v)
		got, err := strconv.ParseFloat(s, 64)
		if err != nil {
			t.Errorf("formatNum(%v) = %q, which does not parse: %v", v, s, err)
			continue
		}
		if got != v {
			t.Errorf("formatNum(%v) = %q, parses back as %v", v, s, got)
		}
		if strings.ContainsAny(s, "eE") {
			t.Errorf("formatNum(%v) = %q, which uses exponent notation", v, s)
		}
	}
}

// Quoting everything is easier and quoting nothing is smaller; the file wants
// exactly the cells that would otherwise be misread, and no others.
func TestCSVFieldQuotesOnlyWhenNeeded(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
	}{
		{"CSPX", "CSPX"},
		{"", ""},
		{"iShares Core S&P 500 UCITS ETF", "iShares Core S&P 500 UCITS ETF"},
		{"0.0007", "0.0007"},
		{"SPOT PHYSICAL GOLD", "SPOT PHYSICAL GOLD"}, // interior spaces are fine
		{"Berkshire Hathaway, Inc.", `"Berkshire Hathaway, Inc."`},
		{`SPDR "core" fund`, `"SPDR ""core"" fund"`},
		{"line one\nline two", "\"line one\nline two\""},
		{"trailing cr\r", "\"trailing cr\r\""},
		{" leading space", `" leading space"`},
		{"trailing space ", `"trailing space "`},
		{"\ttab led", "\"\ttab led\""},
		{" ", `" "`},
	} {
		if got := CSVField(tc.in); got != tc.want {
			t.Errorf("csvField(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFormatDateIsUTCAndZeroSafe(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   time.Time
		want string
	}{
		{"zero renders empty", time.Time{}, ""},
		{"plain date", time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC), "2026-08-05"},
		{"clock is dropped", time.Date(2026, 2, 14, 23, 59, 59, 0, time.UTC), "2026-02-14"},
		{"single digits pad", time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), "2026-01-02"},
		{
			"an instant east of Greenwich lands on its UTC day",
			time.Date(2026, 8, 5, 1, 0, 0, 0, time.FixedZone("CEST", 2*3600)),
			"2026-08-04",
		},
	} {
		if got := FormatDate(tc.in); got != tc.want {
			t.Errorf("%s: formatDate(%v) = %q, want %q", tc.name, tc.in, got, tc.want)
		}
	}
}

// The committed bytes must not depend on the machine that ran the fetch: a UTC
// date renders the same whatever TZ the process runs under.
func TestFormatDateDoesNotDependOnTheLocalZone(t *testing.T) {
	d := time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC)
	want := "2026-08-05"
	for _, offset := range []int{-11 * 3600, -5 * 3600, 0, 2 * 3600, 14 * 3600} {
		zone := time.FixedZone("test", offset)
		if got := FormatDate(d.In(zone)); got != want {
			t.Errorf("offset %+ds: formatDate = %q, want %q", offset, got, want)
		}
	}
}

func TestWriteCSVRows(t *testing.T) {
	var buf bytes.Buffer
	err := WriteCSVRows(&buf, []string{"isin", "ticker", "name", "weight"}, [][]string{
		{"", "", "USD CASH", "0.42"},
		{"US0000000113", "AAPL", "Apple Inc.", "7.1"},
		{"US0000000114", "AMZN", "Amazon.com, Inc.", "3.5"},
	})
	if err != nil {
		t.Fatalf("writeCSVRows: %v", err)
	}
	want := "isin,ticker,name,weight\n" +
		",,USD CASH,0.42\n" +
		"US0000000113,AAPL,Apple Inc.,7.1\n" +
		"US0000000114,AMZN,\"Amazon.com, Inc.\",3.5\n"
	if got := buf.String(); got != want {
		t.Errorf("writeCSVRows wrote\n%q\nwant\n%q", got, want)
	}
}

// LF, no BOM, and a trailing newline: the file has to diff cleanly and end like
// every other text file in the tree.
func TestWriteCSVRowsUsesLFAndEndsWithNewline(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteCSVRows(&buf, []string{"date", "nav"}, [][]string{{"2026-08-05", "512.44"}}); err != nil {
		t.Fatalf("writeCSVRows: %v", err)
	}
	got := buf.String()
	if strings.Contains(got, "\r") {
		t.Errorf("output contains CR: %q", got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Errorf("output does not end with a newline: %q", got)
	}
	if strings.HasPrefix(got, "\ufeff") {
		t.Errorf("output starts with a BOM: %q", got)
	}
	if n := strings.Count(got, "\n"); n != 2 {
		t.Errorf("header + 1 row should be 2 lines, got %d: %q", n, got)
	}
}

// A short row means either a parser stopped early or a publisher dropped a
// column. Padding it would erase the difference, so it is an error.
func TestWriteCSVRowsRejectsRaggedRows(t *testing.T) {
	var buf bytes.Buffer
	err := WriteCSVRows(&buf, []string{"date", "nav", "net_assets"}, [][]string{
		{"2026-08-05", "512.44", "43530290.51"},
		{"2026-08-06", "513.10"},
	})
	if err == nil {
		t.Fatalf("ragged row accepted, wrote %q", buf.String())
	}
	if !strings.Contains(err.Error(), "row 1") {
		t.Errorf("error should name the offending row, got %v", err)
	}
}

func TestWriteCSVRowsRefusesAnEmptyHeader(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteCSVRows(&buf, nil, [][]string{{"a"}}); err == nil {
		t.Fatalf("empty header accepted, wrote %q", buf.String())
	}
}

// Header and no rows is legal at this level - the decision to omit an empty
// series belongs to the writer (T2), not to the formatter.
func TestWriteCSVRowsAcceptsHeaderOnly(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteCSVRows(&buf, []string{"date", "ratio"}, nil); err != nil {
		t.Fatalf("writeCSVRows: %v", err)
	}
	if got, want := buf.String(), "date,ratio\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
