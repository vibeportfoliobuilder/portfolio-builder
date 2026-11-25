// cells.go - rendering a row's values the way the file would, which is a true
// statement only while there is one implementation of the rendering.

package docfmt

import "strings"

// JoinCells renders a point's values minus the date, dropping trailing empty
// cells: most NAV rows would otherwise read "106.9144,, -> 106.9151,,".
func JoinCells(cells ...string) string {
	end := len(cells)
	for end > 0 && cells[end-1] == "" {
		end--
	}
	return strings.Join(cells[:end], ",")
}

// Cell reads one column of a row, tolerating a row that stops short.
func Cell(row []string, i int) string {
	if i < 0 || i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}

// TextAt is a row's cell as trimmed, unquoted text: CleanCell over Cell, "" for a
// short row or a -1 column.
func TextAt(row []string, i int) string { return CleanCell(Cell(row, i)) }
