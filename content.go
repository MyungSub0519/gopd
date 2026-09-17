package gopd

import (
	"errors"
	"fmt"
	"io"

	"github.com/MyungSub0519/gopd/internal/syntax"
)

type contentInterpreter struct {
	b                      *semanticBuilder
	page                   int
	resources              Dictionary
	state                  contentState
	stack                  []contentState
	textMatrix, lineMatrix Matrix
	inText                 bool
	positionComplete       bool
	path                   []DetailedPathSegment
	pathOperations         []int
	pendingClip            bool
	clipEvenOdd            bool
	operands               []Object
	formPath               []FormCall
	formDepth              int
	pathStarted            bool
}

func (c *contentInterpreter) stream(stream Stream) error {
	source, err := c.contentSource(stream)
	if err != nil {
		return err
	}
	return c.interpretSource(source)
}

func (c *contentInterpreter) contentSource(stream Stream) (Source, error) {
	if err := c.b.chargeSemantic("content stream", stream.DictionarySpan); err != nil {
		return Source{}, err
	}
	source, err := c.b.doc.DecodeStream(stream)
	if err != nil {
		return Source{}, err
	}
	if source.Size > c.b.maxContentBytes-c.b.contentBytes {
		return Source{}, fmt.Errorf("%w: cumulative content byte limit exceeded at source %d", ErrLimit, source.ID)
	}
	c.b.contentBytes += source.Size
	return source, nil
}

func (c *contentInterpreter) interpretSource(source Source) error {
	data, err := c.b.doc.Bytes(Span{Source: source.ID, Start: 0, End: source.Size})
	if err != nil {
		return err
	}
	if c.b.extract != nil {
		return c.interpretSelected(data, source.ID)
	}
	tokens, err := Lex(data, source.ID, 0)
	if err != nil {
		return fmt.Errorf("content at source %d: %w", source.ID, err)
	}
	for i := 0; i < len(tokens); {
		token := tokens[i]
		if token.Kind == TokenWhitespace || token.Kind == TokenComment || token.Kind == TokenEOF {
			i++
			continue
		}
		word := string(data[token.Span.Start:token.Span.End])
		if token.Kind == TokenKeyword && word != "true" && word != "false" && word != "null" {
			if err := c.b.chargeSemantic("content operation", token.Span); err != nil {
				return err
			}
			c.b.operations++
			if c.b.operations > c.b.maxObjects {
				return fmt.Errorf("%w: content operation limit at %+v", ErrLimit, token.Span)
			}
			op := Operation{Operator: word, Operands: c.operands, Span: token.Span, FormPath: append([]FormCall(nil), c.formPath...)}
			c.operands = nil
			index := len(c.b.pdf.Pages[c.page].Operations)
			c.b.pdf.Pages[c.page].Operations = append(c.b.pdf.Pages[c.page].Operations, op)
			if err = c.execute(op, index); err != nil {
				return fmt.Errorf("operator %s at source %d offset %d: %w", word, token.Span.Source, token.Span.Start, err)
			}
			i++
			continue
		}
		object, consumed, values, e := syntax.ParseObjectWithValueBudget(data[token.Span.Start:], source.ID, token.Span.Start, c.b.doc.Options.Limits, c.b.maxValues-c.b.semanticValues)
		c.b.semanticValues += values
		if e != nil {
			return e
		}
		if consumed <= 0 {
			return fmt.Errorf("content parser made no progress at %+v", token.Span)
		}
		c.operands = append(c.operands, object)
		if len(c.operands) > 65536 {
			return fmt.Errorf("%w: content operand limit at %+v", ErrLimit, token.Span)
		}
		end := token.Span.Start + int64(consumed)
		for i < len(tokens) && tokens[i].Span.Start < end {
			i++
		}
	}
	return nil
}

func (c *contentInterpreter) source(op Operation, index int) ElementSource {
	if !c.b.wantProvenance() {
		return ElementSource{Page: c.page}
	}
	spans := make([]Span, 0, len(op.Operands)+1)
	for _, operand := range op.Operands {
		spans = append(spans, operand.Span)
	}
	spans = append(spans, op.Span)
	return ElementSource{Page: c.page, Spans: spans, Operations: []int{index}, FormPath: append([]FormCall(nil), c.formPath...)}
}

func (c *contentInterpreter) interpretSelected(data []byte, source SourceID) error {
	scanner, err := syntax.NewContentScanner(data, source, 0, c.b.doc.Options.Limits)
	if err != nil {
		return err
	}
	for {
		object, operator, values, err := scanner.Next(c.b.maxValues - c.b.semanticValues)
		c.b.semanticValues += values
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if !operator {
			c.operands = append(c.operands, object)
			if len(c.operands) > 65536 {
				return fmt.Errorf("%w: content operand limit at %+v", ErrLimit, object.Span)
			}
			continue
		}
		if err := c.b.chargeSemantic("content operation", object.Span); err != nil {
			return err
		}
		c.b.operations++
		if c.b.operations > c.b.maxObjects {
			return fmt.Errorf("%w: content operation limit at %+v", ErrLimit, object.Span)
		}
		op := Operation{Operator: string(object.Value.(Name)), Operands: c.operands, Span: object.Span}
		c.operands = nil
		index := -1
		if c.b.wantProvenance() {
			op.FormPath = append([]FormCall(nil), c.formPath...)
			page := &c.b.pdf.Pages[c.page]
			index = len(page.Operations)
			page.Operations = append(page.Operations, op)
		}
		if err := c.execute(op, index); err != nil {
			return fmt.Errorf("operator %s at source %d offset %d: %w", op.Operator, source, op.Span.Start, err)
		}
	}
}

func (c *contentInterpreter) item(kind ElementKind, index int) {
	page := &c.b.pdf.Pages[c.page]
	page.Items = append(page.Items, ElementRef{kind, index})
}

func (c *contentInterpreter) unsupported(op Operation, message string) error {
	c.state.graphics.Complete = false
	return c.b.diag("unsupported-content-effect", message, op.Span)
}
