package pbcli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sectionDoc = `# Moat Review

Intro.

## Summary
| Ticker | Result |
|---|---|
| RLTY | CONSISTENT |

## RLTY — Realty Example
### Trend test
Flat.
` + "```\n## not a heading\n```" + `

---

## NETW — Network Example
### Trend test
Up.
`

const profileDoc = `# Investor profile

## 1. Snapshot
Snap.

## 6. Strategy rules
Rules.
### 6.1 Groups
G.
### 6.2 Safe
S.
### 6.10 Exclusions
E.

## 10. Not yet settled
N.
`

func sectionData(t *testing.T) string {
	t.Helper()
	data := t.TempDir()
	for p, c := range map[string]string{"runs/2031-03-08/moat_review.md": sectionDoc, "profile/investor_profile.md": profileDoc, "secrets/a.md": "x"} {
		_ = os.MkdirAll(filepath.Join(data, filepath.Dir(p)), 0o755)
		_ = os.WriteFile(filepath.Join(data, p), []byte(c), 0o644)
	}
	return data
}

// pb run section prints a ticker's section, the Summary, or the blocks of numbered headings.
func TestRunSection(t *testing.T) {
	data := sectionData(t)
	out := run(t, data, "run", "section", "runs/2031-03-08/moat_review.md", "netw", "RLTY")
	if !strings.HasPrefix(out, "## NETW — Network Example\n### Trend test\nUp.\n\n## RLTY") || strings.Contains(out, "Summary") || strings.Contains(out, "---") {
		t.Errorf("tickers:\n%s", out)
	}
	if !strings.Contains(out, "## not a heading") {
		t.Errorf("a fenced line ended the section:\n%s", out)
	}
	out = run(t, data, "run", "section", "moat_review.md", "--summary")
	if !strings.HasPrefix(out, "## Summary\n") || strings.Contains(out, "RLTY —") {
		t.Errorf("summary from the newest run:\n%s", out)
	}
	out = run(t, data, "run", "section", "profile/investor_profile.md", "--heading", "1.", "--heading", "6.1", "--heading", "10.")
	if out != "## 1. Snapshot\nSnap.\n\n### 6.1 Groups\nG.\n\n## 10. Not yet settled\nN.\n" {
		t.Errorf("headings:\n%q", out)
	}
	out = run(t, data, "run", "section", "profile/investor_profile.md", "--heading", "6", "--heading", "6.10")
	if strings.Count(out, "### 6.10 Exclusions") != 1 || !strings.HasPrefix(out, "## 6. Strategy rules") || strings.Contains(out, "## 10.") {
		t.Errorf("nested heading printed twice or block too long:\n%s", out)
	}
	out = run(t, data, "run", "section", "profile/investor_profile.md")
	if !strings.Contains(out, "3: ## 1. Snapshot") || !strings.Contains(out, "### 6.10 Exclusions") {
		t.Errorf("table of contents:\n%s", out)
	}
	for _, bad := range [][]string{
		{"runs/2031-03-08/moat_review.md", "AAPL"},
		{"secrets/a.md", "--summary"},
		{"../x.md"},
		{"profile/investor_profile.md", "--summary"},
		{"profile/investor_profile.md", "--heading", "7."},
	} {
		root := NewRoot(&strings.Builder{})
		root.SetArgs(append([]string{"--data", data, "run", "section"}, bad...))
		root.SetErr(&strings.Builder{})
		err := root.Execute()
		if err == nil {
			t.Errorf("%v was accepted", bad)
		}
		if len(bad) > 1 && bad[1] == "AAPL" && (err == nil || !strings.Contains(err.Error(), "sections found: RLTY, NETW")) {
			t.Errorf("unknown ticker error lists no tickers: %v", err)
		}
	}
}

func TestHeadingMatches(t *testing.T) {
	for _, c := range []struct {
		text, prefix string
		want         bool
	}{
		{"6.1 Groups", "6.1", true}, {"6.10 Exclusions", "6.1", false}, {"6.10 Exclusions", "6.10", true},
		{"1. Snapshot", "1.", true}, {"10. Not yet", "1", false}, {"6.1 Groups", "6", true}, {"Summary", "sum", true},
	} {
		if got := headingMatches(c.text, c.prefix); got != c.want {
			t.Errorf("%q %q = %v", c.text, c.prefix, got)
		}
	}
}

// A file pb run join wrote keeps each batch's summary and tables as "#### <heading> (<part>.md)":
// --summary and --heading find them there too.
func TestRunSectionJoinedFile(t *testing.T) {
	md := "# Verdicts (joined from 2 batch files)\n\n### From v_part1.md\n\n#### Summary (v_part1.md)\n\n| A |\n\n" +
		"#### Verdict table (gate 2 of the run) (v_part1.md)\n\n| V1 |\n\n### From v_part2.md\n\n#### Summary (v_part2.md)\n\n| B |\n\n" +
		"## KO — Coca-Cola\n\nbody\n"
	out, err := pickSections(md, "v.md", nil, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "| A |") || !strings.Contains(out, "| B |") || strings.Contains(out, "body") {
		t.Fatalf("summary of a joined file:\n%s", out)
	}
	out, err = pickSections(md, "v.md", nil, false, []string{"Verdict table"})
	if err != nil || !strings.Contains(out, "| V1 |") || strings.Contains(out, "| A |") {
		t.Fatalf("heading in a joined file: %v\n%s", err, out)
	}
}
