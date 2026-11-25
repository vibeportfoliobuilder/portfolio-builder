// encoders_guard_test.go - the conformance guard for the encoder convention: two spaces at
// every encoder, and marshalYAML as the one YAML writer. Carried over from the old
// encoders_test.go with its floors lowered to the trimmed tree (pb, 2026-11-06).
//
// Both are properties of the SOURCE, broken by a new call site elsewhere taking the
// package default, which no test inside an already-conforming package would notice.
//
// So this test reads the whole tree from beside the writer it is about, rooting the
// walk two levels up because that is where internal/ is from here.
package docfmt

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// jsonIndentWant is the second argument every json indent call has to pass, as
// it appears in the source.
const jsonIndentWant = `"  "`

// internalDir is internal/ as seen from this package's own directory, which is
// where a go test run has its working directory.
var internalDir = filepath.Join("..", "..", "internal")

// parseInternal parses every non-test .go file under internal/ once. Test files are
// excluded: a fixture may construct four-space yaml to prove a parser reads it.
func parseInternal(t *testing.T) (*token.FileSet, map[string]*ast.File) {
	t.Helper()
	fset := token.NewFileSet()
	files := map[string]*ast.File{}
	err := filepath.WalkDir(internalDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		files[path] = f
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", internalDir, err)
	}
	if len(files) < 40 {
		// A walk that silently found nothing would make every assertion below
		// pass vacuously, which is the one failure mode a guard must not have.
		t.Fatalf("parsed only %d files under internal/ - the walk is not seeing the tree", len(files))
	}
	return fset, files
}

func srcOf(fset *token.FileSet, n ast.Node) string {
	return fset.Position(n.Pos()).String()
}

// litOf renders an argument the way it is written, for the two forms an indent
// argument can take: a literal, or the named constant one package introduced.
func litOf(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.BasicLit:
		return v.Value
	case *ast.Ident:
		return v.Name
	}
	return ""
}

// TestEveryEncoderIndentsTwoSpaces walks the tree for the three shapes an
// indent is set in and insists on two spaces at every one of them.
func TestEveryEncoderIndentsTwoSpaces(t *testing.T) {
	fset, files := parseInternal(t)

	jsonSites, yamlSites := 0, 0
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			switch sel.Sel.Name {
			case "MarshalIndent":
				// json.MarshalIndent(v, prefix, indent)
				if len(call.Args) != 3 {
					return true
				}
				jsonSites++
				if got := litOf(call.Args[1]); got != `""` {
					t.Errorf("%s: MarshalIndent prefix is %s; every stored file starts at column zero, so it must be \"\"", srcOf(fset, call), got)
				}
				if got := litOf(call.Args[2]); got != jsonIndentWant {
					t.Errorf("%s: MarshalIndent indent is %s; want %s (CONTRIBUTE.md: encoders are 2-space)", srcOf(fset, call), got, jsonIndentWant)
				}
			case "SetIndent":
				switch len(call.Args) {
				case 2: // (*json.Encoder).SetIndent(prefix, indent)
					jsonSites++
					if got := litOf(call.Args[0]); got != `""` {
						t.Errorf("%s: SetIndent prefix is %s; want \"\"", srcOf(fset, call), got)
					}
					if got := litOf(call.Args[1]); got != jsonIndentWant {
						t.Errorf("%s: SetIndent indent is %s; want %s (CONTRIBUTE.md: encoders are 2-space)", srcOf(fset, call), got, jsonIndentWant)
					}
				case 1: // (*yaml.Encoder).SetIndent(spaces)
					yamlSites++
					if got := litOf(call.Args[0]); got != "2" && got != "yamlIndent" {
						t.Errorf("%s: yaml SetIndent is %s; want 2 (CONTRIBUTE.md: encoders are 2-space)", srcOf(fset, call), got)
					}
				}
			}
			return true
		})
	}
	// The counts are asserted so a call site DELETED, or renamed out of this test's
	// reach, fails here rather than reading as a clean pass.
	if jsonSites < 1 {
		t.Errorf("found %d json indent call sites; pb prints JSON through at least one, and a guard that finds none is not a guard", jsonSites)
	}
	if yamlSites != 1 {
		t.Errorf("found %d yaml SetIndent call sites; want exactly 1 - see TestOneYAMLWriterAndItIsNotYAMLMarshal", yamlSites)
	}
}

// TestOneYAMLWriterAndItIsNotYAMLMarshal pins the YAML half: yaml.Marshal exposes
// neither SetIndent nor CompactSeqIndent, so it writes four spaces whatever anyone meant.
//
// The rule is therefore about the entry point - one encoder, marshalYAML, configuring
// both options - and not only about the argument.
func TestOneYAMLWriterAndItIsNotYAMLMarshal(t *testing.T) {
	fset, files := parseInternal(t)

	encoders := map[string]string{} // enclosing func -> position
	for path, f := range files {
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			ast.Inspect(fn, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				pkg, ok := sel.X.(*ast.Ident)
				if !ok || pkg.Name != "yaml" {
					return true
				}
				switch sel.Sel.Name {
				case "Marshal":
					t.Errorf("%s (%s): yaml.Marshal takes the package default - four spaces, sequences indented under the key. Use docfmt.MarshalYAML instead",
						srcOf(fset, call), path)
				case "NewEncoder":
					encoders[fn.Name.Name] = srcOf(fset, call)
				}
				return true
			})
		}
	}
	if len(encoders) != 1 || encoders["marshalYAML"] == "" {
		t.Errorf("yaml.NewEncoder is called from %v; want exactly one writer, marshalYAML", encoders)
	}

	// CompactSeqIndent is half the shape and easy to drop in a refactor that keeps
	// SetIndent, so it is asserted by name rather than only by the golden files.
	found := false
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel.Name == "CompactSeqIndent" {
				found = true
			}
			return true
		})
	}
	if !found {
		t.Error("no CompactSeqIndent call in internal/ - without it the sequence dash is indented under its parent key, which is the shape this convention replaced")
	}
}
