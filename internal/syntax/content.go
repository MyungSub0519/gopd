package syntax

import (
	"io"

	"github.com/MyungSub0519/gopd/internal/pdfmodel"
)

// ContentScanner reads content operands and operators sequentially without
// retaining a token list. It borrows data for its lifetime. Returned objects
// preserve absolute source spans and own their decoded values.
type ContentScanner struct {
	parser    objectParser
	maxValues int
	err       error
}

// NewContentScanner validates the source range and syntax limits. It defers
// content parsing until Next. Zero limits use the ParseObject defaults; each
// scanner also limits its input to 1,048,576 non-EOF tokens, including trivia.
func NewContentScanner(
	data []byte,
	source pdfmodel.SourceID,
	offset int64,
	limits pdfmodel.Limits,
) (*ContentScanner, error) {
	position := pdfmodel.Position{Source: source, Offset: offset}
	negativeLimit := limits.MaxDepth < 0 || limits.MaxTokenBytes < 0 || limits.MaxValues < 0
	if negativeLimit {
		return nil, &syntaxError{Position: position, Message: "syntax limits must not be negative"}
	}
	if limits.MaxDepth > 4096 {
		return nil, &syntaxError{Position: position, Message: "syntax nesting depth limit must not exceed 4096"}
	}
	if limits.MaxDepth == 0 {
		limits.MaxDepth = defaultSyntaxDepth
	}
	if limits.MaxTokenBytes == 0 {
		limits.MaxTokenBytes = defaultSyntaxTokenBytes
	}
	if limits.MaxValues == 0 {
		limits.MaxValues = maxSyntaxObjects
	}
	scanner, err := newSyntaxScanner(data, source, offset, limits.MaxTokenBytes)
	if err != nil {
		return nil, err
	}
	scanner.maxTokens = maxSyntaxTokens
	return &ContentScanner{
		parser:    objectParser{scanner: scanner, maxDepth: limits.MaxDepth},
		maxValues: limits.MaxValues,
	}, nil
}

// Next returns one operand or operator. For operators the boolean is true and
// Object.Value holds a Name. The count charges containers and values, excluding
// dictionary keys and operators, against remainingValues. Zero means exhausted;
// operators and EOF can still be read. Counts include work before an error.
// EOF and parsing errors terminate the scanner; further calls return that error
// without consuming additional values.
func (s *ContentScanner) Next(remainingValues int) (pdfmodel.Object, bool, int, error) {
	if s.err != nil {
		return pdfmodel.Object{}, false, 0, s.err
	}
	p := &s.parser
	if remainingValues < 0 {
		s.err = p.scanner.errorAt(p.scanner.pos, "remaining value budget must not be negative")
		return pdfmodel.Object{}, false, 0, s.err
	}
	token, err := p.scanner.nextNonTrivia()
	if err != nil {
		s.err = err
		return pdfmodel.Object{}, false, 0, err
	}
	object := pdfmodel.Object{Span: token.Span}
	if token.Kind == pdfmodel.TokenEOF {
		s.err = io.EOF
		return object, false, 0, io.EOF
	}
	if token.Kind == pdfmodel.TokenKeyword {
		word := pdfmodel.Name(p.bytes(token))
		if word != "true" && word != "false" && word != "null" {
			object.Value = word
			return object, true, 0, nil
		}
	}
	p.objects = 0
	p.maxValues = min(s.maxValues, remainingValues)
	object, err = p.parseToken(token, 0)
	s.err = err
	return object, false, p.objects, err
}
