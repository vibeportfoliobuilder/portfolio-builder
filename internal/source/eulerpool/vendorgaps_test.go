package eulerpool

// vendorgaps_test.go - the gaps true of every dossier are one short line per dossier, their full
// text kept once (VendorGaps).

import (
	"strings"
	"testing"
)

func TestVendorGapsAreOneLine(t *testing.T) {
	s := &fetchState{d: &Dossier{}}
	s.gap("eulerpool: the insider feed ends 2030-01-01; later transactions are missing")
	s.vendorGap(gapAltmanZ)
	s.vendorGap(gapSpecial)
	s.finishVendorGaps()
	g := s.d.DataGaps
	if len(g) != 2 || g[1] != "eulerpool (vendor-wide, full text in data/vendor_gaps.yaml): no Altman Z; no special-dividend flag" {
		t.Fatalf("%q", g)
	}
	for _, v := range VendorGaps() {
		if v.Short == "" || !strings.HasPrefix(v.Text, "eulerpool: ") || strings.Contains(strings.Join(g, "\n"), v.Text) {
			t.Errorf("%+v", v)
		}
	}
	empty := &fetchState{d: &Dossier{}}
	empty.finishVendorGaps()
	if len(empty.d.DataGaps) != 0 {
		t.Errorf("a dossier with no vendor-wide gap got %q", empty.d.DataGaps)
	}
}
