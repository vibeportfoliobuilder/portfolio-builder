// Package packtest is the pack pair the unit tests run on: Testland (currency XTS, ISO 4217's
// test code), Test Broker and a test strategy. Use it from a test with packtest.Use(t) or from TestMain.
package packtest

import (
	_ "embed"

	"go.yaml.in/yaml/v4"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/pack"
)

//go:embed testland.yaml
var countryYAML []byte

//go:embed testbroker.yaml
var brokerYAML []byte

//go:embed teststrategy.yaml
var strategyYAML []byte

// Set is the test pair.
func Set() pack.Set {
	var s pack.Set
	if err := yaml.Unmarshal(countryYAML, &s.Country); err != nil {
		panic(err)
	}
	if err := yaml.Unmarshal(brokerYAML, &s.Broker); err != nil {
		panic(err)
	}
	if err := yaml.Unmarshal(strategyYAML, &s.Strategy); err != nil {
		panic(err)
	}
	s.Language = "en"
	return s
}

// Activate makes the test pair active for the whole test binary (call it from TestMain).
func Activate() { pack.Use(Set()) }
