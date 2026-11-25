package pbcli

// packs.go - pb packs: which country and broker packs are active, and the built-in ones.

import (
	"github.com/spf13/cobra"

	"github.com/vibeportfoliobuilder/portfolio-builder/internal/pack"
)

func packsCmd(e *env) *cobra.Command {
	return &cobra.Command{
		Use:   "packs",
		Short: "Show the active country and broker packs (data/config.yaml) and the built-in ones",
		Long: `A country pack is where you live: home currency, inflation target, the withholding a dividend
suffers at source, a transaction tax on your home market, the bonds the switch to income buys.
A broker pack is where you buy: fees per market, FX cost, where its withholding differs.
It also prints your strategy (data/profile/strategy.yaml) and strategy_unset: the keys you have not
set, whose rules are skipped.
data/config.yaml names them; data/packs/country/<name>.yaml and data/packs/broker/<name>.yaml
are read first, else a built-in pack. With no config.yaml the made-up examples are used.
Example data/config.yaml:
  country: exampleland
  broker: examplebroker
  language: en`,
		Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			a := pack.Active()
			return e.printJSON(map[string]any{
				"active":           map[string]string{"country": a.Country.Name, "currency": a.Country.Currency, "broker": a.Broker.Name, "language": a.Language},
				"built_in_country": pack.Names("country"),
				"built_in_broker":  pack.Names("broker"),
				"country":          a.Country,
				"broker":           a.Broker,
				"strategy":         a.Strategy,
				"strategy_unset":   a.Strategy.Unset(),
			})
		},
	}
}
