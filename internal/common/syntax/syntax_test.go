package syntax

import (
	"bytes"
	"errors"
	"io"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/MyungSub0519/gopd/internal/common/pdfmodel"
)

func TestContentScannerRejectsInvalidConfiguration(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name   string
		offset int64
		limits pdfmodel.Limits
	}{
		{name: "negative offset", offset: -1},
		{name: "overflowing range", offset: math.MaxInt64},
		{name: "negative depth", limits: pdfmodel.Limits{MaxDepth: -1}},
		{name: "unsafe depth", limits: pdfmodel.Limits{MaxDepth: 4097}},
		{name: "negative token size", limits: pdfmodel.Limits{MaxTokenBytes: -1}},
		{name: "negative values", limits: pdfmodel.Limits{MaxValues: -1}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewContentScanner([]byte("q"), 7, tt.offset, tt.limits)
			if err == nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
}

func TestContentScannerMixedOperandsAndOperators(t *testing.T) {
	t.Parallel()
	data := []byte(" %c\nq /F#31 12 Tf true false null 1.5 2 3 m (a\\n) <F> Tj Q")
	scanner, err := NewContentScanner(data, 7, 100, pdfmodel.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name     string
		value    pdfmodel.Value
		operator bool
		start    int64
		end      int64
	}{
		{name: "save", value: pdfmodel.Name("q"), operator: true, start: 104, end: 105},
		{name: "font name", value: pdfmodel.Name("F1"), start: 106, end: 111},
		{name: "font size", value: pdfmodel.Integer("12"), start: 112, end: 114},
		{name: "font", value: pdfmodel.Name("Tf"), operator: true, start: 115, end: 117},
		{name: "true", value: pdfmodel.Boolean(true), start: 118, end: 122},
		{name: "false", value: pdfmodel.Boolean(false), start: 123, end: 128},
		{name: "null", value: pdfmodel.Null{}, start: 129, end: 133},
		{name: "real", value: pdfmodel.Real("1.5"), start: 134, end: 137},
		{name: "first integer", value: pdfmodel.Integer("2"), start: 138, end: 139},
		{name: "second integer", value: pdfmodel.Integer("3"), start: 140, end: 141},
		{name: "move", value: pdfmodel.Name("m"), operator: true, start: 142, end: 143},
		{name: "literal", value: pdfmodel.PDFString{Form: pdfmodel.StringLiteral, Bytes: []byte("a\n")}, start: 144, end: 149},
		{name: "hex", value: pdfmodel.PDFString{Form: pdfmodel.StringHex, Bytes: []byte{0xf0}}, start: 150, end: 153},
		{name: "show", value: pdfmodel.Name("Tj"), operator: true, start: 154, end: 156},
		{name: "restore", value: pdfmodel.Name("Q"), operator: true, start: 157, end: 158},
	} {
		object, operator, used, err := scanner.Next(100)
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		wantUsed := 1
		if tt.operator {
			wantUsed = 0
		}
		want := pdfmodel.Object{Span: pdfmodel.Span{Source: 7, Start: tt.start, End: tt.end}, Value: tt.value}
		if !reflect.DeepEqual(object, want) || operator != tt.operator || used != wantUsed {
			t.Fatalf("%s: got object=%#v operator=%v values=%d; want object=%#v operator=%v values=%d",
				tt.name, object, operator, used, want, tt.operator, wantUsed)
		}
	}
	for range 2 {
		_, operator, used, err := scanner.Next(0)
		if !errors.Is(err, io.EOF) || operator || used != 0 {
			t.Fatalf("end: operator=%v values=%d err=%v", operator, used, err)
		}
	}
}

func TestContentScannerNestedValuesAndSpans(t *testing.T) {
	t.Parallel()
	scanner, err := NewContentScanner([]byte("[1 << /A [true] /R 12 0 R >>] TJ"), 4, 50, pdfmodel.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	object, operator, used, err := scanner.Next(6)
	if err != nil || operator || used != 6 {
		t.Fatalf("operator=%v values=%d err=%v", operator, used, err)
	}
	want := pdfmodel.Object{
		Span: pdfmodel.Span{Source: 4, Start: 50, End: 79},
		Value: pdfmodel.Array{Items: []pdfmodel.Object{
			{Span: pdfmodel.Span{Source: 4, Start: 51, End: 52}, Value: pdfmodel.Integer("1")},
			{
				Span: pdfmodel.Span{Source: 4, Start: 53, End: 78},
				Value: pdfmodel.Dictionary{Entries: []pdfmodel.DictionaryEntry{
					{
						Key: "A", KeySpan: pdfmodel.Span{Source: 4, Start: 56, End: 58},
						Value: pdfmodel.Object{
							Span: pdfmodel.Span{Source: 4, Start: 59, End: 65},
							Value: pdfmodel.Array{Items: []pdfmodel.Object{
								{Span: pdfmodel.Span{Source: 4, Start: 60, End: 64}, Value: pdfmodel.Boolean(true)},
							}},
						},
					},
					{
						Key: "R", KeySpan: pdfmodel.Span{Source: 4, Start: 66, End: 68},
						Value: pdfmodel.Object{
							Span:  pdfmodel.Span{Source: 4, Start: 69, End: 75},
							Value: pdfmodel.Reference{ID: pdfmodel.ObjectID{Number: 12}},
						},
					},
				}},
			},
		}},
	}
	if !reflect.DeepEqual(object, want) {
		t.Fatalf("got %#v; want %#v", object, want)
	}
	object, operator, used, err = scanner.Next(0)
	if err != nil || !operator || used != 0 || object.Value != pdfmodel.Name("TJ") {
		t.Fatalf("operator after exhausted value budget: object=%#v operator=%v values=%d err=%v",
			object, operator, used, err)
	}
}

func TestContentScannerLimits(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name      string
		input     string
		limits    pdfmodel.Limits
		remaining int
		wantUsed  int
	}{
		{name: "empty value budget", input: "null", remaining: 0},
		{name: "nested value budget", input: "[1 [2]]", remaining: 3, wantUsed: 3},
		{name: "configured values", input: "[1 2]", limits: pdfmodel.Limits{MaxValues: 2}, remaining: 10, wantUsed: 2},
		{name: "configured depth", input: "[[0]]", limits: pdfmodel.Limits{MaxDepth: 1}, remaining: 10, wantUsed: 2},
		{name: "configured operand token", input: "(abc)", limits: pdfmodel.Limits{MaxTokenBytes: 4}, remaining: 10},
		{name: "configured operator token", input: "unknown", limits: pdfmodel.Limits{MaxTokenBytes: 4}, remaining: 10},
		{name: "configured trivia token", input: "%comment\nq", limits: pdfmodel.Limits{MaxTokenBytes: 4}, remaining: 10},
	} {
		t.Run(tt.name, func(t *testing.T) {
			scanner, err := NewContentScanner([]byte(tt.input), 3, 80, tt.limits)
			if err != nil {
				t.Fatal(err)
			}
			_, _, used, err := scanner.Next(tt.remaining)
			if !errors.Is(err, pdfmodel.ErrLimit) || used != tt.wantUsed {
				t.Fatalf("values=%d err=%v; want values=%d and ErrLimit", used, err, tt.wantUsed)
			}
		})
	}
}

func TestContentScannerValueBudgetAcrossCalls(t *testing.T) {
	t.Parallel()
	scanner, err := NewContentScanner([]byte("1 q [2] 3"), 1, 0, pdfmodel.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	remaining := 3
	for range 3 {
		_, _, used, err := scanner.Next(remaining)
		remaining -= used
		if err != nil {
			t.Fatal(err)
		}
	}
	_, _, used, err := scanner.Next(remaining)
	if remaining != 0 || used != 0 || !errors.Is(err, pdfmodel.ErrLimit) {
		t.Fatalf("remaining=%d used=%d err=%v", remaining, used, err)
	}
}

func TestContentScannerErrorsAreTerminal(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name      string
		input     string
		remaining int
	}{
		{name: "negative budget", input: "q", remaining: -1},
		{name: "partial array", input: "[1 2] q", remaining: 2},
		{name: "malformed token", input: "<badX> q", remaining: 10},
	} {
		t.Run(tt.name, func(t *testing.T) {
			scanner, err := NewContentScanner([]byte(tt.input), 1, 0, pdfmodel.Limits{})
			if err != nil {
				t.Fatal(err)
			}
			_, _, _, firstErr := scanner.Next(tt.remaining)
			if firstErr == nil {
				t.Fatal("expected parsing or budget error")
			}
			object, operator, used, err := scanner.Next(100)
			if err != firstErr || used != 0 || operator || object.Value != nil {
				t.Fatalf("continued after error: object=%#v operator=%v values=%d err=%v",
					object, operator, used, err)
			}
		})
	}
}

func TestContentScannerDefersMalformedTail(t *testing.T) {
	t.Parallel()
	for _, tail := range []string{"(unterminated", "<invalid", "[1", "<< /A >>", "]", "0 0 R"} {
		t.Run(tail, func(t *testing.T) {
			scanner, err := NewContentScanner([]byte("q 12 "+tail), 6, 40, pdfmodel.Limits{})
			if err != nil {
				t.Fatalf("constructor parsed content: %v", err)
			}
			for range 2 {
				if _, _, _, err := scanner.Next(100); err != nil {
					t.Fatalf("valid prefix failed: %v", err)
				}
			}
			_, _, _, err = scanner.Next(100)
			if err == nil || errors.Is(err, io.EOF) {
				t.Fatalf("malformed trailing syntax did not fail: %v", err)
			}
			var syntaxErr *syntaxError
			if !errors.As(err, &syntaxErr) || syntaxErr.Position.Source != 6 || syntaxErr.Position.Offset < 45 {
				t.Fatalf("error omitted absolute source position: %v", err)
			}
		})
	}
}

func TestContentScannerTokenCountIncludesTriviaAndContainers(t *testing.T) {
	t.Parallel()
	const tokenLimit = 1 << 20
	trivia := strings.Repeat("%\n", tokenLimit/2)
	for _, tt := range []struct {
		name  string
		input string
		want  error
	}{
		{name: "exact limit then EOF", input: trivia, want: io.EOF},
		{name: "operator beyond limit", input: trivia + "q", want: pdfmodel.ErrLimit},
		{name: "trivia inside container", input: "[" + trivia + "]", want: pdfmodel.ErrLimit},
	} {
		t.Run(tt.name, func(t *testing.T) {
			scanner, err := NewContentScanner([]byte(tt.input), 1, 0, pdfmodel.Limits{})
			if err != nil {
				t.Fatal(err)
			}
			_, _, _, err = scanner.Next(100)
			if !errors.Is(err, tt.want) {
				t.Fatalf("error=%v; want %v", err, tt.want)
			}
		})
	}
}

func TestContentScannerTokenBudgetCountsCommittedLookahead(t *testing.T) {
	t.Parallel()
	const tokenLimit = 1 << 20
	for _, tt := range []struct {
		name   string
		input  string
		values []pdfmodel.Value
	}{
		{
			name:  "reference commits lookahead tokens",
			input: strings.Repeat("%\n", (tokenLimit-6)/2) + " 1 0 R",
			values: []pdfmodel.Value{
				pdfmodel.Reference{ID: pdfmodel.ObjectID{Number: 1}},
			},
		},
		{
			name:   "integers discard speculative lookahead tokens",
			input:  strings.Repeat("%\n", (tokenLimit-4)/2) + "1 2 ",
			values: []pdfmodel.Value{pdfmodel.Integer("1"), pdfmodel.Integer("2")},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			scanner, err := NewContentScanner([]byte(tt.input+" q"), 1, 0, pdfmodel.Limits{})
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range tt.values {
				object, operator, used, err := scanner.Next(1)
				if err != nil || operator || used != 1 || !reflect.DeepEqual(object.Value, want) {
					t.Fatalf("object=%#v operator=%v used=%d err=%v; want value=%#v",
						object, operator, used, err, want)
				}
			}
			_, _, _, err = scanner.Next(0)
			if !errors.Is(err, pdfmodel.ErrLimit) {
				t.Fatalf("accepted tokens beyond limit: %v", err)
			}
		})
	}
}

