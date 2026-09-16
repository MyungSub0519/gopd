package syntax

import (
	"bytes"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/MyungSub0519/gopd/internal/pdfmodel"
)

func TestLexPreservesTriviaAndSourceSpans(t *testing.T) {
	data := []byte(" \x00%hi\r\n/A#20B 01 +.5 (x) <F> [] <<>> q true")
	want := []struct {
		kind pdfmodel.TokenKind
		raw  string
	}{
		{pdfmodel.TokenWhitespace, " \x00"}, {pdfmodel.TokenComment, "%hi"}, {pdfmodel.TokenWhitespace, "\r\n"},
		{pdfmodel.TokenName, "/A#20B"}, {pdfmodel.TokenWhitespace, " "}, {pdfmodel.TokenInteger, "01"},
		{pdfmodel.TokenWhitespace, " "}, {pdfmodel.TokenReal, "+.5"}, {pdfmodel.TokenWhitespace, " "},
		{pdfmodel.TokenLiteralString, "(x)"}, {pdfmodel.TokenWhitespace, " "}, {pdfmodel.TokenHexString, "<F>"},
		{pdfmodel.TokenWhitespace, " "}, {pdfmodel.TokenArrayOpen, "["}, {pdfmodel.TokenArrayClose, "]"},
		{pdfmodel.TokenWhitespace, " "}, {pdfmodel.TokenDictOpen, "<<"}, {pdfmodel.TokenDictClose, ">>"},
		{pdfmodel.TokenWhitespace, " "}, {pdfmodel.TokenKeyword, "q"}, {pdfmodel.TokenWhitespace, " "},
		{pdfmodel.TokenKeyword, "true"}, {pdfmodel.TokenEOF, ""},
	}
	tokens, err := Lex(data, 7, 123)
	if err != nil {
		t.Fatal(err)
	}
	if len(tokens) != len(want) {
		t.Fatalf("token count = %d, want %d: %+v", len(tokens), len(want), tokens)
	}
	next := int64(123)
	for i, token := range tokens {
		if token.Kind != want[i].kind || token.Span.Source != 7 || token.Span.Start != next {
			t.Fatalf("token %d = %+v, want kind %d source 7 start %d", i, token, want[i].kind, next)
		}
		if token.Span.End < token.Span.Start || token.Span.End > 123+int64(len(data)) {
			t.Fatalf("invalid token span: %+v", token)
		}
		if got := string(data[token.Span.Start-123 : token.Span.End-123]); got != want[i].raw {
			t.Fatalf("token %d raw = %q, want %q", i, got, want[i].raw)
		}
		next = token.Span.End
	}
	if next != 123+int64(len(data)) {
		t.Fatal("lexer did not account for all source bytes")
	}
}

func TestParseObjectPrimitiveValues(t *testing.T) {
	tests := []struct {
		input string
		want  pdfmodel.Value
	}{
		{"+00012", pdfmodel.Integer("+00012")}, {"-0", pdfmodel.Integer("-0")}, {".50", pdfmodel.Real(".50")},
		{"+1.", pdfmodel.Real("+1.")}, {"-.0020", pdfmodel.Real("-.0020")}, {"true", pdfmodel.Boolean(true)},
		{"false", pdfmodel.Boolean(false)}, {"null", pdfmodel.Null{}}, {"/A#20B#ff", pdfmodel.Name("A B\xff")},
		{"/", pdfmodel.Name("")}, {"()", pdfmodel.PDFString{Form: pdfmodel.StringLiteral, Bytes: []byte{}}},
		{"(a(b)c)", pdfmodel.PDFString{Form: pdfmodel.StringLiteral, Bytes: []byte("a(b)c")}},
		{`(a\101\12\1\777\(\)\\\n\r\t\b\f\z)`, pdfmodel.PDFString{Form: pdfmodel.StringLiteral, Bytes: []byte("aA\n\x01\xff()\\\n\r\t\b\fz")}},
		{"(a\rb\r\nc\nd)", pdfmodel.PDFString{Form: pdfmodel.StringLiteral, Bytes: []byte("a\nb\nc\nd")}},
		{"(a\\\r\nb\\\rc\\\nd)", pdfmodel.PDFString{Form: pdfmodel.StringLiteral, Bytes: []byte("abcd")}},
		{"<A b C\x00>", pdfmodel.PDFString{Form: pdfmodel.StringHex, Bytes: []byte{0xab, 0xc0}}},
		{"<>", pdfmodel.PDFString{Form: pdfmodel.StringHex, Bytes: []byte{}}},
		{"12 0 R", pdfmodel.Reference{ID: pdfmodel.ObjectID{Number: 12}}},
		{"4294967295 65535 R", pdfmodel.Reference{ID: pdfmodel.ObjectID{Number: 4294967295, Generation: 65535}}},
		{"12 % ref\r\n 34 R", pdfmodel.Reference{ID: pdfmodel.ObjectID{Number: 12, Generation: 34}}},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			got, n, err := ParseObject([]byte(test.input), 2, 100)
			if err != nil {
				t.Fatal(err)
			}
			if n != len(test.input) || got.Span != (pdfmodel.Span{Source: 2, Start: 100, End: 100 + int64(len(test.input))}) {
				t.Fatalf("consumed = %d, span = %+v", n, got.Span)
			}
			if !reflect.DeepEqual(got.Value, test.want) {
				t.Fatalf("value = %#v, want %#v", got.Value, test.want)
			}
		})
	}
}

