package content

import (
	"fmt"
	"math"
	"strings"

	"github.com/MyungSub0519/gopd/internal/model"
)

type textState struct {
	font                                              int
	size, charSpace, wordSpace, hscale, leading, rise float64
	renderMode                                        int
}
type contentState struct {
	graphics GraphicsState
	text     textState
}

func initialContentState() contentState {
	return contentState{graphics: GraphicsState{CTM: model.IdentityMatrix(), LineWidth: 1, MiterLimit: 10, Stroke: Color{Space: "DeviceGray", Components: []float64{0}}, Fill: Color{Space: "DeviceGray", Components: []float64{0}}, StrokeAlpha: 1, FillAlpha: 1, BlendMode: "Normal", Complete: true}, text: textState{font: -1, hscale: 1}}
}

func operationNumbers(op Operation, n int) ([]float64, error) {
	if len(op.Operands) != n {
		return nil, fmt.Errorf("expected %d operands, got %d", n, len(op.Operands))
	}
	numbers := make([]float64, n)
	for i, operand := range op.Operands {
		value, err := model.Number(operand)
		if err != nil {
			return nil, err
		}
		numbers[i] = value
	}
	return numbers, nil
}
func operationName(op Operation) (model.Name, error) {
	if len(op.Operands) != 1 {
		return "", fmt.Errorf("expected one name operand")
	}
	name, ok := op.Operands[0].Value.(model.Name)
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
func translate(x, y float64) model.Matrix { return model.Matrix{1, 0, 0, 1, x, y} }

func finitePoint(p model.Point) bool {
	return !math.IsNaN(p.X) && !math.IsNaN(p.Y) && !math.IsInf(p.X, 0) && !math.IsInf(p.Y, 0)
}
func finiteMatrix(m model.Matrix) bool {
	for _, v := range m {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return true
}

func (c *contentInterpreter) execute(op Operation, index int) error {
	g := &c.state.graphics
	t := &c.state.text
	switch op.Operator {
	case "q":
		if err := noOperands(op); err != nil {
			return err
		}
		if len(c.stack) >= 256 || len(c.stack) >= c.b.maxDepth {
			return fmt.Errorf("graphics state depth limit")
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
		n, e := operationNumbers(op, 6)
		if e != nil {
			return e
		}
		g.CTM = g.CTM.Mul(model.Matrix(n))
		for _, v := range g.CTM {
			if math.IsInf(v, 0) || math.IsNaN(v) {
				return fmt.Errorf("matrix overflow")
			}
		}
	case "w", "J", "j", "M":
		n, e := operationNumbers(op, 1)
		if e != nil {
			return e
		}
		switch op.Operator {
		case "w":
			if n[0] < 0 {
				return fmt.Errorf("negative line width")
			}
			g.LineWidth = n[0]
		case "J", "j":
			if n[0] != math.Trunc(n[0]) || n[0] < 0 || n[0] > 2 {
				return fmt.Errorf("invalid line cap/join")
			}
			if op.Operator == "J" {
				g.LineCap = int(n[0])
			} else {
				g.LineJoin = int(n[0])
			}
		case "M":
			if n[0] < 1 {
				return fmt.Errorf("invalid miter limit")
			}
			g.MiterLimit = n[0]
		}
	case "d":
		if len(op.Operands) != 2 {
			return fmt.Errorf("d requires two operands")
		}
		dash, e := c.b.numbers(op.Operands[0], -1)
		if e != nil {
			return e
		}
		total := float64(0)
		for _, v := range dash {
			if v < 0 {
				return fmt.Errorf("negative dash length")
			}
			total += v
		}
		if len(dash) > 0 && total == 0 {
			return fmt.Errorf("all dash lengths zero")
		}
		phase, e := model.Number(op.Operands[1])
		if e != nil {
			return e
		}
		g.Dash = dash
		g.DashPhase = phase
	case "ri":
		name, e := operationName(op)
		if e != nil {
			return e
		}
		g.RenderingIntent = name
	case "gs":
		name, e := operationName(op)
		if e != nil {
			return e
		}
		return c.extGState(name, op)
	case "G", "g", "RG", "rg", "K", "k":
		count := 1
		space := model.Name("DeviceGray")
		if op.Operator == "RG" || op.Operator == "rg" {
			count = 3
			space = "DeviceRGB"
		}
		if op.Operator == "K" || op.Operator == "k" {
			count = 4
			space = "DeviceCMYK"
		}
		n, e := operationNumbers(op, count)
		if e != nil {
			return e
		}
		color := Color{Space: space, Components: n}
		if op.Operator == strings.ToUpper(op.Operator) {
			g.Stroke = color
		} else {
			g.Fill = color
		}
	case "CS", "cs":
		name, e := operationName(op)
		if e != nil {
			return e
		}
		color := Color{Space: name}
		switch name {
		case "DeviceGray":
			color.Components = []float64{0}
		case "DeviceRGB":
			color.Components = []float64{0, 0, 0}
		case "DeviceCMYK":
			color.Components = []float64{0, 0, 0, 1}
		default:
			c.unsupported(op, "Non-device color spaces are retained without color conversion")
		}
		if op.Operator == "CS" {
			g.Stroke = color
		} else {
			g.Fill = color
		}
	case "SC", "SCN", "sc", "scn":
		color := g.Fill
		if op.Operator == "SC" || op.Operator == "SCN" {
			color = g.Stroke
		}
		color.Components = nil
		for i, operand := range op.Operands {
			if name, ok := operand.Value.(model.Name); ok && i == len(op.Operands)-1 {
				color.Pattern = name
				c.unsupported(op, "Pattern colors are retained without pattern execution")
			} else {
				n, e := model.Number(operand)
				if e != nil {
					return e
				}
				color.Components = append(color.Components, n)
			}
		}
		if op.Operator == "SC" || op.Operator == "SCN" {
			g.Stroke = color
		} else {
			g.Fill = color
		}
	case "m", "l", "c", "v", "y", "h", "re":
		return c.pathOperation(op, index)
	case "W", "W*":
		if e := noOperands(op); e != nil {
			return e
		}
		c.pendingClip = true
		c.clipEvenOdd = op.Operator == "W*"
		c.pathOperations = append(c.pathOperations, index)
	case "S", "s", "f", "F", "f*", "B", "B*", "b", "b*", "n":
		if e := noOperands(op); e != nil {
			return e
		}
		return c.paint(op, index)
	case "BT":
		if e := noOperands(op); e != nil {
			return e
		}
		if c.inText {
			return fmt.Errorf("nested BT")
		}
		c.inText = true
		c.textMatrix = model.IdentityMatrix()
		c.lineMatrix = model.IdentityMatrix()
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
			return fmt.Errorf("Tf requires font name and size")
		}
		name, ok := op.Operands[0].Value.(model.Name)
		if !ok {
			return fmt.Errorf("Tf font is not a name")
		}
		size, e := model.Number(op.Operands[1])
		if e != nil {
			return e
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
			if t.renderMode >= 4 {
				c.unsupported(op, "Text clipping requires glyph outlines and is not applied")
			}
		}
	case "Tm":
		if !c.inText {
			return fmt.Errorf("Tm outside text object")
		}
		n, e := operationNumbers(op, 6)
		if e != nil {
			return e
		}
		c.textMatrix = model.Matrix(n)
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
			return fmt.Errorf("T* outside text object")
		}
		c.moveText(0, -t.leading)
	case "Tj", "TJ", "'", "\"":
		return c.showText(op, index)
	case "Do":
		name, e := operationName(op)
		if e != nil {
			return e
		}
		return c.xobject(name, op, index)
	case "BI", "ID", "EI":
		return fmt.Errorf("inline image content is unsupported; original content source is retained")
	case "BMC", "BDC", "EMC", "MP", "DP":
		c.unsupported(op, "Marked-content properties and optional-content visibility are retained without evaluation")
	case "BX", "EX":
		if e := noOperands(op); e != nil {
			return e
		}
	default:
		c.unsupported(op, fmt.Sprintf("Operator %s is retained but its effect is unsupported", op.Operator))
	}
	return nil
}

func (c *contentInterpreter) pathOperation(op Operation, index int) error {
	count := map[string]int{"m": 2, "l": 2, "c": 6, "v": 4, "y": 4, "h": 0, "re": 4}[op.Operator]
	n, e := operationNumbers(op, count)
	if e != nil {
		return e
	}
	if op.Operator != "m" && op.Operator != "re" && len(c.path) == 0 {
		return fmt.Errorf("path operator without current point")
	}
	var points []model.Point
	for i := 0; i < len(n); i += 2 {
		points = append(points, c.state.graphics.CTM.Transform(model.Point{X: n[i], Y: n[i+1]}))
	}
	if op.Operator == "re" {
		points = c.rectanglePoints(model.Rect{Min: model.Point{X: n[0], Y: n[1]}, Max: model.Point{X: n[0] + n[2], Y: n[1] + n[3]}})
	}
	if op.Operator == "v" {
		points = append([]model.Point{c.currentPoint()}, points...)
	}
	if op.Operator == "y" {
		points = append(points, points[len(points)-1])
	}
	if op.Operator == "h" {
		points = []model.Point{c.subpathStart()}
	}
	for _, point := range points {
		if !finitePoint(point) {
			return fmt.Errorf("path coordinate overflow")
		}
	}
	c.path = append(c.path, DetailedPathSegment{Operator: op.Operator, Points: points, Span: op.Span})
	c.pathOperations = append(c.pathOperations, index)
	return nil
}
func (c *contentInterpreter) currentPoint() model.Point {
	segment := c.path[len(c.path)-1]
	if segment.Operator == "re" {
		return segment.Points[0]
	}
	return segment.Points[len(segment.Points)-1]
}
func (c *contentInterpreter) subpathStart() model.Point {
	for i := len(c.path) - 1; i >= 0; i-- {
		if c.path[i].Operator == "m" || c.path[i].Operator == "re" {
			return c.path[i].Points[0]
		}
	}
	return model.Point{}
}
func (c *contentInterpreter) rectanglePoints(r model.Rect) []model.Point {
	m := c.state.graphics.CTM
	return []model.Point{m.Transform(r.Min), m.Transform(model.Point{X: r.Max.X, Y: r.Min.Y}), m.Transform(r.Max), m.Transform(model.Point{X: r.Min.X, Y: r.Max.Y})}
}
func (c *contentInterpreter) addRectClip(r model.Rect, span model.Span) error {
	points := c.rectanglePoints(r)
	for _, point := range points {
		if !finitePoint(point) {
			return fmt.Errorf("clipping coordinate overflow")
		}
	}
	return c.addClip(ClipPath{Segments: []DetailedPathSegment{{Operator: "re", Points: points, Span: span}}})
}
func (c *contentInterpreter) addClip(clip ClipPath) error {
	count := len(c.state.graphics.Clip) + 1
	if count > c.b.maxObjects-c.b.clipReferences {
		return fmt.Errorf("cumulative clipping snapshot limit exceeded")
	}
	c.b.clipReferences += count
	clips := append([]ClipPath(nil), c.state.graphics.Clip...)
	c.state.graphics.Clip = append(clips, clip)
	return nil
}

func (c *contentInterpreter) paint(op Operation, index int) error {
	if (op.Operator == "s" || op.Operator == "b" || op.Operator == "b*") && len(c.path) > 0 {
		c.path = append(c.path, DetailedPathSegment{Operator: "h", Points: []model.Point{c.subpathStart()}, Span: op.Span})
	}
	if op.Operator != "n" && len(c.path) > 0 {
		source := c.source(op, index)
		source.Operations = append(append([]int(nil), c.pathOperations...), index)
		for _, segment := range c.path {
			source.Spans = append(source.Spans, segment.Span)
		}
		graphic := DetailedGraphic{Source: source, Segments: c.path, Paint: op.Operator, State: c.state.graphics, EvenOdd: strings.HasSuffix(op.Operator, "*")}
		graphic.Stroke = op.Operator == "S" || op.Operator == "s" || strings.HasPrefix(op.Operator, "B") || strings.HasPrefix(op.Operator, "b")
		graphic.Fill = op.Operator != "S" && op.Operator != "s"
		c.item(ElementGraphic, len(c.b.pdf.Graphics))
		c.b.pdf.Graphics = append(c.b.pdf.Graphics, graphic)
	}
	if c.pendingClip {
		if err := c.addClip(ClipPath{Segments: c.path, EvenOdd: c.clipEvenOdd}); err != nil {
			return err
		}
	}
	c.path = nil
	c.pathOperations = nil
	c.pendingClip = false
	return nil
}

func (c *contentInterpreter) moveText(x, y float64) {
	c.lineMatrix = c.lineMatrix.Mul(translate(x, y))
	c.textMatrix = c.lineMatrix
	c.positionComplete = true
}
func (c *contentInterpreter) showText(op Operation, index int) error {
	if !c.inText {
		return fmt.Errorf("text show outside BT/ET")
	}
	t := &c.state.text
	var elements []model.Object
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
		t.wordSpace, e = model.Number(op.Operands[0])
		if e != nil {
			return e
		}
		t.charSpace, e = model.Number(op.Operands[1])
		if e != nil {
			return e
		}
		c.moveText(0, -t.leading)
		elements = op.Operands[2:]
	case "TJ":
		if len(op.Operands) != 1 {
			return fmt.Errorf("TJ requires one array")
		}
		array, ok := op.Operands[0].Value.(model.Array)
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
		raw, ok := element.Value.(model.PDFString)
		if !ok {
			if op.Operator != "TJ" {
				return fmt.Errorf("text show operand is not a string")
			}
			number, e := model.Number(element)
			if e != nil {
				return e
			}
			c.textMatrix = c.textMatrix.Mul(translate(-number/1000*t.size*t.hscale, 0))
			if !finiteMatrix(c.textMatrix) {
				return fmt.Errorf("text displacement overflow")
			}
			continue
		}
		if len(raw.Bytes) > c.b.maxObjects-c.b.glyphCodes {
			return fmt.Errorf("text character-code byte limit exceeded")
		}
		c.b.glyphCodes += len(raw.Bytes)
		text.RawCodes = append(text.RawCodes, raw.Bytes...)
		decoded, codes, complete, decodeError := font.decodeBounded(raw.Bytes, c.b.maxUnicodeBytes-c.b.unicodeBytes)
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
			advance := (width/1000*t.size + t.charSpace) * t.hscale
			if len(code.bytes) == 1 && code.bytes[0] == 32 {
				advance += t.wordSpace * t.hscale
			}
			matrix := c.state.graphics.CTM.Mul(c.textMatrix)
			origin := matrix.Transform(model.Point{X: 0, Y: t.rise})
			end := matrix.Transform(model.Point{X: advance, Y: t.rise})
			if !finitePoint(origin) || !finitePoint(end) {
				return fmt.Errorf("text glyph coordinate overflow")
			}
			text.Glyphs = append(text.Glyphs, Glyph{Code: code.bytes, Unicode: code.unicode, Origin: origin, Advance: model.Point{X: end.X - origin.X, Y: end.Y - origin.Y}, DecodeComplete: code.complete, WidthKnown: widthKnown})
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
		c.b.diag("incomplete-text-decoding", "Some character codes have no supported Unicode mapping", op.Span)
	}
	if !text.PositionComplete {
		c.b.diag("incomplete-text-positioning", "Some glyph widths or writing directions are unsupported", op.Span)
	}
	c.item(ElementText, len(c.b.pdf.Texts))
	c.b.pdf.Texts = append(c.b.pdf.Texts, text)
	return nil
}

