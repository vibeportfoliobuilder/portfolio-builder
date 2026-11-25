// Package dotenv loads .env into the process environment at startup. A variable
// already set in the real environment always wins, so the file is the fallback.
package dotenv

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"strings"
)

// DefaultFile is the file Load reads, relative to the working directory.
const DefaultFile = ".env"

// FileEnv names a different file to read. The escape hatch for a run whose
// credentials are not the ones beside the checkout.
const FileEnv = "PB_ENV_FILE"

// Load reads DefaultFile (or FileEnv's file) and sets every assignment the
// environment does not already carry; a key assigned twice takes its LAST value.
func Load() (int, error) {
	path := strings.TrimSpace(os.Getenv(FileEnv))
	if path == "" {
		path = DefaultFile
	}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		// A clone with no .env is the normal case, not a misconfiguration: every
		// credential here is optional and each absence is reported where it bites.
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("dotenv: reading %s: %w", path, err)
	}
	return apply(b, os.LookupEnv, os.Setenv)
}

// apply is Load's body over injectable environment access, which is the seam
// the tests drive.
func apply(b []byte, lookup func(string) (string, bool), set func(string, string) error) (int, error) {
	// Read the whole file before setting anything, so the LAST assignment of a key wins
	// as in `set -a; . ./.env`, which every live Makefile target uses.
	order := make([]string, 0, 16)
	last := make(map[string]string, 16)
	sc := bufio.NewScanner(bytes.NewReader(b))
	// A cookie header is longer than bufio's 64 KiB default line.
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for line := 1; sc.Scan(); line++ {
		k, v, ok := parse(sc.Text())
		if !ok {
			continue
		}
		if k == "" {
			return 0, fmt.Errorf("dotenv: line %d: an assignment with no name", line)
		}
		if _, seen := last[k]; !seen {
			order = append(order, k)
		}
		last[k] = v
	}
	if err := sc.Err(); err != nil {
		return 0, fmt.Errorf("dotenv: %w", err)
	}
	var n int
	for _, k := range order {
		// The real environment wins over the file: a variable set on the command line and a CI secret
		// must not be replaced by whatever is on disk.
		if _, already := lookup(k); already {
			continue
		}
		if err := set(k, last[k]); err != nil {
			return n, fmt.Errorf("dotenv: setting %s: %w", k, err)
		}
		n++
	}
	return n, nil
}

// parse reads one line. ok is false for a blank line and for a comment, which
// are the two things a file like this is mostly made of.
func parse(line string) (key, value string, ok bool) {
	s := strings.TrimSpace(line)
	if s == "" || strings.HasPrefix(s, "#") {
		return "", "", false
	}
	s = strings.TrimPrefix(s, "export ")
	i := strings.IndexByte(s, '=')
	if i < 0 {
		return "", "", false
	}
	return strings.TrimSpace(s[:i]), unquote(strings.TrimSpace(s[i+1:])), true
}

// unquote takes the quotes off a value: single-quoted is literal (cookie headers
// full of `;` and `=`), only double-quoted carries escapes.
func unquote(v string) string {
	if len(v) < 2 {
		return v
	}
	switch {
	case v[0] == '\'' && v[len(v)-1] == '\'':
		return v[1 : len(v)-1]
	case v[0] == '"' && v[len(v)-1] == '"':
		r := strings.NewReplacer(`\n`, "\n", `\t`, "\t", `\"`, `"`, `\\`, `\`)
		return r.Replace(v[1 : len(v)-1])
	}
	// Unquoted, and a trailing `# ...` is NOT stripped: a token may contain a
	// hash, and guessing wrong truncates a credential silently.
	return v
}
