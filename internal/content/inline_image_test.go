package content

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/MyungSub0519/gopd/internal/model"
)

// inlinePage builds a one-page document whose content stream is exactly what
// the caller supplies.
func inlinePage(t *testing.T, content string) *DetailedPDF {
	t.Helper()
	return semanticRead(t,
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] /Resources << >> >>`,
		`<< /Type /Page /Parent 2 0 R /Contents 4 0 R >>`,
		semanticStream("", content))
}

// readContent builds the same document as inlinePage but returns the error, so
// that malformed content can be asserted on.
func readContent(t *testing.T, content string) (*DetailedPDF, error) {
	t.Helper()
	data := semanticFixture(
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] /Resources << >> >>`,
		`<< /Type /Page /Parent 2 0 R /Contents 4 0 R >>`,
		semanticStream("", content))
	return Read(bytes.NewReader(data), int64(len(data)))
}

func inlinePayload(t *testing.T, p *DetailedPDF, index int) []byte {
	t.Helper()
	span := p.ImageResources[index].Stream.Encoded
	if span == nil {
		t.Fatal("inline image has no payload span")
	}
	data, err := p.Document.Bytes(*span)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestInlineImageKeepsSurroundingContent(t *testing.T) {
	p := inlinePage(t, "q 10 0 0 10 0 0 cm\nBI /W 2 /H 2 /BPC 1 /CS /G /F /AHx ID\nc0>\nEI Q\n0 0 5 5 re f\n")
	if len(p.Images) != 1 || len(p.ImageResources) != 1 {
		t.Fatalf("images=%d resources=%d, want 1 and 1", len(p.Images), len(p.ImageResources))
	}
	// The path painted after EI is the regression this guards: interpreting
	// used to abort at BI and lose the rest of the page.
	if len(p.Graphics) != 1 {
		t.Fatalf("graphics after EI = %d, want 1", len(p.Graphics))
	}
	if len(p.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", p.Diagnostics)
	}
	if !p.Pages[0].Complete {
		t.Fatal("page should be complete")
	}
	image := p.ImageResources[0]
	if image.Width != 2 || image.Height != 2 || image.BitsPerComponent != 1 || image.ImageMask {
		t.Fatalf("metadata = %+v", image)
	}
	// The placement matrix is the CTM, so the image occupies the 10x10 square
	// the cm operator established.
	if got := p.Images[0].Matrix; got[0] != 10 || got[3] != 10 {
		t.Fatalf("placement matrix = %v", got)
	}
}

func TestInlineImageLengthFromDimensions(t *testing.T) {
	// 2x2 DeviceRGB at 8 bits is exactly 12 bytes. The payload contains a
	// decoy EI that a naive search would stop at.
	raw := []byte{0, 1, 2, 3, ' ', 'E', 'I', ' ', 8, 9, 10, 11}
	p := inlinePage(t, "BI /W 2 /H 2 /BPC 8 /CS /RGB ID "+string(raw)+"\nEI\n")
	if got := inlinePayload(t, p, 0); !bytes.Equal(got, raw) {
		t.Fatalf("payload = %v, want %v", got, raw)
	}
	if got := p.ImageResources[0].Stream.Boundary; got != model.StreamRecovered {
		t.Fatalf("boundary = %v, want model.StreamRecovered", got)
	}
}

func TestInlineImageRowsArePaddedToBytes(t *testing.T) {
	// 3 pixels of 1-bit gray pad to one byte per row, so 3 rows are 3 bytes.
	// Getting the padding wrong would read 9 bits per row and run over.
	raw := []byte{0xA0, 0x40, 0xE0}
	p := inlinePage(t, "BI /W 3 /H 3 /BPC 1 /CS /G ID "+string(raw)+" EI\n")
	if got := inlinePayload(t, p, 0); !bytes.Equal(got, raw) {
		t.Fatalf("payload = %v, want %v", got, raw)
	}
}

func TestInlineImageMaskIsOneBitPerSample(t *testing.T) {
	// An image mask is one bit per sample whatever /BPC claims, so 8x1 is a
	// single byte rather than eight.
	raw := []byte{0x5A}
	p := inlinePage(t, "BI /W 8 /H 1 /IM true ID "+string(raw)+" EI\n")
	if got := inlinePayload(t, p, 0); !bytes.Equal(got, raw) {
		t.Fatalf("payload = %v, want %v", got, raw)
	}
	if !p.ImageResources[0].ImageMask {
		t.Fatal("ImageMask was not recorded")
	}
}

func TestInlineImageExplicitLengthWins(t *testing.T) {
	raw := "\x01\x02EI\x03\x04"
	p := inlinePage(t, fmt.Sprintf("BI /W 6 /H 1 /BPC 8 /CS /G /L %d ID %s EI\n", len(raw), raw))
	if got := inlinePayload(t, p, 0); string(got) != raw {
		t.Fatalf("payload = %q, want %q", got, raw)
	}
	if got := p.ImageResources[0].Stream.Boundary; got != model.StreamFromLength {
		t.Fatalf("boundary = %v, want model.StreamFromLength", got)
	}
}

func TestInlineImageRejectsImplausibleEI(t *testing.T) {
	// A filtered image's length cannot be computed, so EI is searched for.
	// The first candidate is followed by ')', which is not valid syntax, so
	// it must be rejected in favour of the real terminator.
	raw := "\x01 EI ) \x02"
	p := inlinePage(t, "BI /W 4 /H 1 /BPC 8 /CS /G /F /Fl ID "+raw+" EI\n1 0 0 1 2 3 cm\n")
	if got := inlinePayload(t, p, 0); string(got) != raw {
		t.Fatalf("payload = %q, want %q", got, raw)
	}
	// The cm after the real EI must still have been executed.
	ops := p.Pages[0].Operations
	if len(ops) == 0 || ops[len(ops)-1].Operator != "cm" {
		t.Fatalf("operations after EI = %+v", ops)
	}
}

func TestInlineImageAcceptsFullKeyNames(t *testing.T) {
	raw := []byte{1, 2, 3, 4, 5, 6}
	p := inlinePage(t, "BI /Width 2 /Height 1 /BitsPerComponent 8 /ColorSpace /DeviceRGB ID "+string(raw)+" EI\n")
	if got := inlinePayload(t, p, 0); !bytes.Equal(got, raw) {
		t.Fatalf("payload = %v, want %v", got, raw)
	}
	image := p.ImageResources[0]
	if image.Width != 2 || image.Height != 1 || image.BitsPerComponent != 8 {
		t.Fatalf("metadata = %+v", image)
	}
}

func TestInlineImageInsideFormIsSharedAcrossPages(t *testing.T) {
	// One form holding an inline image, drawn by two pages. The bytes are the
	// same, so there is one resource, but two placements.
	p := semanticRead(t,
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 /MediaBox [0 0 100 100] /Resources << /XObject << /Fm 6 0 R >> >> >>`,
		`<< /Type /Page /Parent 2 0 R /Contents 5 0 R >>`,
		`<< /Type /Page /Parent 2 0 R /Contents 5 0 R >>`,
		semanticStream("", `/Fm Do`),
		semanticStream(`/Type /XObject /Subtype /Form /BBox [0 0 10 10]`, "BI /W 1 /H 1 /BPC 8 /CS /G ID \x7f EI"))
	if len(p.ImageResources) != 1 {
		t.Fatalf("resources = %d, want 1", len(p.ImageResources))
	}
	if len(p.Images) != 2 {
		t.Fatalf("placements = %d, want 2", len(p.Images))
	}
	for i, image := range p.Images {
		if image.Source.Page != i {
			t.Fatalf("placement %d attributed to page %d", i, image.Source.Page)
		}
		if len(image.Source.FormPath) != 1 {
			t.Fatalf("placement %d lost its form path", i)
		}
	}
}

func TestInlineImageCRLFAfterID(t *testing.T) {
	raw := []byte{9, 9}
	p := inlinePage(t, "BI /W 2 /H 1 /BPC 8 /CS /G ID\r\n"+string(raw)+" EI\n")
	if got := inlinePayload(t, p, 0); !bytes.Equal(got, raw) {
		t.Fatalf("payload = %v, want %v", got, raw)
	}
}

func TestInlineImageRejectsMalformed(t *testing.T) {
	for name, content := range map[string]string{
		"no EI":              "BI /W 1 /H 1 /BPC 8 /CS /G /F /Fl ID \x01\x02\x03",
		"no ID":              "BI /W 1 /H 1 /BPC 8 /CS /G\n",
		"key is not a name":  "BI 42 /H 1 ID \x01 EI\n",
		"stray keyword":      "BI /W 1 foo /H 1 ID \x01 EI\n",
		"no space after ID":  "BI /W 1 /H 1 /BPC 8 /CS /G /L 1 ID\x01 EI\n",
		"length past end":    "BI /W 1 /H 1 /L 9999 ID \x01 EI\n",
		"length misses EI":   "BI /W 1 /H 1 /L 1 ID \x01\x02\x03 EI\n",
		"dimensions overrun": "BI /W 99 /H 99 /BPC 8 /CS /G ID \x01 EI\n",
		"operands before BI": "1 2 BI /W 1 /H 1 /BPC 8 /CS /G ID \x01 EI\n",
		"bare EI":            "EI\n",
		"bare ID":            "ID\n",
	} {
		if _, err := readContent(t, content); err == nil {
			t.Errorf("%s: expected an error", name)
		} else if !strings.Contains(err.Error(), "inline image") && !strings.Contains(err.Error(), "BI") {
			t.Errorf("%s: unhelpful error %v", name, err)
		}
	}
}

// FuzzContentStream feeds arbitrary bytes to the interpreter as a page's
// content.
//
// It targets the incremental scanner and the inline image path in particular,
// where a malformed BI could otherwise stall: the EI search and the operand
// loop both have to keep moving forward whatever the input, and a payload that
// never terminates must be an error rather than a hang.
func FuzzContentStream(f *testing.F) {
	f.Add([]byte("BI /W 2 /H 2 /BPC 8 /CS /RGB ID 000000000000 EI"))
	f.Add([]byte("BI /W 1 /H 1 /L 4 ID \x00\x00\x00\x00 EI Q"))
	f.Add([]byte("BI ID EI"))
	f.Add([]byte("BI /F /Fl ID \x01 EI ) EI"))
	f.Add([]byte("q 1 0 0 1 0 0 cm BT /F1 12 Tf (hi) Tj ET Q"))
	f.Fuzz(func(t *testing.T, content []byte) {
		if len(content) > 4096 {
			t.Skip()
		}
		p, err := readContent(t, string(content))
		if err != nil {
			return
		}
		// Every placement must point at a resource that exists, and every
		// payload span must be readable; a boundary computed past the end of
		// the stream would show up here.
		for _, image := range p.Images {
			if image.Resource < 0 || image.Resource >= len(p.ImageResources) {
				t.Fatalf("placement references resource %d of %d", image.Resource, len(p.ImageResources))
			}
		}
		for i, resource := range p.ImageResources {
			if resource.Stream.Encoded == nil {
				continue
			}
			if _, err := p.Document.Bytes(*resource.Stream.Encoded); err != nil {
				t.Fatalf("resource %d payload unreadable: %v", i, err)
			}
		}
	})
}
