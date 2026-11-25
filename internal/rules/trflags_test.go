package rules

import "testing"

func TestTotalReturnFlags(t *testing.T) {
	st := func(c []Check) (string, string) { return c[0].Status, c[1].Status }
	for _, tc := range []struct {
		name            string
		y, net, p10, g5 *float64
		wantR1b, wantR3 string
	}{
		{"high yield, falling price", p(7), p(7), p(-3), p(2), Flagged, Pass},
		{"high yield, flat price", p(7), p(4.9), p(-1), p(2), Pass, Fail},
		{"high yield, no 10y", p(6), p(6), nil, p(3), NeedsLook, Pass},
		{"low yield, low growth", p(3), p(2.1), p(-5), p(4), Skipped, Flagged},
		{"no 5y growth", p(4), p(4), p(5), nil, Skipped, NeedsLook},
		{"no dividend", p(0), p(0), p(10), nil, Skipped, Skipped},
		{"unknown yield", nil, nil, p(10), p(5), NeedsLook, NeedsLook},
	} {
		if tc.wantR3 == Fail {
			tc.wantR3 = Flagged // 4.9 + 2 < 8
		}
		r1b, r3 := st(TotalReturnFlags(tc.y, tc.net, tc.p10, tc.g5))
		if r1b != tc.wantR1b || r3 != tc.wantR3 {
			t.Errorf("%s: R1b %s want %s, R3 %s want %s", tc.name, r1b, tc.wantR1b, r3, tc.wantR3)
		}
	}
	c := TotalReturnFlags(p(5), p(3.5), p(1), p(4.5))
	if c[1].Value == nil || *c[1].Value != 8 || c[1].Status != Pass {
		t.Errorf("R3 at the limit passes: %+v", c[1])
	}
}
