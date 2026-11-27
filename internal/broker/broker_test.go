package broker

// broker_test.go - a news row read from an older broker.yaml loses its related-instrument ids
// when it is written again.

import (
	"strings"
	"testing"

	"go.yaml.in/yaml/v4"
)

func TestNewsItemCarriesNoInstrumentIDs(t *testing.T) {
	var s Stock
	in := "news:\n- id: n1\n  title: Testco raises its dividend\n  type: ARTICLE\n  published: \"2030-03-08\"\n  instrument_ids:\n  - 00000000-0000-0000-0000-000000000001\n"
	if err := yaml.Unmarshal([]byte(in), &s); err != nil {
		t.Fatal(err)
	}
	out, err := yaml.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.News) != 1 || strings.Contains(string(out), "instrument_ids") {
		t.Errorf("%s", out)
	}
}
