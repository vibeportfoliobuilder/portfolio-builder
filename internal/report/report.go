// Package report renders the run's page: one committed HTML template with the run's data embedded
// as JSON. Gathering the data is pb's job (internal/pbcli); this package only renders. A connector
// may add pages (RegisterPage): its CSS, navigation button, view and script go into the template's
// slots, and its script reads the data pb put under extra.
package report

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"strings"
	"sync"
)

//go:embed template/index.html
var page []byte

// placeholder is where the data goes: inside <script type="application/json" id="pb-data">.
const placeholder = "__PB_DATA__"

// Page is one extra page: CSS, its navigation button (data-view="<view>"), its view section
// (id="view-<view>") and a script that runs inside the page's closure and adds
// {view: "<view>", render: function () {…}} to EXTRA.
type Page struct {
	Name, CSS, Nav, View, JS string
}

var (
	mu    sync.Mutex
	pages []Page
)

// RegisterPage adds an extra page to every report built after it.
func RegisterPage(p Page) {
	mu.Lock()
	defer mu.Unlock()
	pages = append(pages, p)
}

// slots are where the extra pages go, each held exactly once by the template.
var slots = []struct {
	mark string
	part func(Page) string
}{
	{"/*pb:extra-css*/", func(p Page) string { return p.CSS }},
	{"<!--pb:extra-nav-->", func(p Page) string { return p.Nav }},
	{"<!--pb:extra-views-->", func(p Page) string { return p.View }},
	{"/*pb:extra-js*/", func(p Page) string { return p.JS }},
}

// Render returns the page with data embedded. json.Marshal escapes <, > and &, so the data cannot
// close the script element it sits in.
func Render(data any) ([]byte, error) {
	b, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	if bytes.Count(page, []byte(placeholder)) != 1 {
		return nil, errors.New("report: the template must hold the data placeholder exactly once")
	}
	out := page
	mu.Lock()
	ps := append([]Page(nil), pages...)
	mu.Unlock()
	for _, sl := range slots {
		if bytes.Count(out, []byte(sl.mark)) != 1 {
			return nil, errors.New("report: the template must hold the slot " + sl.mark + " exactly once")
		}
		var parts []string
		for _, p := range ps {
			parts = append(parts, sl.part(p))
		}
		out = bytes.Replace(out, []byte(sl.mark), []byte(strings.Join(parts, "\n")), 1)
	}
	return bytes.Replace(out, []byte(placeholder), b, 1), nil
}