func (c *contentInterpreter) extGState(name model.Name, op Operation) error {
	object, err := c.resource("ExtGState", name)
	if err != nil {
		return err
	}
	object, err = c.b.doc.ResolveObject(object)
	if err != nil {
		return err
	}
	dict, err := semDictionary(object)
	if err != nil {
		return err
	}
	for _, entry := range dict.Entries {
		value, err := c.b.doc.ResolveObject(entry.Value)
		if err != nil {
			return err
		}
		switch entry.Key {
		case "Type":
		case "LW", "LC", "LJ", "ML":
			mapped := map[model.Name]string{"LW": "w", "LC": "J", "LJ": "j", "ML": "M"}[entry.Key]
			if err = c.execute(Operation{Operator: mapped, Operands: []model.Object{value}, Span: op.Span}, -1); err != nil {
				return err
			}
		case "CA", "ca":
			n, e := model.Number(value)
			if e != nil || n < 0 || n > 1 {
				return fmt.Errorf("invalid alpha value")
			}
			if entry.Key == "CA" {
				c.state.graphics.StrokeAlpha = n
			} else {
				c.state.graphics.FillAlpha = n
			}
		case "BM":
			mode, ok := value.Value.(model.Name)
			if !ok {
				c.unsupported(op, "Blend mode arrays are retained without choosing a renderer-supported mode")
			} else {
				c.state.graphics.BlendMode = mode
			}
		case "D":
			array, ok := value.Value.(model.Array)
			if !ok {
				return fmt.Errorf("invalid ExtGState D")
			}
			if err = c.execute(Operation{Operator: "d", Operands: array.Items, Span: op.Span}, -1); err != nil {
				return err
			}
		case "RI":
			if err = c.execute(Operation{Operator: "ri", Operands: []model.Object{value}, Span: op.Span}, -1); err != nil {
				return err
			}
		case "Font":
			array, ok := value.Value.(model.Array)
			if !ok || len(array.Items) != 2 {
				return fmt.Errorf("invalid ExtGState Font")
			}
			font, e := c.b.font(array.Items[0])
			if e != nil {
				return e
			}
			size, e := model.Number(array.Items[1])
			if e != nil {
				return e
			}
			c.state.text.font = font
			c.state.text.size = size
		case "SMask":
			if mode, ok := value.Value.(model.Name); !ok || mode != "None" {
				c.unsupported(op, "Soft masks are retained without evaluating their transparency")
			}
		default:
			c.unsupported(op, fmt.Sprintf("ExtGState /%s is retained but its rendering effect is unsupported", entry.Key))
		}
	}
	return nil
}
