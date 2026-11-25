package store

// writefile_test.go - the observable half of an atomic write: the bytes, the
// mode, the created directory and the absence of scratch files.

import (
	"os"
	"path/filepath"
	"testing"
)

// TestWriteFileAtomicLeavesOnlyTheFinishedFile: a caller reading the directory
// afterwards must see the destination and nothing else, at the mode git records.
func TestWriteFileAtomicLeavesOnlyTheFinishedFile(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "nested", "deeper", "f.json")

	if err := WriteFileAtomic(p, []byte("first\n")); err != nil {
		t.Fatalf("write into a missing directory: %v", err)
	}
	if err := WriteFileAtomic(p, []byte("second\n")); err != nil {
		t.Fatalf("overwrite: %v", err)
	}

	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != "second\n" {
		t.Errorf("content = %q, want the second write", got)
	}
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if fi.Mode().Perm() != 0o644 {
		t.Errorf("mode = %v, want 0644", fi.Mode().Perm())
	}

	ents, err := os.ReadDir(filepath.Dir(p))
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(ents) != 1 || ents[0].Name() != "f.json" {
		names := make([]string, len(ents))
		for i, e := range ents {
			names[i] = e.Name()
		}
		t.Errorf("directory holds %v, want only f.json", names)
	}
}
