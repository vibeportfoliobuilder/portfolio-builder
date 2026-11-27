package pbcli

// newslist_test.go - pb news list on a made-up ledger: one line per item, duplicates folded with
// the other publishers kept, noise dropped and every dropped row counted.

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func newsListData(t *testing.T) string {
	t.Helper()
	data := t.TempDir()
	copyTree(t, "newslist", data)
	return data
}

func TestNewsListPlainKeepsEveryRowNewestFirst(t *testing.T) {
	out := run(t, newsListData(t), "news", "list", "TSTA", "--md")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if !strings.HasPrefix(lines[0], "## TSTA: 11 of 11 items") {
		t.Fatalf("heading: %q", lines[0])
	}
	if len(lines) != 12 || !strings.HasPrefix(lines[1], "0a000009 2030-03-09 IB Market Signals: TSTA Price Spike Up") {
		t.Errorf("want 11 item lines, newest first:\n%s", out)
	}
	if strings.Contains(out, "Left out") {
		t.Errorf("nothing should be left out without filters:\n%s", out)
	}
}

func TestNewsListDedupeFoldsAndCounts(t *testing.T) {
	out := run(t, newsListData(t), "news", "list", "TSTA", "--dedupe", "--md")
	want := []string{
		"## TSTA: 3 of 11 items",
		"Left out: 3 duplicate headline (folded into the first: see also); 1 noise: IB Market Signals price/volume signal; 1 noise: exchange order-imbalance notice; 2 noise: listicle; 1 noise: vendor sector or industry report.",
		"0a000007 2030-03-08 Wire One: Testco raises quarterly dividend to XTS 0.55 (also: Wire Two 0a000008, Portal ep-00000000007a)",
		"0a000005 2030-03-07 Daily Ledger: Testco Profit Falls After Input Costs Rise -- Journal",
		"0a000000 2030-02-10 Wire One: Testco names new chief financial officer",
	}
	got := strings.Split(strings.TrimSpace(out), "\n")
	if len(got) != len(want) {
		t.Fatalf("got\n%s", out)
	}
	for i := range want {
		if !strings.HasPrefix(got[i], want[i]) {
			t.Errorf("line %d:\n got %q\nwant %q", i, got[i], want[i])
		}
	}
}

func TestNewsListSinceAndLimitAreCounted(t *testing.T) {
	data := newsListData(t)
	out := run(t, data, "news", "list", "--tickers", "TSTA", "--dedupe", "--since", "2030-03-01", "--limit", "1")
	var v struct {
		Names []NewsListing `json:"names"`
	}
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if len(v.Names) != 1 {
		t.Fatalf("%s", out)
	}
	l := v.Names[0]
	if l.Total != 11 || l.Shown != 1 || l.Items[0].ID != "0a000007" || l.File != filepath.Join(data, "stocks", "TSTA", "news.yaml") {
		t.Errorf("%+v", l)
	}
	if l.Dropped[dropOlder] != 2 || l.Dropped[dropLimit] != 1 {
		t.Errorf("dropped %v", l.Dropped)
	}
	n := l.Shown
	for _, c := range l.Dropped {
		n += c
	}
	if n != l.Total {
		t.Errorf("shown plus dropped is %d, not the %d rows in the file: a row went missing silently", n, l.Total)
	}
}

func TestHeadlineKeyIgnoresWireTags(t *testing.T) {
	same := []string{"BRIEF-Testco Files For IPO", "Testco files for IPO", "(CNW) Testco Files for IPO", "Testco Files For IPO -- WSJ", "UPDATE 2-Testco files for IPO"}
	for _, h := range same[1:] {
		if headlineKey(h) != headlineKey(same[0]) {
			t.Errorf("%q -> %q, want %q", h, headlineKey(h), headlineKey(same[0]))
		}
	}
	if headlineKey("Testco files for IPO in Testland") == headlineKey(same[0]) {
		t.Error("a longer headline was folded")
	}
}
