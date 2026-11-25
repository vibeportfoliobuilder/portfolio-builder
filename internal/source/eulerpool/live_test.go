package eulerpool

// live_test.go - one cheap request against the real vendor (make test-live sets PB_LIVE_EULERPOOL;
// needs EULERPOOL_API_KEY). A failure here with the unit tests green is a vendor change or an outage.

import (
	"context"
	"os"
	"testing"
)

func TestLiveProfile(t *testing.T) {
	if os.Getenv("PB_LIVE_EULERPOOL") == "" {
		t.Skip("set PB_LIVE_EULERPOOL=1 and EULERPOOL_API_KEY to spend one request")
	}
	c, err := NewClient()
	if err != nil {
		t.Skip(err)
	}
	p, err := c.Profile(context.Background(), "US1912161007") // Coca-Cola, a long-listed public name
	if err != nil {
		t.Fatal(err)
	}
	if p.Name == "" {
		t.Errorf("empty profile %+v", p)
	}
}
