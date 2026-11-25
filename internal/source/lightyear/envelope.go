package lightyear

// envelope.go - the two blocks an explore page carries, shared by the fund
// screener and the company one: they differ only in the columns a row states.

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
)

// The two blocks' markers. Each appears exactly once in a page's payload, which
// is why the rows are found by a spelling that includes their first field.
const (
	rowsKey        = `"data":[{"instrumentId"`
	instrumentsKey = `"instruments":`
)

// totalRe reads the screener envelope's own count. The three keys before it are
// matched too, so that a "total" belonging to some other block cannot answer.
var totalRe = regexp.MustCompile(`"page":\d+,"limit":\d+,"total":(\d+),"data":\[`)

// instrument is identity - it is the only one of the two blocks that states an
// ISIN - and is spelled the same on both screeners.
type instrument struct {
	ID        string `json:"id"`
	Symbol    string `json:"symbol"`
	Name      string `json:"name"`
	Exchange  string `json:"exchange"`
	ISIN      string `json:"isin"`
	MIC       string `json:"mic"`
	Currency  string `json:"currency"`
	IssueType string `json:"issueType"`
	Summary   struct {
		DistributionPolicy string `json:"distributionPolicy"`
		Sector             string `json:"sector"`
	} `json:"summary"`
}

// envelope is one decoded page before its rows are read: the raw row array, the
// instruments they join to, and the count that bounds the walk.
type envelope struct {
	rows  []byte
	byID  map[string]instrument
	total int
}

// readEnvelope pulls both blocks out of a page. The row columns are left raw
// because the two screeners state different ones.
func readEnvelope(body []byte) (envelope, error) {
	f, err := flight(body)
	if err != nil {
		return envelope{}, err
	}
	m := totalRe.FindStringSubmatch(f)
	if m == nil {
		return envelope{}, errors.New("lightyear: no screener envelope on the page: the explore page's payload shape has changed")
	}
	total, err := strconv.Atoi(m[1])
	if err != nil {
		return envelope{}, fmt.Errorf("lightyear: screener total %q: %w", m[1], err)
	}

	// The screener's rows are keyed on the first row's own first field, because
	// "data" alone appears throughout the payload and this spelling appears once.
	rawRows, err := valueAfter(f, rowsKey, len(`"data":`))
	if err != nil {
		return envelope{}, fmt.Errorf("lightyear: %w", err)
	}
	rawInst, err := valueAfter(f, instrumentsKey, len(instrumentsKey))
	if err != nil {
		return envelope{}, fmt.Errorf("lightyear: %w", err)
	}
	var insts []instrument
	if err := json.Unmarshal([]byte(rawInst), &insts); err != nil {
		return envelope{}, fmt.Errorf("lightyear: instruments: %w", err)
	}
	byID := make(map[string]instrument, len(insts))
	for _, in := range insts {
		byID[in.ID] = in
	}
	return envelope{rows: []byte(rawRows), byID: byID, total: total}, nil
}
