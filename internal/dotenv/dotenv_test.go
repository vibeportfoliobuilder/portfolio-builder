package dotenv

// dotenv_test.go - the precedence rule is the one worth freezing: a file that
// could override an exported variable would silently undo a variable set on the command line.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// env is a fake environment apply writes into.
type env struct {
	have map[string]string
	set  map[string]string
}

func newEnv(have map[string]string) *env {
	return &env{have: have, set: map[string]string{}}
}

func (e *env) lookup(k string) (string, bool) { v, ok := e.have[k]; return v, ok }
func (e *env) setter(k, v string) error       { e.set[k] = v; return nil }

func TestAnExportedVariableWinsOverTheFile(t *testing.T) {
	e := newEnv(map[string]string{"EXAMPLE_PORT": "7496"})
	n, err := apply([]byte("EXAMPLE_PORT=7497\nFINNHUB_API_KEY=tok\n"), e.lookup, e.setter)
	if err != nil {
		t.Fatal(err)
	}
	if _, overwritten := e.set["EXAMPLE_PORT"]; overwritten {
		t.Error("the file overrode a variable the environment already carried")
	}
	if e.set["FINNHUB_API_KEY"] != "tok" {
		t.Errorf("the unset variable was not loaded: %v", e.set)
	}
	if n != 1 {
		t.Errorf("want one assignment applied, got %d", n)
	}
}

func TestTheShapesAFileLikeThisIsMadeOf(t *testing.T) {
	// Every line here is one .env.example actually contains, cookie included.
	const file = `# a comment

   # an indented comment
export SEC_USER_AGENT='portfolio-builder/1.0 (you@example.com)'
EXAMPLE_COOKIE='x-sess-uuid=0.0.0; web=1; SBID=ab'
EXAMPLE_PORT=7497
QUOTED="a\tb"
BARE=no-quotes#not-a-comment
not an assignment
`
	e := newEnv(nil)
	if _, err := apply([]byte(file), e.lookup, e.setter); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"SEC_USER_AGENT": "portfolio-builder/1.0 (you@example.com)",
		// The one that matters: a cookie is full of `;` and `=` and must survive
		// whole, which is why a single-quoted value is literal.
		"EXAMPLE_COOKIE": "x-sess-uuid=0.0.0; web=1; SBID=ab",
		"EXAMPLE_PORT":   "7497",
		"QUOTED":         "a\tb",
		// A `#` inside an unquoted value is part of it: guessing otherwise
		// truncates a token silently.
		"BARE": "no-quotes#not-a-comment",
	}
	for k, v := range want {
		if e.set[k] != v {
			t.Errorf("%s = %q, want %q", k, e.set[k], v)
		}
	}
	if len(e.set) != len(want) {
		t.Errorf("loaded %d assignments, want %d: %v", len(e.set), len(want), e.set)
	}
}

// A clone with no .env is the normal case, and every credential in it is
// optional, so an absent file is silence rather than a startup failure.
func TestAMissingFileIsNotAnError(t *testing.T) {
	t.Setenv(FileEnv, filepath.Join(t.TempDir(), "nothing-here"))
	n, err := Load()
	if err != nil || n != 0 {
		t.Fatalf("Load over a missing file = %d, %v", n, err)
	}
}

func TestLoadReadsTheFileNamedByTheEnvironment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "creds")
	if err := os.WriteFile(path, []byte("PB_DOTENV_PROBE='it worked'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(FileEnv, path)
	t.Setenv("PB_DOTENV_PROBE", "")
	os.Unsetenv("PB_DOTENV_PROBE")
	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("PB_DOTENV_PROBE"); got != "it worked" {
		t.Errorf("PB_DOTENV_PROBE = %q", got)
	}
	os.Unsetenv("PB_DOTENV_PROBE")
}

// A cookie header is longer than bufio's default line, which is the scanner
// limit this package raises.
func TestALineLongerThanTheScannerDefault(t *testing.T) {
	long := strings.Repeat("a", 200000)
	e := newEnv(nil)
	if _, err := apply([]byte("BIG="+long+"\n"), e.lookup, e.setter); err != nil {
		t.Fatal(err)
	}
	if e.set["BIG"] != long {
		t.Errorf("a %d-byte value did not survive, got %d bytes", len(long), len(e.set["BIG"]))
	}
}

// A key assigned twice takes its last value, because `set -a; . ./.env` does
// and every live target in the Makefile sources this file into a shell. First
// wins would make the binary and `make` read different credentials from one file.
func TestALaterAssignmentWinsLikeTheShellDoes(t *testing.T) {
	const file = "FINNHUB_API_KEY='first'\nTAVILY_API_KEY='t'\nFINNHUB_API_KEY='second'\n"
	e := newEnv(nil)
	n, err := apply([]byte(file), e.lookup, e.setter)
	if err != nil {
		t.Fatal(err)
	}
	if e.set["FINNHUB_API_KEY"] != "second" {
		t.Errorf("FINNHUB_API_KEY = %q, want the last assignment %q", e.set["FINNHUB_API_KEY"], "second")
	}
	// Two keys, not three: a repeated key is one variable.
	if n != 2 {
		t.Errorf("applied %d assignments, want 2", n)
	}
}

// And the real environment still beats the file, however many times the file
// says otherwise.
func TestADuplicatedKeyStillLosesToTheEnvironment(t *testing.T) {
	e := newEnv(map[string]string{"FINNHUB_API_KEY": "exported"})
	if _, err := apply([]byte("FINNHUB_API_KEY='a'\nFINNHUB_API_KEY='b'\n"), e.lookup, e.setter); err != nil {
		t.Fatal(err)
	}
	if len(e.set) != 0 {
		t.Errorf("the file overrode an exported variable: %v", e.set)
	}
}
