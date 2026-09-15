package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPDFParseReturnsBasicObject(t *testing.T) {
	path := filepath.Join("..", "..", "테스트PDF.pdf")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("user fixture is not present")
	}
	doc, err := pdfparse(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Texts) != 3 || len(doc.Graphics) != 3 || doc.Details() == nil {
		t.Fatal("pdfparse did not return classified basic content")
	}
	wantTexts := []int{58, 39, 7}
	wantGraphics := []int{1052, 1571, 16}
	for page := range doc.Texts {
		if len(doc.Texts[page]) != wantTexts[page] || len(doc.Graphics[page]) != wantGraphics[page] {
			t.Fatalf("page %d counts changed", page)
		}
	}
	if doc.Texts[0][0].Font == nil {
		t.Fatal("basic font information was lost")
	}
}

func TestPDFParsePropagatesFileError(t *testing.T) {
	doc, err := pdfparse(filepath.Join(t.TempDir(), "missing.pdf"))
	if doc != nil || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("result=%v error=%v", doc, err)
	}
}

func TestInspectUserPDF(t *testing.T) {
	path := filepath.Join("..", "..", "테스트PDF.pdf")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("user fixture is not present")
	}
	var out, stderr bytes.Buffer
	code := run([]string{"-json", path}, &out, &stderr)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	var result struct {
		Pages    int `json:"pages"`
		Texts    int `json:"texts"`
		Graphics int `json:"graphics"`
		Images   int `json:"images"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Pages != 3 || result.Texts == 0 || result.Graphics == 0 || result.Images == 0 {
		t.Fatalf("missing classified elements: %+v", result)
	}
}

func TestUnreadablePDFIsAnError(t *testing.T) {
	var out, stderr bytes.Buffer
	if run([]string{filepath.Join(t.TempDir(), "absent.pdf")}, &out, &stderr) == 0 {
		t.Fatal("missing file reported success")
	}
	if out.Len() != 0 || stderr.Len() == 0 {
		t.Fatal("failure must go to stderr")
	}
}
