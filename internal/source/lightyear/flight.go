package lightyear

// flight.go - the React payload the explore page ships its rows in. The table is
// not in the markup: the page renders it from this JSON, so this is where it is read.

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// chunkOpen is what precedes each of the payload's chunks. The argument after it
// is a JavaScript string literal, which is also a JSON one.
const chunkOpen = `self.__next_f.push([1,`

// flight concatenates the page's payload chunks into the one document they
// encode. A page has shipped it as a single chunk, and the format permits many.
func flight(page []byte) (string, error) {
	s := string(page)
	var b strings.Builder
	for i := 0; ; {
		j := strings.Index(s[i:], chunkOpen)
		if j < 0 {
			break
		}
		at := i + j + len(chunkOpen)
		end, err := endOfString(s, at)
		if err != nil {
			return "", fmt.Errorf("lightyear: payload chunk at byte %d: %w", at, err)
		}
		var chunk string
		if err := json.Unmarshal([]byte(s[at:end]), &chunk); err != nil {
			return "", fmt.Errorf("lightyear: payload chunk at byte %d: %w", at, err)
		}
		b.WriteString(chunk)
		i = end
	}
	if b.Len() == 0 {
		return "", errors.New("lightyear: no " + chunkOpen + " payload on the page")
	}
	return b.String(), nil
}

// endOfString returns the offset just past the JSON string literal starting at
// from, which is the first quote not spoken for by a backslash.
func endOfString(s string, from int) (int, error) {
	if from >= len(s) || s[from] != '"' {
		return 0, errors.New("expected a string literal")
	}
	for i := from + 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '"':
			return i + 1, nil
		}
	}
	return 0, errors.New("unterminated string literal")
}

// value returns the JSON array or object starting at from. The payload is not a
// JSON document of its own, so a value inside it is taken by matching brackets.
func value(s string, from int) (string, error) {
	if from >= len(s) || (s[from] != '[' && s[from] != '{') {
		return "", errors.New("expected an array or an object")
	}
	depth := 0
	for i := from; i < len(s); i++ {
		switch s[i] {
		case '"':
			end, err := endOfString(s, i)
			if err != nil {
				return "", err
			}
			i = end - 1
		case '[', '{':
			depth++
		case ']', '}':
			if depth--; depth == 0 {
				return s[from : i+1], nil
			}
		}
	}
	return "", errors.New("unterminated array or object")
}

// valueAfter returns the JSON value found at skip bytes into the match of key.
// The two differ where a key is spelled long enough to be unique: see rowsKey.
func valueAfter(s, key string, skip int) (string, error) {
	i := strings.Index(s, key)
	if i < 0 {
		return "", fmt.Errorf("no %s in the page payload", key)
	}
	return value(s, i+skip)
}
