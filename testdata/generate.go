//go:build ignore

// Run from the repository root: go run ./testdata/generate.go [-check]
// All document content is invented here; no external files are read as input.
package main

import (
	"bytes"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/MyungSub0519/gopd/internal/common/pdftest"
)

func main() {
	check := flag.Bool("check", false, "verify the checked-in PDF matches the generator")
	flag.Parse()
	const path = "testdata/synthetic.pdf"
	data := fixture()
	if *check {
		stored, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if !bytes.Equal(stored, data) {
			fmt.Fprintln(os.Stderr, "synthetic.pdf differs; run go run ./testdata/generate.go")
			os.Exit(1)
		}
		return
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func fixture() []byte {
	// Fixed ASCII bytes and object order make output independent of clocks,
	// random seeds, machine paths, installed fonts and platform line endings.
	pageOne := `BT /F 20 Tf 1 0 0 1 50 720 Tm (GoPD synthetic fixture) Tj ET
BT /F 12 Tf 1 0 0 1 50 680 Tm (Alpha beta 123) Tj ET
0.1 0.4 0.7 RG 2 w 50 600 180 40 re S
50 580 m 230 580 l S
q 60 0 0 60 50 480 cm /Im Do Q
`
	pageTwo := `BT /F 16 Tf 1 0 0 1 50 720 Tm (Page two: shared resources) Tj ET
q 1 0 0 1 50 500 cm /Fm Do Q
q 60 0 0 60 50 400 cm /Im Do Q
`
	form := `BT /F 12 Tf 1 0 0 1 10 90 Tm (Reusable form) Tj ET
0.7 0.2 0.1 RG 2 w 10 30 m 80 30 l 45 60 l h S
`
	// Four invented RGB pixels: red, green, blue, white.
	pixels := []byte{255, 0, 0, 0, 255, 0, 0, 0, 255, 255, 255, 255}
	return pdftest.File([]string{
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 /MediaBox [0 0 612 792] /Resources << /Font << /F 5 0 R >> /XObject << /Im 8 0 R /Fm 9 0 R >> >> >>`,
		`<< /Type /Page /Parent 2 0 R /Contents 6 0 R >>`,
		`<< /Type /Page /Parent 2 0 R /Contents 7 0 R >>`,
		`<< /Type /Font /Subtype /Type1 /BaseFont /Courier /Encoding /WinAnsiEncoding /FirstChar 32 /LastChar 126 /Widths [` + strings.Repeat("600 ", 95) + `] >>`,
		pdftest.Stream("", pageOne),
		pdftest.Stream("", pageTwo),
		pdftest.Stream(`/Type /XObject /Subtype /Image /Width 2 /Height 2 /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /ASCIIHexDecode`, hex.EncodeToString(pixels)+">"),
		pdftest.Stream(`/Type /XObject /Subtype /Form /BBox [0 0 300 120] /Resources << /Font << /F 5 0 R >> >>`, form),
	}, "")
}
