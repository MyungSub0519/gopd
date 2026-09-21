package parser

import (
	"fmt"
	"math"
	"strings"
)

// Text is one text-show operation. Matrix is in unrotated page user space,
// before font scaling. Glyph positions and source bytes are available in Details.
type Text struct {
	Page             int
	Unicode          string
	Font             *FontInfo
	FontSize         float64
	Matrix           Matrix
	RenderingMode    int
	Style            PaintStyle
	DecodeComplete   bool
	PositionComplete bool
}

type Glyph struct {
	Code           []byte
	Unicode        string
	Origin         Point
	Advance        Point
	DecodeComplete bool
	WidthKnown     bool
}

type DetailedText struct {
	Source           ElementSource
	RawCodes         []byte
	Unicode          string
	DecodeComplete   bool
	PositionComplete bool
	Font             int // index in DetailedPDF.Fonts; -1 when no font was selected
	FontSize         float64
	Matrix           Matrix // page-space text matrix before the show operation, before font scaling
	RenderingMode    int
	Glyphs           []Glyph
	State            GraphicsState
}

// ExtractedText is one text-show operation, with independently selected details.
type ExtractedText struct {
	Unicode        string
	Font           *FontInfo `json:",omitempty"`
	DecodeComplete bool
	Position       *TextPosition  `json:",omitempty"`
	Style          *PaintStyle    `json:",omitempty"`
	Glyphs         []Glyph        `json:",omitempty"`
	Source         *ElementSource `json:",omitempty"`
}

// TextPosition describes text before the show operation, before font scaling.
type TextPosition struct {
	FontSize      float64
	Matrix        Matrix
	RenderingMode int
	Complete      bool
}

func (c *contentInterpreter) executeText(op Operation, index int) error {
	t := &c.state.text
	switch op.Operator {
	case "BT":
		if e := noOperands(op); e != nil {
			return e
		}
		if c.inText {
			return fmt.Errorf("nested BT")
		}
		c.inText = true
		c.textMatrix = IdentityMatrix()
		c.lineMatrix = IdentityMatrix()
		c.positionComplete = true
	case "ET":
		if e := noOperands(op); e != nil {
			return e
		}
		if !c.inText {
			return fmt.Errorf("ET outside text object")
		}
		c.inText = false
	case "Tf":
		if len(op.Operands) != 2 {
			return fmt.Errorf("operator Tf requires font name and size")
		}
		name, ok := op.Operands[0].Value.(Name)
		if !ok {
			return fmt.Errorf("operator Tf font is not a name")
		}
		size, e := Number(op.Operands[1])
		if e != nil {
			return e
		}
		if !c.b.wants(ContentText) {
			return nil
		}
		resource, e := c.resource("Font", name)
		if e != nil {
			return e
		}
		font, e := c.b.font(resource)
		if e != nil {
			return e
		}
		t.font = font
		t.size = size
	case "Tc", "Tw", "Tz", "TL", "Ts", "Tr":
		n, e := operationNumbers(op, 1)
		if e != nil {
			return e
		}
		switch op.Operator {
		case "Tc":
			t.charSpace = n[0]
		case "Tw":
			t.wordSpace = n[0]
		case "Tz":
			t.hscale = n[0] / 100
		case "TL":
			t.leading = n[0]
		case "Ts":
			t.rise = n[0]
		case "Tr":
			if n[0] < 0 || n[0] > 7 || n[0] != math.Trunc(n[0]) {
				return fmt.Errorf("invalid text rendering mode")
			}
			t.renderMode = int(n[0])
			if t.renderMode >= 4 && c.b.wantStyles() {
				return c.unsupported(op, "Text clipping requires glyph outlines and is not applied")
			}
		}
	case "Tm":
		if !c.inText {
			return fmt.Errorf("operator Tm outside text object")
		}
		if !c.b.wants(ContentText) || !c.b.wantPositions() {
			return nil
		}
		n, e := operationNumbers(op, 6)
		if e != nil {
			return e
		}
		c.textMatrix = Matrix(n)
		c.lineMatrix = c.textMatrix
		c.positionComplete = true
	case "Td", "TD":
		if !c.inText {
			return fmt.Errorf("text movement outside text object")
		}
		n, e := operationNumbers(op, 2)
		if e != nil {
			return e
		}
		if op.Operator == "TD" {
			t.leading = -n[1]
		}
		c.moveText(n[0], n[1])
	case "T*":
		if e := noOperands(op); e != nil {
			return e
		}
		if !c.inText {
			return fmt.Errorf("operator T* outside text object")
		}
		c.moveText(0, -t.leading)
	case "Tj", "TJ", "'", "\"":
		return c.showText(op, index)
	}
	return nil
}

