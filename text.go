package gopd

import (
	"fmt"
	"strings"
)

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
