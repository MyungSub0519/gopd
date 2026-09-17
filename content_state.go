package gopd

import (
	"fmt"
	"math"
	"strings"
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
	t := &c.state.text
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
	case "w", "J", "j", "M":
		n, e := operationNumbers(op, 1)
		if e != nil {
			return e
		}
		return c.setLineParameter(op.Operator, n[0])
	case "d":
		if len(op.Operands) != 2 {
			return fmt.Errorf("d requires two operands")
		}
		dash, e := c.b.numbers(op.Operands[0], -1)
		if e != nil {
			return e
		}
		phase, e := Number(op.Operands[1])
		if e != nil {
			return e
		}
		return c.setDash(dash, phase)
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
		space := Name("DeviceGray")
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
			if err := c.unsupported(op, "Non-device color spaces are retained without color conversion"); err != nil {
				return err
			}
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
			if name, ok := operand.Value.(Name); ok && i == len(op.Operands)-1 {
				color.Pattern = name
				if err := c.unsupported(op, "Pattern colors are retained without pattern execution"); err != nil {
					return err
				}
			} else {
				n, e := Number(operand)
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
		if c.b.wantStyles() {
			c.pendingClip = true
			c.clipEvenOdd = op.Operator == "W*"
			if c.b.wantProvenance() {
				c.pathOperations = append(c.pathOperations, index)
			}
		}
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
	case "Do":
		name, e := operationName(op)
		if e != nil {
			return e
		}
		return c.xobject(name, op, index)
	case "BI", "ID", "EI":
		if c.b.extract != nil && !c.b.wantProvenance() {
			return fmt.Errorf("inline image content is unsupported")
		}
		return fmt.Errorf("inline image content is unsupported; original content source is retained")
	case "sh":
		if !c.b.wants(ContentGraphics) {
			return nil
		}
		return c.unsupported(op, "Operator sh is retained but its effect is unsupported")
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

func (c *contentInterpreter) pathOperation(op Operation, index int) error {
	count := map[string]int{"m": 2, "l": 2, "c": 6, "v": 4, "y": 4, "h": 0, "re": 4}[op.Operator]
	n, e := operationNumbers(op, count)
	if e != nil {
		return e
	}
	if !c.b.wantPaths() {
		if op.Operator != "m" && op.Operator != "re" && !c.pathStarted {
			return fmt.Errorf("path operator without current point")
		}
		c.pathStarted = true
		return nil
	}
	if op.Operator != "m" && op.Operator != "re" && len(c.path) == 0 {
		return fmt.Errorf("path operator without current point")
	}
	var points []Point
	for i := 0; i < len(n); i += 2 {
		points = append(points, c.state.graphics.CTM.Transform(Point{X: n[i], Y: n[i+1]}))
	}
	if op.Operator == "re" {
		points = c.rectanglePoints(Rect{Min: Point{X: n[0], Y: n[1]}, Max: Point{X: n[0] + n[2], Y: n[1] + n[3]}})
	}
	if op.Operator == "v" {
		points = append([]Point{c.currentPoint()}, points...)
	}
	if op.Operator == "y" {
		points = append(points, points[len(points)-1])
	}
	if op.Operator == "h" {
		points = []Point{c.subpathStart()}
	}
	for _, point := range points {
		if !finitePoint(point) {
			return fmt.Errorf("path coordinate overflow")
		}
	}
	c.path = append(c.path, DetailedPathSegment{Operator: op.Operator, Points: points, Span: op.Span})
	if c.b.wantProvenance() {
		c.pathOperations = append(c.pathOperations, index)
	}
	return nil
}
func (c *contentInterpreter) currentPoint() Point {
	segment := c.path[len(c.path)-1]
	if segment.Operator == "re" {
		return segment.Points[0]
	}
	return segment.Points[len(segment.Points)-1]
}
func (c *contentInterpreter) subpathStart() Point {
	for i := len(c.path) - 1; i >= 0; i-- {
		if c.path[i].Operator == "m" || c.path[i].Operator == "re" {
			return c.path[i].Points[0]
		}
	}
	return Point{}
}
func (c *contentInterpreter) rectanglePoints(r Rect) []Point {
	m := c.state.graphics.CTM
	return []Point{m.Transform(r.Min), m.Transform(Point{X: r.Max.X, Y: r.Min.Y}), m.Transform(r.Max), m.Transform(Point{X: r.Min.X, Y: r.Max.Y})}
}
func (c *contentInterpreter) addRectClip(r Rect, span Span) error {
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
		return fmt.Errorf("%w: cumulative clipping snapshot limit exceeded", ErrLimit)
	}
	c.b.clipReferences += count
	clips := append([]ClipPath(nil), c.state.graphics.Clip...)
	c.state.graphics.Clip = append(clips, clip)
	return nil
}

func (c *contentInterpreter) paint(op Operation, index int) error {
	if (op.Operator == "s" || op.Operator == "b" || op.Operator == "b*") && len(c.path) > 0 {
		c.path = append(c.path, DetailedPathSegment{Operator: "h", Points: []Point{c.subpathStart()}, Span: op.Span})
	}
	if c.b.wants(ContentGraphics) && op.Operator != "n" && len(c.path) > 0 {
		if err := c.b.chargeStyle(c.state.graphics, op.Span); err != nil {
			return err
		}
		source := c.source(op, index)
		if c.b.wantProvenance() {
			source.Operations = append(append([]int(nil), c.pathOperations...), index)
			for _, segment := range c.path {
				source.Spans = append(source.Spans, segment.Span)
			}
		}
		graphic := DetailedGraphic{Source: source, Segments: c.path, Paint: op.Operator, State: c.state.graphics, EvenOdd: strings.HasSuffix(op.Operator, "*")}
		graphic.Stroke = op.Operator == "S" || op.Operator == "s" || strings.HasPrefix(op.Operator, "B") || strings.HasPrefix(op.Operator, "b")
		graphic.Fill = op.Operator != "S" && op.Operator != "s"
		c.emitGraphic(graphic)
	}
	if c.pendingClip {
		if err := c.addClip(ClipPath{Segments: c.path, EvenOdd: c.clipEvenOdd}); err != nil {
			return err
		}
	}
	c.path = nil
	c.pathOperations = nil
	c.pendingClip = false
	c.pathStarted = false
	return nil
}