func TestParseObjectPreservesDictionaryEntriesAndNestedSpans(t *testing.T) {
	input := []byte(" \n<< /A (a\\101) /A [12 0 R false null] >> stream")
	got, n, err := ParseObject(input, 4, 100)
	if err != nil {
		t.Fatal(err)
	}
	want := pdfmodel.Object{Span: pdfmodel.Span{Source: 4, Start: 102, End: 141}, Value: pdfmodel.Dictionary{Entries: []pdfmodel.DictionaryEntry{
		{Key: "A", KeySpan: pdfmodel.Span{Source: 4, Start: 105, End: 107}, Value: pdfmodel.Object{Span: pdfmodel.Span{Source: 4, Start: 108, End: 115}, Value: pdfmodel.PDFString{Form: pdfmodel.StringLiteral, Bytes: []byte("aA")}}},
		{Key: "A", KeySpan: pdfmodel.Span{Source: 4, Start: 116, End: 118}, Value: pdfmodel.Object{Span: pdfmodel.Span{Source: 4, Start: 119, End: 138}, Value: pdfmodel.Array{Items: []pdfmodel.Object{
			{Span: pdfmodel.Span{Source: 4, Start: 120, End: 126}, Value: pdfmodel.Reference{ID: pdfmodel.ObjectID{Number: 12, Generation: 0}}},
			{Span: pdfmodel.Span{Source: 4, Start: 127, End: 132}, Value: pdfmodel.Boolean(false)},
			{Span: pdfmodel.Span{Source: 4, Start: 133, End: 137}, Value: pdfmodel.Null{}},
		}}}},
	}}}
	if n != 41 || !reflect.DeepEqual(got, want) {
		t.Fatalf("got consumed=%d object=%#v\nwant consumed=41 object=%#v", n, got, want)
	}
}

func TestParseObjectStopsBeforeTrailingTriviaAndOtherObjects(t *testing.T) {
	for _, input := range []string{" 12 34", " 12 q", " 12 (unterminated", " 12 % trailing", " 12 <invalid", " 12"} {
		got, n, err := ParseObject([]byte(input), 1, 9)
		if err != nil || n != 3 || got.Value != pdfmodel.Integer("12") || got.Span != (pdfmodel.Span{Source: 1, Start: 10, End: 12}) {
			t.Fatalf("%q: object=%+v consumed=%d error=%v", input, got, n, err)
		}
	}
}

func TestSyntaxRejectsMalformedObjects(t *testing.T) {
	for _, input := range []string{
		"", " \x00%only comment", "q", "truex", "obj", "R", "1e3", "+", ".", "--1", "1.2.3",
		"[", "[1", "[1 >>", "<<", "<< /A >>", "<< 1 2 >>", "<< /A 1 ]", "]", ">>",
		"(unterminated", "(escaped\\)", "(trailing\\", "<123", "<1x>", "/A#", "/A#0", "/A#GG", "/A#00",
		"}", "{", ">", ")", "0 0 R", "-1 0 R", "1 -1 R", "4294967296 0 R", "1 65536 R",
	} {
		t.Run(input, func(t *testing.T) {
			_, _, err := ParseObject([]byte(input), 3, 80)
			if err == nil {
				t.Fatalf("accepted malformed object %q", input)
			}
			if !strings.Contains(err.Error(), "80") && !strings.Contains(err.Error(), "byte") {
				t.Fatalf("error does not identify byte position: %v", err)
			}
		})
	}
}