// Emission is the only boundary that chooses a retained output model. The
// interpreter's transient state and resource caches are shared by both APIs.
func (c *contentInterpreter) emitText(text DetailedText) {
	if c.b.extract == nil {
		c.item(ElementText, len(c.b.pdf.Texts))
		c.b.pdf.Texts = append(c.b.pdf.Texts, text)
		return
	}
	b := c.b
	output := ExtractedText{Unicode: text.Unicode, DecodeComplete: text.DecodeComplete}
	if text.Font >= 0 {
		if b.fontInfos == nil {
			b.fontInfos = make(map[int]*FontInfo)
		}
		info, ok := b.fontInfos[text.Font]
		if !ok {
			font := b.pdf.Fonts[text.Font]
			info = &FontInfo{BaseFont: font.BaseFont, Subtype: font.Subtype}
			b.fontInfos[text.Font] = info
		}
		output.Font = info
	}
	if b.wantPositions() {
		output.Position = &TextPosition{
			FontSize: text.FontSize, Matrix: text.Matrix,
			RenderingMode: text.RenderingMode, Complete: text.PositionComplete,
		}
	}
	if b.wantGlyphs() {
		output.Glyphs = text.Glyphs
	}
	output.Style = b.extractStyle(text.State)
	if b.wantProvenance() {
		source := text.Source
		output.Source = &source
	}
	page := &b.extract.Pages[c.page]
	page.Texts = append(page.Texts, output)
}

func (c *contentInterpreter) moveText(x, y float64) {
	if !c.b.wants(ContentText) || !c.b.wantPositions() {
		return
	}
	c.lineMatrix = c.lineMatrix.Mul(translate(x, y))
	c.textMatrix = c.lineMatrix
	c.positionComplete = true
}

