package syntax

import (
	"errors"
	"io"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/MyungSub0519/gopd/internal/pdfmodel"
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
