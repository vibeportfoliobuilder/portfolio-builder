// Package pbcli is the pb command tree: data-only fetchers and the profile's rules.
// Every command prints JSON (or Markdown with --md) to stdout and writes compact,
// normalized YAML under the data folder, replacing the previous file.
package pbcli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/pack"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v4"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/docfmt"
	"github.com/vibeportfoliobuilder/portfolio-builder/internal/store"
)

// env is what every command shares: the data folder, the output switch and the clock.
type env struct {
	data string
	md   bool
	out  io.Writer
	now  func() time.Time
	// keepWatchlist makes pb ly import read its names from data/watchlist.yaml.
	keepWatchlist bool
	importDir     string
}

// NewRoot builds the pb command tree writing to out.
func NewRoot(out io.Writer) *cobra.Command {
	e := &env{data: "data", out: out, now: time.Now}
	root := &cobra.Command{
		Use:   "pb",
		Short: "Data-only fetchers and profile rules for the portfolio-builder agents",
		Long: `pb fetches and normalizes data for the watchlist and applies the investor profile's rules.
It never recommends: data comes from code, moat and fair value from human analysts (a research
connector, your own human_view.yaml or a cited web source), recommendations only from the agents.

Where you live and where you buy are packs (data/config.yaml names them; see pb packs): the home
currency, withholding, transaction taxes and bonds of your country, and your broker's fees.

Every command prints JSON to stdout (Markdown with --md where supported) and writes
compact YAML under --data (default ./data), replacing the previous file in place.

Typical run:
  pb ly screen stocks             # the example screener (Lightyear's public one, no login)
  pb ep fetch --watchlist         # Eulerpool financials -> data/stocks/<T>/financial_data.yaml
  pb ep etf --watchlist           # Eulerpool ETF data   -> data/etfs/<T>/financial_data.yaml
  pb ep macro --watchlist         # rates, FX vs your home currency, CPI, calendar -> data/macro.yaml
  pb derive --watchlist           # profile rules -> data/stocks/<T>/derived.yaml
  pb portfolio check allocation.yaml`,
		SilenceUsage: true,
		// The packs data/config.yaml names, and the strategy in data/profile/strategy.yaml, become
		// active before any command runs. With neither file the active set is left as it is (the
		// made-up example packs and an empty strategy, or the tests' own).
		PersistentPreRunE: func(*cobra.Command, []string) error {
			_, errCfg := os.Stat(filepath.Join(e.data, "config.yaml"))
			_, errStr := os.Stat(filepath.Join(e.data, pack.StrategyFile))
			if errCfg != nil && errStr != nil {
				return nil
			}
			s, err := pack.Load(e.data)
			if err != nil {
				return err
			}
			pack.Use(s)
			return nil
		},
	}
	root.PersistentFlags().StringVar(&e.data, "data", "data", "data folder to read and write")
	root.PersistentFlags().BoolVar(&e.md, "md", false, "print Markdown instead of JSON where the command supports it")
	root.AddCommand(lyCmd(e), epCmd(e), edgarCmd(e), deriveCmd(e), portfolioCmd(e), screenCmd(e), watchlistCmd(e), finalCmd(e), getCmd(e), reportCmd(e), runCmd(e),
		searchCmd(e), fetchURLCmd(e), newsCmd(e), packsCmd(e))
	for _, f := range extraCommands {
		root.AddCommand(f(e))
	}
	return root
}

// printJSON writes v as two-space JSON with the same keys as its YAML files: v goes
// through YAML first, since the source packages tag their types for YAML only.
func (e *env) printJSON(v any) error {
	y, err := docfmt.MarshalYAML(v)
	if err != nil {
		return err
	}
	var generic any
	if err := yaml.Unmarshal(y, &generic); err != nil {
		return err
	}
	b, err := json.MarshalIndent(generic, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(e.out, "%s\n", b)
	return err
}

// path joins parts under the data folder.
func (e *env) path(parts ...string) string {
	return filepath.Join(append([]string{e.data}, parts...)...)
}

// writeYAML writes v as YAML at a path under the data folder, atomically.
func (e *env) writeYAML(v any, parts ...string) (string, error) {
	b, err := docfmt.MarshalYAML(v)
	if err != nil {
		return "", err
	}
	p := e.path(parts...)
	return p, store.WriteFileAtomic(p, b)
}

// readYAML reads a YAML file under the data folder into v; ok is false when absent.
func (e *env) readYAML(v any, parts ...string) (bool, error) {
	b, err := os.ReadFile(e.path(parts...))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := yaml.Unmarshal(b, v); err != nil {
		return false, fmt.Errorf("%s: %w", e.path(parts...), err)
	}
	return true, nil
}

// today is the run date, YYYY-MM-DD.
func (e *env) today() string { return e.now().UTC().Format("2006-01-02") }

// dirName is a ticker as a folder name: upper case, no path separators.
func dirName(t string) string {
	t = strings.ToUpper(strings.TrimSpace(t))
	return strings.NewReplacer("/", "-", ":", "-", " ", "_", "\\", "-").Replace(t)
}

// mdTable renders rows as a Markdown table.
func mdTable(head []string, rows [][]string) string {
	var b bytes.Buffer
	b.WriteString("| " + strings.Join(head, " | ") + " |\n|")
	for range head {
		b.WriteString("---|")
	}
	b.WriteString("\n")
	for _, r := range rows {
		b.WriteString("| " + strings.Join(r, " | ") + " |\n")
	}
	return b.String()
}

// num formats an optional number for a table.
func num(p *float64, dp int) string {
	if p == nil {
		return "–"
	}
	return fmt.Sprintf("%.*f", dp, *p)
}

// extraCommands are commands a connector adds (RegisterCommand, from its init).
var extraCommands []func(*Env) *cobra.Command

// Env is what a registered command gets: the data folder, the clock and the output (the same
// env every built-in command uses).
type Env = env

// RegisterCommand adds a top-level command to every pb built after it. A connector in its own
// package calls it from init; the package is linked in by a blank import in a file of its own
// next to main.go (CONTRIBUTING.md, "Extension points").
func RegisterCommand(f func(*Env) *cobra.Command) { extraCommands = append(extraCommands, f) }

// The helpers a registered command or report extension uses (the same ones the built-ins use).

// Path is a file under the data folder.
func (e *env) Path(parts ...string) string { return e.path(parts...) }

// ReadYAML reads a data/ file into v; false when it does not exist.
func (e *env) ReadYAML(v any, parts ...string) (bool, error) { return e.readYAML(v, parts...) }

// WriteYAML writes v to a data/ file atomically and returns its path.
func (e *env) WriteYAML(v any, parts ...string) (string, error) { return e.writeYAML(v, parts...) }

// PrintJSON writes v to the command's output as JSON.
func (e *env) PrintJSON(v any) error { return e.printJSON(v) }

// Today is the run date, YYYY-MM-DD.
func (e *env) Today() string { return e.today() }

// Markdown reports the --md flag.
func (e *env) Markdown() bool { return e.md }

// Out is the command's output.
func (e *env) Out() io.Writer { return e.out }

// ReadWatchlist reads data/watchlist.yaml.
func (e *env) ReadWatchlist() (Watchlist, error) { return e.readWatchlist() }
