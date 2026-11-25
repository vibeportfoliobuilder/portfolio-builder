package rules

import (
	"os"
	"testing"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/pack/packtest"
)

// TestMain runs the package's tests on the test packs (Testland, currency XTS, and Test Broker).
func TestMain(m *testing.M) {
	packtest.Activate()
	os.Exit(m.Run())
}
