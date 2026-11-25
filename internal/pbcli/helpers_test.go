package pbcli

// helpers_test.go - running pb in-process on a temporary data folder, and the help-text check.

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v4"
)

func run(t *testing.T, data string, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	root := NewRoot(&out)
	root.SetArgs(append([]string{"--data", data}, args...))
	root.SetErr(&out)
	if err := root.Execute(); err != nil {
		t.Fatalf("pb %s: %v\n%s", strings.Join(args, " "), err, out.String())
	}
	return out.String()
}

// copyTree copies testdata/<from> into dst.
func copyTree(t *testing.T, from, dst string) {
	t.Helper()
	src := filepath.Join("testdata", from)
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func readYAMLFile(t *testing.T, p string, v any) {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(b, v); err != nil {
		t.Fatal(err)
	}
}

func TestEveryCommandHasHelp(t *testing.T) {
	root := NewRoot(&bytes.Buffer{})
	for _, c := range allCommands(root) {
		if c.Short == "" {
			t.Errorf("%s has no help text", c.CommandPath())
		}
	}
}

func TestTargetsAcceptTickerEqualsISIN(t *testing.T) {
	e := &env{data: t.TempDir(), now: time.Now}
	ts, err := e.targets([]string{"homea=ZZ0000000017", "NETW"}, false, "")
	if err != nil || len(ts) != 2 || ts[0].Ticker != "HOMEA" || ts[0].ID != "ZZ0000000017" || ts[1].ID != "NETW" {
		t.Errorf("%+v %v", ts, err)
	}
	if _, err := e.targets(nil, true, ""); err == nil {
		t.Error("--watchlist with no watchlist.yaml was accepted")
	}
}

func allCommands(c *cobra.Command) []*cobra.Command {
	out := []*cobra.Command{c}
	for _, sub := range c.Commands() {
		out = append(out, allCommands(sub)...)
	}
	return out
}

// screenData is a data folder holding the hand-written screen fixtures (made-up companies).
func screenData(t *testing.T) string {
	t.Helper()
	data := t.TempDir()
	copyTree(t, "screen", filepath.Join(data, "screen"))
	return data
}