func TestLexRejectsTruncatedAndInvalidTokens(t *testing.T) {
	for _, input := range []string{"(abc", "(abc\\", "<ab", "<ax>", "/a#", "/a#Z0", "/a#00", ">", ")", "{"} {
		if _, err := Lex([]byte(input), 1, 0); err == nil {
			t.Errorf("accepted invalid token %q", input)
		}
	}
}

func TestSyntaxRejectsInvalidOffsetsAndEnforcesLimits(t *testing.T) {
	for _, offset := range []int64{-1, math.MaxInt64} {
		if _, err := Lex([]byte("[]"), 1, offset); err == nil {
			t.Errorf("Lex accepted offset %d", offset)
		}
		if _, _, err := ParseObject([]byte("[]"), 1, offset); err == nil {
			t.Errorf("ParseObject accepted offset %d", offset)
		}
	}
	if _, _, err := ParseObject([]byte(strings.Repeat("[", 300)+strings.Repeat("]", 300)), 1, 0); err == nil {
		t.Fatal("accepted excessive nesting")
	}
	if _, _, err := ParseObjectWithLimits([]byte("[[[]]]"), 1, 0, pdfmodel.Limits{MaxDepth: 2}); err == nil {
		t.Fatal("ignored custom depth limit")
	}
	if _, _, err := ParseObjectWithLimits([]byte("(12345)"), 1, 0, pdfmodel.Limits{MaxTokenBytes: 6}); err == nil {
		t.Fatal("ignored custom token byte limit")
	}
	if _, _, err := ParseObjectWithLimits([]byte("true"), 1, 0, pdfmodel.Limits{MaxDepth: -1}); err == nil {
		t.Fatal("accepted negative depth limit")
	}
	if _, _, err := ParseObjectWithLimits([]byte("true"), 1, 0, pdfmodel.Limits{MaxTokenBytes: -1}); err == nil {
		t.Fatal("accepted negative token limit")
	}
	if _, _, err := ParseObjectWithLimits([]byte("true"), 1, 0, pdfmodel.Limits{MaxDepth: 1_000_000}); err == nil {
		t.Fatal("accepted a nesting limit that can exhaust the Go call stack")
	}
	oversized := bytes.Repeat([]byte("a"), 16<<20+1)
	if _, err := Lex(oversized, 1, 0); err == nil {
		t.Fatal("accepted oversized lexical token")
	}
}

func FuzzParseObject(f *testing.F) {
	for _, seed := range []string{"", "[1 -2 .3 true null]", "<< /A (nested(x)\\101) /Ref 1 0 R >>", "<F>", "/A#20B", "[", "(\\\r\n)"} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		object, consumed, err := ParseObject(data, 5, 17)
		if consumed < 0 || consumed > len(data) {
			t.Fatalf("invalid consumed bytes: %d/%d", consumed, len(data))
		}
		if err == nil {
			if object.Value == nil || consumed == 0 || object.Span.Source != 5 || object.Span.Start < 17 || object.Span.End != 17+int64(consumed) {
				t.Fatalf("invalid successful parse: %+v consumed=%d", object, consumed)
			}
		}
	})
}

func FuzzLex(f *testing.F) {
	for _, seed := range []string{"", "\x00%comment\r\n/a#20 (x\\123) <F>", "[<< /A true >>]", "(\r\n)", "/bad#"} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		tokens, err := Lex(data, 9, 3)
		next := int64(3)
		for _, token := range tokens {
			if token.Span.Source != 9 || token.Span.Start != next || token.Span.End < next || token.Span.End > 3+int64(len(data)) {
				t.Fatalf("invalid token span: %+v", token)
			}
			next = token.Span.End
		}
		if err == nil && (len(tokens) == 0 || tokens[len(tokens)-1].Kind != pdfmodel.TokenEOF || next != 3+int64(len(data))) {
			t.Fatalf("successful lexer omitted EOF or bytes: %+v", tokens)
		}
	})
}
