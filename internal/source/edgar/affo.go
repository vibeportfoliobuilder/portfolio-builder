package edgar

// affo.go - adjusted funds from operations, tagged nowhere: a non-GAAP figure read from the MD&A.
// THE SCALE IS NOT IN THE TABLE, so every reading is checked against the tagged FFO.

import (
	"regexp"
	"strconv"
	"strings"
)

// The label of the line the figure sits on, and the words marking a line that is
// not the total: a per-share figure, a share count, or an addend.
var (
	affoLabelRE = regexp.MustCompile(`(?i)^\s*(?:diluted\s+)?(?:adjusted funds from operations|affo)\b[^0-9]{0,80}$`)
	notTheTotal = regexp.MustCompile(`(?i)per\s+(?:common\s+)?share|per\s+diluted|weighted average|shares|allocable|after distributions|adjustment|margin|payout|growth`)
	numberRE    = regexp.MustCompile(`^\(?\$?\s*([\d,]+(?:\.\d+)?)\s*\)?$`)
	// A trailing footnote marker is part of the label, not a figure: Farmland
	// Partners heads the total "AFFO (1)".
	footnoteRE = regexp.MustCompile(`\s*\(\d{1,2}\)\s*$`)
)

// affoScales are the three the filings print in, largest first: a table headed
// "in millions" and one headed "in thousands" carry the same figure differently.
var affoScales = []float64{1e6, 1e3, 1}

// The band a reading must land in against the tagged FFO: AFFO moves it by tens of
// percent, never by a factor of two.
const (
	minAFFOOfFFO = 0.4
	maxAFFOOfFFO = 1.6
)

// maxAFFOLookahead is how far past the label the figure may sit: a rendered
// table puts the currency sign, and sometimes a blank cell, on its own line.
const maxAFFOLookahead = 4

// ReadAFFO finds AFFO in a filing's reconciliation, in the filer's dollars; ffo decides the
// scale, and without it there is no reading, as a possibly 1000x-wrong figure is worse than none.
func ReadAFFO(text string, ffo float64) (affo float64, heading string, found bool) {
	if ffo <= 0 {
		return 0, "", false
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		label := footnoteRE.ReplaceAllString(line, "")
		if !affoLabelRE.MatchString(label) || notTheTotal.MatchString(label) {
			continue
		}
		raw, ok := firstNumberAfter(lines, i)
		if !ok {
			continue
		}
		for _, scale := range affoScales {
			v := raw * scale
			if v >= ffo*minAFFOOfFFO && v <= ffo*maxAFFOOfFFO {
				return v, strings.TrimSpace(line), true
			}
		}
	}
	return 0, "", false
}

// firstNumberAfter reads the first numeric cell following a label, skipping the
// currency sign and the empty cells a rendered table leaves between them.
func firstNumberAfter(lines []string, at int) (float64, bool) {
	for j := at + 1; j < len(lines) && j <= at+maxAFFOLookahead; j++ {
		s := strings.TrimSpace(lines[j])
		if s == "" || s == "$" || s == "—" || s == "-" {
			continue
		}
		m := numberRE.FindStringSubmatch(s)
		if m == nil {
			return 0, false
		}
		v, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", ""), 64)
		if err != nil {
			return 0, false
		}
		return v, true
	}
	return 0, false
}
