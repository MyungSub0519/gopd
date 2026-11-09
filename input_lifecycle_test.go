package gopd_test

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/MyungSub0519/gopd"
	"github.com/MyungSub0519/gopd/internal/pdftest"
)

type lifecycleReader struct {
	*bytes.Reader
	closes int
	reads  int
}

func (r *lifecycleReader) Close() error { r.closes++; return nil }

func (r *lifecycleReader) ReadAt(p []byte, off int64) (int, error) {
	r.reads++
	return r.Reader.ReadAt(p, off)
}

func closeResult(t *testing.T, result any) {
	t.Helper()
	closer, ok := result.(io.Closer)
	if !ok {
		t.Fatalf("%T must implement io.Closer", result)
	}
	for range 2 {
		if err := closer.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}
}

func closePublicInputOnCleanup(t *testing.T, result io.Closer) {
	t.Helper()
	t.Cleanup(func() {
		if err := result.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
}

func TestPublicInputLifecycle(t *testing.T) {
	_, data := publicFixture(t)
	for _, tc := range []struct {
		name  string
		level gopd.DetailLevel
	}{
		{"basic", gopd.DetailBasic},
		{"layout", gopd.DetailLayout},
		{"full", gopd.DetailFull},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &lifecycleReader{Reader: bytes.NewReader(data)}
			result, err := gopd.ParseReader(r, int64(len(data)), gopd.ParseOptions{Detail: tc.level})
			if err != nil {
				t.Fatal(err)
			}
			if result.Document != nil {
				header, err := result.Document.Bytes(gopd.Span{Source: result.Document.Structure.File, End: 8})
				if err != nil || string(header) != "%PDF-1.7" {
					t.Fatalf("header = %q, %v", header, err)
				}
			}
			closeResult(t, result)
			if r.closes != 0 {
				t.Fatal("closed caller-owned reader")
			}
			if result.Pages[0].Texts[0].Unicode != "A" {
				t.Fatal("Close invalidated eager output")
			}
			if result.Document != nil {
				_, err := result.Document.Bytes(gopd.Span{Source: result.Document.Structure.File, End: 8})
				if !errors.Is(err, os.ErrClosed) {
					t.Fatalf("source after Close = %v", err)
				}
				closeResult(t, result.Details())
			}
		})
	}
}

func TestPublicLegacyInputLifecycle(t *testing.T) {
	path, data := publicFixture(t)
	entries := []struct {
		name  string
		parse func() (any, *gopd.DetailedPDF, error)
	}{
		{"basic", func() (any, *gopd.DetailedPDF, error) { p, err := gopd.ParsePDF(path); return p, p.Details(), err }},
		{"open", func() (any, *gopd.DetailedPDF, error) { p, err := gopd.Open(path); return p, p, err }},
		{"read", func() (any, *gopd.DetailedPDF, error) {
			p, err := gopd.Read(bytes.NewReader(data), int64(len(data)))
			return p, p, err
		}},
	}
	for _, tc := range entries {
		t.Run(tc.name, func(t *testing.T) {
			result, detail, err := tc.parse()
			if err != nil {
				t.Fatal(err)
			}
			closeResult(t, result)
			if _, err := detail.Document.Bytes(gopd.Span{Source: detail.Document.Structure.File, End: 1}); !errors.Is(err, os.ErrClosed) {
				t.Fatalf("retained source after Close = %v", err)
			}
			if detail.Texts[0].Unicode != "A" {
				t.Fatal("Close invalidated detailed output")
			}
		})
	}
}

func TestPublicPartialResultInputLifecycle(t *testing.T) {
	// The catalog is structurally readable but omits required /Pages
	// (ISO 32000-1:2008, 7.7.2), so semantic interpretation returns a partial result.
	data := pdftest.File([]string{`<< /Type /Catalog >>`}, "")
	r := &lifecycleReader{Reader: bytes.NewReader(data)}
	result, err := gopd.ParseReader(r, int64(len(data)), gopd.ParseOptions{Detail: gopd.DetailFull})
	if err == nil || result == nil || result.Document == nil {
		t.Fatalf("partial result = %+v, %v", result, err)
	}
	if _, err := result.Document.Bytes(gopd.Span{Source: result.Document.Structure.File, End: 8}); err != nil {
		t.Fatal(err)
	}
	closeResult(t, result)
	if r.closes != 0 {
		t.Fatal("closed caller-owned reader on partial-result path")
	}
}

func TestPublicInputBufferValidation(t *testing.T) {
	_, data := publicFixture(t)
	r := &lifecycleReader{Reader: bytes.NewReader(data)}
	options := gopd.ParseOptions{ReadOptions: gopd.ReadOptions{BufferSize: -1}}
	if result, err := gopd.ParseReader(r, int64(len(data)), options); result != nil || err == nil {
		t.Fatalf("negative buffer accepted: %+v, %v", result, err)
	}
	if r.reads != 0 || r.closes != 0 {
		t.Fatal("invalid buffer accessed caller input")
	}
	if result, err := gopd.ParseFile("missing-buffer-validation.pdf", options); result != nil || err == nil || errors.Is(err, os.ErrNotExist) {
		t.Fatalf("negative buffer was not rejected before file access: %+v, %v", result, err)
	}
}

func TestPublicNilResultClose(t *testing.T) {
	var result *gopd.Result
	var basic *gopd.PDF
	var detail *gopd.DetailedPDF
	closeResult(t, result)
	closeResult(t, basic)
	closeResult(t, detail)
}

func TestPublicFileInputLifecycle(t *testing.T) {
	for _, tc := range []struct {
		name  string
		level gopd.DetailLevel
	}{
		{"basic", gopd.DetailBasic},
		{"layout", gopd.DetailLayout},
		{"full", gopd.DetailFull},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path, _ := publicFixture(t)
			result, err := gopd.ParseFile(path, gopd.ParseOptions{Detail: tc.level})
			if result != nil {
				t.Cleanup(func() { closeResult(t, result) })
			}
			if err != nil {
				t.Fatal(err)
			}
			if tc.level == gopd.DetailFull {
				closeResult(t, result)
			} else if result.Document != nil || result.Details() != nil {
				t.Fatal("eager result retained document")
			}
			// Windows refuses removal while a file handle is retained. On all
			// platforms the eager output must remain usable after releasing input.
			if err := os.Remove(path); err != nil {
				t.Fatalf("input file still open: %v", err)
			}
			if result.Pages[0].Texts[0].Unicode != "A" {
				t.Fatal("released input changed output")
			}
		})
	}
}

func TestPublicFileErrorInputLifecycle(t *testing.T) {
	for _, tc := range []struct {
		name    string
		data    []byte
		partial bool
	}{
		{"structure", []byte("%PDF-1.7\ninvalid cross-reference\n"), false},
		{"semantic", pdftest.File([]string{`<< /Type /Catalog >>`}, ""), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "invalid.pdf")
			if err := os.WriteFile(path, tc.data, 0600); err != nil {
				t.Fatal(err)
			}
			result, err := gopd.ParseFile(path, gopd.ParseOptions{Detail: gopd.DetailFull})
			if result != nil {
				t.Cleanup(func() { closeResult(t, result) })
			}
			if err == nil || (result != nil) != tc.partial {
				t.Fatalf("result = %+v, %v", result, err)
			}
			closeResult(t, result)
			if err := os.Remove(path); err != nil {
				t.Fatalf("error path retained input file: %v", err)
			}
		})
	}
}
