package pbcli

// screen_enrich.go - pb screen enrich: what the public screener lacks, on the run's screen.yaml
// Funds get yield, TER, distribution policy and AUM from the broker's own screener (when a
// connector saved its rows) and TER, AUM and holdings of 0.5%+ from Eulerpool; stocks get an
// industry (Eulerpool's profile, else the broker's finer sector).

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/pack"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/source/eulerpool"
)

func screenEnrichCmd(e *env) *cobra.Command {
	var run, brokerDir string
	var noEP bool
	c := &cobra.Command{
		Use:   "enrich [--run D] [--broker DIR] [--no-eulerpool]",
		Short: "Add fund yield/TER/policy/AUM/holdings and stock industries to the run's screen.yaml",
		Long: `Fills what the public screener leaves out, so the Screen page can judge funds and sectors:
  funds:  yield, TER, distribution policy, AUM and benchmark from the broker's own screener: the
          pages a broker connector saved in DIR (--broker; needs a connector that reads them);
          TER, AUM and every holding of 0.5% or more from Eulerpool (2 cached requests per ISIN).
  stocks: the industry from Eulerpool's profile (1 cached request each), else the broker's sector.
An accumulating fund's yield is 0. The yield also goes into the fund's returns block, which the page's
Yield column reads. Run after pb ep returns --screen (which rewrites the returns blocks).
Example:
  pb screen enrich --run <date>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			r := cmp.Or(run, e.today())
			var doc ScreenDoc
			ok, err := e.readYAML(&doc, "runs", r, "screen.yaml")
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("no screen.yaml for run %s", r)
			}
			rep := map[string]any{}
			if brokerDir != "" {
				enrichFromBroker := screenEnrichers[strings.ToLower(pack.Active().Broker.Key)]
				if enrichFromBroker == nil {
					return errors.New("--broker: no broker connector that reads a saved screener is installed")
				}
				n, err := enrichFromBroker(&doc, brokerDir)
				if err != nil {
					return err
				}
				rep["broker_rows"] = n
			}
			if !noEP {
				cl, err := epClient()
				if err != nil {
					return err
				}
				rep["eulerpool_errors"] = enrichFromEulerpool(cmd.Context(), cl, &doc)
			}
			p, err := e.writeYAML(doc, "runs", r, "screen.yaml")
			if err != nil {
				return err
			}
			rep["file"] = p
			return e.printJSON(rep)
		},
	}
	c.Flags().StringVar(&run, "run", "", "run date (default today)")
	c.Flags().StringVar(&brokerDir, "broker", "", "folder with the broker screener pages a connector saved")
	c.Flags().BoolVar(&noEP, "no-eulerpool", false, "skip the Eulerpool requests")
	return c
}

// ScreenEnricher merges a broker's own screener rows (saved by its connector in dir) into the
// run's candidates and returns how many rows it read. A broker connector registers one.
type ScreenEnricher func(doc *ScreenDoc, dir string) (int, error)

var screenEnrichers = map[string]ScreenEnricher{}

// RegisterScreenEnricher installs the broker screener merge pb screen enrich --broker runs while
// the broker pack named key is active.
func RegisterScreenEnricher(key string, f ScreenEnricher) {
	screenEnrichers[strings.ToLower(strings.TrimSpace(key))] = f
}

// NotCoveredFlag marks a screen stock Eulerpool does not carry: no statements, dividend ledger or returns
// from the vendor, so the analysts would mark its figures DATA UNAVAILABLE (the screener's rows still apply).
const NotCoveredFlag = "not covered by Eulerpool: no vendor financials, dividend ledger or returns"

// enrichFromEulerpool adds TER, AUM and holdings to funds, and the industry to stocks.
func enrichFromEulerpool(ctx context.Context, cl *eulerpool.Client, doc *ScreenDoc) []string {
	var mu sync.Mutex
	var errs []string
	fail := func(t string, err error) {
		mu.Lock()
		defer mu.Unlock()
		why := err.Error()
		if errors.Is(err, eulerpool.ErrNotFound) {
			why = "not covered by Eulerpool"
		}
		errs = append(errs, t+": "+why)
	}
	facts := map[string]*eulerpool.ETFFacts{}
	var isins []string
	for _, f := range doc.Funds {
		if f.ISIN != "" && facts[f.ISIN] == nil {
			facts[f.ISIN] = &eulerpool.ETFFacts{}
			isins = append(isins, f.ISIN)
		}
	}
	sort.Strings(isins)
	pool(len(isins), func(i int) {
		x, err := cl.Facts(ctx, isins[i])
		if err != nil {
			fail(isins[i], err)
			return
		}
		mu.Lock()
		*facts[isins[i]] = x
		mu.Unlock()
	})
	for i := range doc.Funds {
		c := &doc.Funds[i]
		x := facts[c.ISIN]
		if x == nil || (x.TER == nil && x.AUMMillions == nil && len(x.Holdings) == 0) {
			continue
		}
		f := factsOf(c)
		f.TERPct = cmp.Or(f.TERPct, x.TER)
		f.AUMMillions = cmp.Or(f.AUMMillions, x.AUMMillions)
		f.Holdings = x.Holdings
		addSource(f, "eulerpool etf profile and holdings")
	}
	pool(len(doc.Stocks), func(i int) {
		c := &doc.Stocks[i]
		if c.ISIN == "" || c.Industry != "" {
			return
		}
		p, err := cl.Profile(ctx, c.ISIN)
		if err != nil {
			fail(c.Ticker, err)
			if errors.Is(err, eulerpool.ErrNotFound) {
				// Shown on the Screen page: picking it means no vendor financials downstream.
				mu.Lock()
				c.Flags = append(c.Flags, NotCoveredFlag)
				mu.Unlock()
			}
			return
		}
		c.Industry = cmp.Or(p.Industry, p.Sector)
	})
	sort.Strings(errs)
	return errs
}

func factsOf(c *FundCandidate) *FundFacts {
	if c.Facts == nil {
		c.Facts = &FundFacts{}
	}
	return c.Facts
}

func addSource(f *FundFacts, s string) {
	for _, x := range f.Sources {
		if x == s {
			return
		}
	}
	f.Sources = append(f.Sources, s)
}

func ptrF(v float64) *float64 { return &v }
