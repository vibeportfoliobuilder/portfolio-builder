package pbcli

// calltext_test.go - pb calltext says why it wrote nothing: no deck saved, or no pdftotext.

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCallTextWritesNothingWithoutADeckOrPDFToText(t *testing.T) {
	data := t.TempDir()
	for _, tk := range []string{"TSTA", "TSTB"} {
		if err := os.MkdirAll(filepath.Join(data, "stocks", tk), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(data, "stocks", "TSTB", callPDF), []byte("%PDF-1.4\n"), 0o644)
	t.Setenv("PATH", t.TempDir())
	var out bytes.Buffer
	e := &env{data: data, out: &out, now: func() time.Time { return time.Date(2030, 3, 10, 0, 0, 0, 0, time.UTC) }}
	c := callTextCmd(e)
	c.SetArgs([]string{"TSTA", "TSTB"})
	c.SetOut(&bytes.Buffer{})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	var v struct {
		Results []callTextResult `json:"results"`
	}
	if err := json.Unmarshal(out.Bytes(), &v); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if len(v.Results) != 2 || v.Results[0].Status != "no earnings_call.pdf" || v.Results[1].Status != "pdftotext not found" {
		t.Errorf("%+v", v.Results)
	}
	for _, tk := range []string{"TSTA", "TSTB"} {
		if _, err := os.Stat(filepath.Join(data, "stocks", tk, callText)); err == nil {
			t.Errorf("%s: a text file was written", tk)
		}
	}
}
