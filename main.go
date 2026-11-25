// Command pb is the data-only binary of portfolio-builder: Eulerpool, EDGAR, Tavily and the example
// screener's fetchers, the country and broker packs, and the investor profile's rules.
package main

import (
	"fmt"
	"os"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/dotenv"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/pbcli"
)

func main() {
	// Credentials come from the environment; an exported variable wins over .env.
	if _, err := dotenv.Load(); err != nil {
		fmt.Fprintln(os.Stderr, "warning:", err)
	}
	if err := pbcli.NewRoot(os.Stdout).Execute(); err != nil {
		os.Exit(1)
	}
}
