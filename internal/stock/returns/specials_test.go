package returns

import (
	"testing"
	"time"
)

func TestSpecialsFindsALargeOneOffPayment(t *testing.T) {
	now := time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC)
	var ps []Payment
	for _, d := range []string{"2023-03-01", "2023-06-01", "2023-09-01", "2023-12-01", "2024-03-01", "2024-06-01", "2024-09-01", "2024-12-01", "2025-03-01", "2025-06-01"} {
		ps = append(ps, Payment{ExDate: d, Amount: 0.5})
	}
	ps = append(ps, Payment{ExDate: "2024-12-15", Amount: 2.0})
	s := Specials(ps, now)
	if s == nil || s.Count != 1 || s.LastExDate != "2024-12-15" || s.MaxMultiple != 4 || s.SharePct != 28.6 {
		t.Fatalf("%+v", s)
	}
	// A steady grower has none.
	if Specials(ps[:10], now) != nil {
		t.Error("a regular payer was flagged")
	}
	// A yearly final twice the interim is regular, and so is a raise the next payment confirms.
	var fin []Payment
	for _, y := range []string{"2022", "2023", "2024", "2025", "2026"} {
		fin = append(fin, Payment{ExDate: y + "-03-10", Amount: 0.36}, Payment{ExDate: y + "-08-10", Amount: 0.1})
	}
	if s := Specials(fin, now); s != nil {
		t.Errorf("a final/interim pattern was flagged: %+v", s)
	}
	raise := append(ps[:10:10], Payment{ExDate: "2025-09-01", Amount: 2}, Payment{ExDate: "2025-12-01", Amount: 2})
	if s := Specials(raise, now); s != nil {
		t.Errorf("a raise was flagged: %+v", s)
	}
	// Too few payments to judge.
	if Specials([]Payment{{"2025-01-01", 1}, {"2026-01-01", 3}}, now) != nil {
		t.Error("two payments judged")
	}
}