func TestParseObjectValueBudget(t *testing.T) {
	for _, tt := range []struct {
		name       string
		input      string
		remaining  int
		wantValues int
		wantError  bool
	}{
		{"exact array", "[1 [2]]", 4, 4, false},
		{"array overflow", "[1 [2]]", 3, 3, true},
		{"dictionary values", "<< /A 1 /B [2] >>", 4, 4, false},
		{"reference is one value", "1 0 R", 1, 1, false},
		{"empty remaining", "null", 0, 0, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, _, used, err := ParseObjectWithValueBudget([]byte(tt.input), 1, 0, pdfmodel.Limits{}, tt.remaining)
			if (err != nil) != tt.wantError || used != tt.wantValues {
				t.Fatalf("used=%d err=%v; want used=%d error=%v", used, err, tt.wantValues, tt.wantError)
			}
		})
	}
}

func TestParseObjectConfiguredValueLimit(t *testing.T) {
	if _, _, err := ParseObjectWithLimits([]byte("[1 2 3]"), 1, 0, pdfmodel.Limits{MaxValues: 3}); err == nil {
		t.Fatal("array plus three values exceeded configured value limit")
	}
	if _, _, err := ParseObjectWithLimits([]byte("null"), 1, 0, pdfmodel.Limits{MaxValues: -1}); err == nil {
		t.Fatal("negative value limit accepted")
	}
}

func TestSyntaxResourceErrorsAreClassifiable(t *testing.T) {
	for _, tt := range []struct {
		input  string
		limits pdfmodel.Limits
	}{
		{"[1 2]", pdfmodel.Limits{MaxValues: 2}},
		{"[[0]]", pdfmodel.Limits{MaxDepth: 1}},
		{"(abc)", pdfmodel.Limits{MaxTokenBytes: 4}},
	} {
		_, _, err := ParseObjectWithLimits([]byte(tt.input), 1, 0, tt.limits)
		if !errors.Is(err, pdfmodel.ErrLimit) {
			t.Fatalf("%q: expected resource error, got %v", tt.input, err)
		}
	}
}

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