func (c *contentInterpreter) showText(op Operation, index int) error {
	if !c.inText {
		return fmt.Errorf("text show outside BT/ET")
	}
	if !c.b.wants(ContentText) {
		return nil
	}
	t := &c.state.text
	var elements []Object
	switch op.Operator {
	case "Tj", "'":
		if len(op.Operands) != 1 {
			return fmt.Errorf("text show requires one string")
		}
		elements = op.Operands
		if op.Operator == "'" {
			c.moveText(0, -t.leading)
		}
	case "\"":
		if len(op.Operands) != 3 {
			return fmt.Errorf("double quote requires word spacing, character spacing, string")
		}
		var e error
		t.wordSpace, e = Number(op.Operands[0])
		if e != nil {
			return e
		}
		t.charSpace, e = Number(op.Operands[1])
		if e != nil {
			return e
		}
		c.moveText(0, -t.leading)
		elements = op.Operands[2:]
	case "TJ":
		if len(op.Operands) != 1 {
			return fmt.Errorf("TJ requires one array")
		}
		array, ok := op.Operands[0].Value.(Array)
		if !ok {
			return fmt.Errorf("TJ operand is not array")
		}
		elements = array.Items
	}
	text := DetailedText{Source: c.source(op, index), Font: t.font, FontSize: t.size, Matrix: c.state.graphics.CTM.Mul(c.textMatrix), RenderingMode: t.renderMode, State: c.state.graphics, DecodeComplete: true, PositionComplete: c.positionComplete}
	if !finiteMatrix(text.Matrix) {
		return fmt.Errorf("text transformation overflow")
	}
	// A zero Font decodes every code as unsupported with unknown widths, so a
	// show operator before Tf still yields a Text element flagged incomplete.
	font := &Font{}
	if t.font >= 0 {
		font = &c.b.pdf.Fonts[t.font]
	}
	var result strings.Builder
	for _, element := range elements {
		raw, ok := element.Value.(PDFString)
		if !ok {
			if op.Operator != "TJ" {
				return fmt.Errorf("text show operand is not a string")
			}
			number, e := Number(element)
			if e != nil {
				return e
			}
			if !c.b.wantPositions() {
				continue
			}
			c.textMatrix = c.textMatrix.Mul(translate(-number/1000*t.size*t.hscale, 0))
			if !finiteMatrix(c.textMatrix) {
				return fmt.Errorf("text displacement overflow")
			}
			continue
		}
		if len(raw.Bytes) > c.b.maxObjects-c.b.glyphCodes {
			return fmt.Errorf("%w: text character-code byte limit exceeded", ErrLimit)
		}
		c.b.glyphCodes += len(raw.Bytes)
		if c.b.extract == nil {
			text.RawCodes = append(text.RawCodes, raw.Bytes...)
		}
		decoded, codes, complete, decodeError := font.decodeSelected(
			raw.Bytes, c.b.maxUnicodeBytes-c.b.unicodeBytes, c.b.wantPositions(),
		)
		if decodeError != nil {
			return decodeError
		}
		c.b.unicodeBytes += int64(len(decoded))
		result.WriteString(decoded)
		text.DecodeComplete = text.DecodeComplete && complete
		for _, code := range codes {
			width, widthKnown := font.widths[codeNumber(code.bytes)]
			if !widthKnown {
				width = font.defaultWidth
				widthKnown = font.defaultWidthKnown
			}
			if !font.PositioningSupported {
				widthKnown = false
			}
			advance := (width*font.effectiveWidthScale()*t.size + t.charSpace) * t.hscale
			if len(code.bytes) == 1 && code.bytes[0] == 32 {
				advance += t.wordSpace * t.hscale
			}
			matrix := c.state.graphics.CTM.Mul(c.textMatrix)
			origin := matrix.Transform(Point{X: 0, Y: t.rise})
			end := matrix.Transform(Point{X: advance, Y: t.rise})
			if !finitePoint(origin) || !finitePoint(end) {
				return fmt.Errorf("text glyph coordinate overflow")
			}
			if c.b.wantGlyphs() {
				text.Glyphs = append(text.Glyphs, Glyph{Code: code.bytes, Unicode: code.unicode, Origin: origin, Advance: Point{X: end.X - origin.X, Y: end.Y - origin.Y}, DecodeComplete: code.complete, WidthKnown: widthKnown})
			}
			text.PositionComplete = text.PositionComplete && widthKnown
			c.positionComplete = c.positionComplete && widthKnown
			c.textMatrix = c.textMatrix.Mul(translate(advance, 0))
			if !finiteMatrix(c.textMatrix) {
				return fmt.Errorf("text advance overflow")
			}
		}
	}
	text.Unicode = result.String()
	if !text.DecodeComplete {
		if err := c.b.diag("incomplete-text-decoding", "Some character codes have no supported Unicode mapping", op.Span); err != nil {
			return err
		}
	}
	if c.b.wantPositions() && !text.PositionComplete {
		if err := c.b.diag("incomplete-text-positioning", "Some glyph widths or writing directions are unsupported", op.Span); err != nil {
			return err
		}
	}
	if err := c.b.chargeStyle(text.State, op.Span); err != nil {
		return err
	}
	c.emitText(text)
	return nil
}
