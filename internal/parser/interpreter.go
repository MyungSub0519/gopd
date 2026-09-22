package parser

import (
	"errors"
	"fmt"
	"io"
	"math"

	"github.com/MyungSub0519/gopd/internal/common/syntax"
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

type textState struct {
	font                                              int
	size, charSpace, wordSpace, hscale, leading, rise float64
	renderMode                                        int
}

type contentState struct {
	graphics GraphicsState
	text     textState
}

type FormCall struct {
	ObjectID ObjectID
	Span     Span
	Call     Span
}

type ElementSource struct {
	Page       int
	Spans      []Span
	Operations []int // indexes into the page's flattened Operations slice
	FormPath   []FormCall
}

type Operation struct {
	Operator string
	Operands []Object
	Span     Span // operator token; operands retain their own source spans
	FormPath []FormCall
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
	if c.b.result != nil {
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

func initialContentState() contentState {
	return contentState{
		graphics: GraphicsState{
			CTM: IdentityMatrix(), LineWidth: 1, MiterLimit: 10,
			Stroke:      Color{Space: "DeviceGray", Components: []float64{0}},
			Fill:        Color{Space: "DeviceGray", Components: []float64{0}},
			StrokeAlpha: 1, FillAlpha: 1, BlendMode: "Normal", Complete: true,
		},
		text: textState{font: -1, hscale: 1},
	}
}

func operationNumbers(op Operation, n int) ([]float64, error) {
	if len(op.Operands) != n {
		return nil, fmt.Errorf("expected %d operands, got %d", n, len(op.Operands))
	}
	numbers := make([]float64, n)
	for i, operand := range op.Operands {
		value, err := Number(operand)
		if err != nil {
			return nil, err
		}
		numbers[i] = value
	}
	return numbers, nil
}

func operationName(op Operation) (Name, error) {
	if len(op.Operands) != 1 {
		return "", fmt.Errorf("expected one name operand")
	}
	name, ok := op.Operands[0].Value.(Name)
	if !ok {
		return "", fmt.Errorf("expected name")
	}
	return name, nil
}

func noOperands(op Operation) error {
	if len(op.Operands) != 0 {
		return fmt.Errorf("expected no operands")
	}
	return nil
}

func translate(x, y float64) Matrix { return Matrix{1, 0, 0, 1, x, y} }

func finitePoint(p Point) bool {
	return !math.IsNaN(p.X) && !math.IsNaN(p.Y) && !math.IsInf(p.X, 0) && !math.IsInf(p.Y, 0)
}

func finiteMatrix(m Matrix) bool {
	for _, v := range m {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return true
}

func (c *contentInterpreter) execute(op Operation, index int) error {
	g := &c.state.graphics
	if !c.b.wantStyles() {
		switch op.Operator {
		case "w", "J", "j", "M", "d", "ri", "G", "g", "RG", "rg", "K", "k",
			"CS", "cs", "SC", "SCN", "sc", "scn":
			return nil
		}
	}
	switch op.Operator {
	case "q":
		if err := noOperands(op); err != nil {
			return err
		}
		if len(c.stack) >= 256 || len(c.stack) >= c.b.maxDepth {
			return fmt.Errorf("%w: graphics state depth limit", ErrLimit)
		}
		c.stack = append(c.stack, c.state)
	case "Q":
		if err := noOperands(op); err != nil {
			return err
		}
		if len(c.stack) == 0 {
			return fmt.Errorf("graphics state stack underflow")
		}
		c.state = c.stack[len(c.stack)-1]
		c.stack = c.stack[:len(c.stack)-1]
	case "cm":
		if !c.b.wantTransforms() {
			return nil
		}
		n, e := operationNumbers(op, 6)
		if e != nil {
			return e
		}
		g.CTM = g.CTM.Mul(Matrix(n))
		for _, v := range g.CTM {
			if math.IsInf(v, 0) || math.IsNaN(v) {
				return fmt.Errorf("matrix overflow")
			}
		}
	case "BT", "ET", "Tf", "Tc", "Tw", "Tz", "TL", "Ts", "Tr",
		"Tm", "Td", "TD", "T*", "Tj", "TJ", "'", "\"":
		return c.executeText(op, index)
	case "w", "J", "j", "M", "d", "ri", "G", "g", "RG", "rg", "K", "k",
		"CS", "cs", "SC", "SCN", "sc", "scn", "m", "l", "c", "v", "y", "h", "re",
		"W", "W*", "S", "s", "f", "F", "f*", "B", "B*", "b", "b*", "n", "sh":
		return c.executeGraphic(op, index)
	case "gs":
		name, e := operationName(op)
		if e != nil {
			return e
		}
		return c.extGState(name, op)
	case "Do":
		name, e := operationName(op)
		if e != nil {
			return e
		}
		return c.xobject(name, op, index)
	case "BI", "ID", "EI":
		if c.b.result != nil && !c.b.wantProvenance() {
			return fmt.Errorf("inline image content is unsupported")
		}
		return fmt.Errorf("inline image content is unsupported; original content source is retained")
	case "BMC", "BDC", "EMC", "MP", "DP":
		return c.unsupported(op, "Marked-content properties and optional-content visibility are retained without evaluation")
	case "BX", "EX":
		if e := noOperands(op); e != nil {
			return e
		}
	default:
		return c.unsupported(op, fmt.Sprintf("Operator %s is retained but its effect is unsupported", op.Operator))
	}
	return nil
}
