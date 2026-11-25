package pbcli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pb get reads fields without a shell, and never outside data/ or into data/secrets.
func TestGetReadsFieldsAndRefusesSecrets(t *testing.T) {
	data := t.TempDir()
	_ = os.MkdirAll(filepath.Join(data, "stocks", "RLTY"), 0o755)
	_ = os.MkdirAll(filepath.Join(data, "secrets"), 0o755)
	_ = os.WriteFile(filepath.Join(data, "stocks", "RLTY", "derived.yaml"), []byte("yield_pct: 5.46\nrules:\n  safe_dividend: pass\nyears:\n  - {fiscal_year: 2025}\n"), 0o644)
	_ = os.WriteFile(filepath.Join(data, "secrets", "keys.yaml"), []byte("k: v\n"), 0o600)
	_ = os.WriteFile(filepath.Join(data, "watchlist.yaml"), []byte("as_of: x\nsource: t\nitems:\n  - {ticker: RLTY, kind: stock}\n  - {ticker: Z, kind: stock}\n"), 0o644)
	out := run(t, data, "get", "stocks/RLTY/derived.yaml", "yield_pct", "rules.safe_dividend", "years.0.fiscal_year")
	if !strings.Contains(out, `"rules.safe_dividend": "pass"`) || !strings.Contains(out, `"years.0.fiscal_year": 2025`) {
		t.Errorf("%s", out)
	}
	md := run(t, data, "get", "--watchlist", "derived.yaml", "yield_pct", "--md")
	if !strings.Contains(md, "| RLTY | 5.46 |") || !strings.Contains(md, "| Z | no derived.yaml |") {
		t.Errorf("%s", md)
	}
	for _, bad := range []string{"secrets/keys.yaml", "../x.yaml"} {
		root := NewRoot(&strings.Builder{})
		root.SetArgs([]string{"--data", data, "get", bad})
		root.SetErr(&strings.Builder{})
		if err := root.Execute(); err == nil {
			t.Errorf("%s was read", bad)
		}
	}
}
