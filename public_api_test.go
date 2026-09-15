package gopd_test

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/MyungSub0519/gopd"
)

const publicContent = "0 0 m 10 10 l S BT /F 12 Tf 1 0 0 1 20 30 Tm (A) Tj ET"

// This fixture exercises only public entry points; it needs no private helpers
// or local input PDF. Object offsets and the stream length are computed here.
func publicFixture(t *testing.T) (string, []byte) {
	t.Helper()
	encoded := hex.EncodeToString([]byte(publicContent)) + ">"
	objects := []string{
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`,
		`<< /Type /Page /Parent 2 0 R /Contents 4 0 R /Resources << /Font << /F 5 0 R >> >> >>`,
		fmt.Sprintf("<< /Length %d /Filter /ASCIIHexDecode >>\nstream\n%s\nendstream", len(encoded), encoded),
		`<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding /FirstChar 65 /Widths [600] >>`,
	}
	var file bytes.Buffer
	file.WriteString("%PDF-1.7\n")
	offsets := make([]int, len(objects))
	for i, object := range objects {
		offsets[i] = file.Len()
		fmt.Fprintf(&file, "%d 0 obj\n%s\nendobj\n", i+1, object)
	}
	xref := file.Len()
	fmt.Fprintf(&file, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, offset := range offsets {
		fmt.Fprintf(&file, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&file, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	path := filepath.Join(t.TempDir(), "public-api.pdf")
	if err := os.WriteFile(path, file.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return path, file.Bytes()
}

func checkPublicDetail(t *testing.T, detail *gopd.DetailedPDF) {
	t.Helper()
	if detail == nil || len(detail.Pages) != 1 || len(detail.Texts) != 1 || len(detail.Graphics) != 1 {
		t.Fatal("public detailed parser lost classified content")
	}
	if detail.Texts[0].Unicode != "A" || detail.Texts[0].Matrix[4] != 20 || detail.Texts[0].Matrix[5] != 30 {
		t.Fatal("text decoding or placement changed")
	}
	items := detail.Pages[0].Items
	if len(items) != 2 || items[0].Kind != gopd.ElementGraphic || items[1].Kind != gopd.ElementText {
		t.Fatal("public detailed result lost drawing order")
	}
}

func TestPublicBasicAndDetailedEntryPoints(t *testing.T) {
	path, data := publicFixture(t)
	basic, err := gopd.ParsePDF(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(basic.Texts) != 1 || len(basic.Texts[0]) != 1 || basic.Texts[0][0].Unicode != "A" || len(basic.Graphics) != 1 || len(basic.Graphics[0]) != 1 {
		t.Fatal("basic content must remain grouped by page")
	}
	checkPublicDetail(t, basic.Details())
	for _, open := range []func() (*gopd.DetailedPDF, error){
		func() (*gopd.DetailedPDF, error) { return gopd.Open(path) },
		func() (*gopd.DetailedPDF, error) { return gopd.Read(bytes.NewReader(data), int64(len(data))) },
	} {
		detail, err := open()
		if err != nil {
			t.Fatal(err)
		}
		checkPublicDetail(t, detail)
	}
	var absent *gopd.PDF
	if absent.Details() != nil {
		t.Fatal("nil basic result must have nil details")
	}
}

func TestPublicDocumentObjectsAndStreams(t *testing.T) {
	path, data := publicFixture(t)
	options := gopd.ReadOptions{MaxFileBytes: int64(len(data))}
	for _, parse := range []func() (*gopd.Document, error){
		func() (*gopd.Document, error) { return gopd.ParseFile(path, options) },
		func() (*gopd.Document, error) { return gopd.Parse(bytes.NewReader(data), int64(len(data)), options) },
	} {
		doc, err := parse()
		if err != nil {
			t.Fatal(err)
		}
		catalog, err := doc.Catalog()
		if err != nil {
			t.Fatal(err)
		}
		typeObject, err := catalog.Value.(gopd.Dictionary).Get("Type")
		if err != nil || typeObject.Value != gopd.Name("Catalog") {
			t.Fatalf("catalog lookup = %+v, %v", typeObject, err)
		}
		id := gopd.ObjectID{Number: 4, Generation: 0}
		object, err := doc.Load(id)
		if err != nil || !gopd.IsStream(object.Body) {
			t.Fatalf("stream lookup = %+v, %v", object, err)
		}
		ref := gopd.Reference{ID: id}
		resolved, err := doc.Resolve(ref)
		if err != nil || resolved.Span != object.Body.Span {
			t.Fatalf("reference resolution = %+v, %v", resolved, err)
		}
		resolved, err = doc.ResolveObject(gopd.Object{Value: ref})
		if err != nil || resolved.Span != object.Body.Span {
			t.Fatalf("object resolution = %+v, %v", resolved, err)
		}
		raw, err := doc.RawObject(resolved)
		if err != nil || !bytes.Equal(raw, data[resolved.Span.Start:resolved.Span.End]) {
			t.Fatal("raw source bytes changed")
		}
		source, err := doc.DecodeStream(resolved.Value.(gopd.Stream))
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := doc.Bytes(gopd.Span{Source: source.ID, Start: 0, End: source.Size})
		if err != nil || string(decoded) != publicContent {
			t.Fatalf("decoded source = %q, %v", decoded, err)
		}
		detail, err := gopd.BuildPDF(doc)
		if err != nil {
			t.Fatal(err)
		}
		checkPublicDetail(t, detail)
	}
}

func TestPublicSyntaxValuesAndGeometry(t *testing.T) {
	data := []byte(" [12 /Name true] ")
	tokens, err := gopd.Lex(data, 17, 40)
	if err != nil || tokens[0].Kind != gopd.TokenWhitespace || tokens[0].Span.Source != 17 || tokens[0].Span.Start != 40 || tokens[len(tokens)-1].Kind != gopd.TokenEOF {
		t.Fatalf("public tokens = %+v, %v", tokens, err)
	}
	object, consumed, err := gopd.ParseObject(data, 17, 40)
	if err != nil || consumed != len(data)-1 || object.Span.Start != 41 {
		t.Fatalf("public object = %+v, consumed=%d, err=%v", object, consumed, err)
	}
	n, err := gopd.Int(object.Value.(gopd.Array).Items[0])
	if err != nil || n != 12 {
		t.Fatalf("integer conversion = %d, %v", n, err)
	}
	real, err := gopd.Number(gopd.Object{Value: gopd.Real("3.5")})
	if err != nil || real != 3.5 {
		t.Fatalf("number conversion = %v, %v", real, err)
	}
	dict := gopd.Dictionary{Entries: []gopd.DictionaryEntry{{Key: "Items", Value: object}}}
	if all := dict.GetAll("Items"); len(all) != 1 || all[0].Span != object.Span {
		t.Fatal("dictionary enumeration changed")
	}
	if _, err := dict.Get("Missing"); !errors.Is(err, gopd.ErrMissingKey) {
		t.Fatalf("missing key error = %v", err)
	}
	point := gopd.Point{X: 3, Y: 4}
	if gopd.IdentityMatrix().Transform(point) != point {
		t.Fatal("identity transformation changed")
	}
	translate, scale := gopd.Matrix{1, 0, 0, 1, 10, 20}, gopd.Matrix{2, 0, 0, 3, 0, 0}
	if translate.Mul(scale).Transform(point) != (gopd.Point{X: 16, Y: 32}) {
		t.Fatal("matrix composition order changed")
	}
}

func TestPublicErrorsAndReadLimits(t *testing.T) {
	_, data := publicFixture(t)
	if p, err := gopd.ParsePDF(filepath.Join(t.TempDir(), "missing.pdf")); p != nil || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("file error = %v", err)
	}
	if _, err := gopd.Parse(bytes.NewReader(data), int64(len(data)), gopd.ReadOptions{MaxFileBytes: int64(len(data) - 1)}); err == nil {
		t.Fatal("public read limits were not applied")
	}
	if _, err := gopd.Read(bytes.NewReader([]byte("invalid")), 7); err == nil {
		t.Fatal("invalid PDF accepted")
	}
	if _, err := gopd.Int(gopd.Object{Value: gopd.Name("Name")}); err == nil {
		t.Fatal("integer conversion accepted a name")
	}
}
