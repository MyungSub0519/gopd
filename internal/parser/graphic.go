package parser

import (
	"fmt"
	"math"
	"strings"
)

// Graphic is one path painting operation; it is not a whole chart or figure.
type Graphic struct {
	Page                  int
	Segments              []PathSegment
	Paint                 string
	Stroke, Fill, EvenOdd bool
	Style                 PaintStyle
}

// PathSegment points are already transformed into unrotated page user space.
type PathSegment struct {
	Operator string
	Points   []Point
}

type DetailedPathSegment struct {
	Operator string
	Points   []Point // transformed to page user space when the segment is constructed
	Span     Span
}

type DetailedGraphic struct {
	Source                ElementSource
	Segments              []DetailedPathSegment
	Paint                 string
	Stroke, Fill, EvenOdd bool
	State                 GraphicsState
}

// ExtractedGraphic is a painted path, not a complete chart or figure. Geometry
// is always in unrotated page user space, independent of Positions.
type ExtractedGraphic struct {
	Segments              []PathSegment
	Paint                 string
	Stroke, Fill, EvenOdd bool
	Style                 *PaintStyle    `json:",omitempty"`
	Source                *ElementSource `json:",omitempty"`
}

func (c *contentInterpreter) executeGraphic(op Operation, index int) error {
	g := &c.state.graphics
	switch op.Operator {
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
	case "sh":
		if !c.b.wants(ContentGraphics) {
			return nil
		}
		return c.unsupported(op, "Operator sh is retained but its effect is unsupported")
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

// Both content operands and ExtGState resources apply already resolved values.
func (c *contentInterpreter) setLineParameter(operator string, value float64) error {
	graphics := &c.state.graphics
	switch operator {
	case "w":
		if value < 0 {
			return fmt.Errorf("negative line width")
		}
		graphics.LineWidth = value
	case "J", "j":
		if value != math.Trunc(value) || value < 0 || value > 2 {
			return fmt.Errorf("invalid line cap/join")
		}
		if operator == "J" {
			graphics.LineCap = int(value)
		} else {
			graphics.LineJoin = int(value)
		}
	case "M":
		if value < 1 {
			return fmt.Errorf("invalid miter limit")
		}
		graphics.MiterLimit = value
	}
	return nil
}

func (c *contentInterpreter) setDash(dash []float64, phase float64) error {
	nonzero := false
	for _, value := range dash {
		if value < 0 {
			return fmt.Errorf("negative dash length")
		}
		nonzero = nonzero || value > 0
	}
	if len(dash) > 0 && !nonzero {
		return fmt.Errorf("all dash lengths zero")
	}
	c.state.graphics.Dash = dash
	c.state.graphics.DashPhase = phase
	return nil
}

func (c *contentInterpreter) emitGraphic(graphic DetailedGraphic) {
	if c.b.extract == nil {
		c.item(ElementGraphic, len(c.b.pdf.Graphics))
		c.b.pdf.Graphics = append(c.b.pdf.Graphics, graphic)
		return
	}
	segments := make([]PathSegment, len(graphic.Segments))
	for i, segment := range graphic.Segments {
		// Path points are immutable after construction; the path accumulator is
		// replaced after paint, so output can own these without another copy.
		segments[i] = PathSegment{Operator: segment.Operator, Points: segment.Points}
	}
	output := ExtractedGraphic{
		Segments: segments, Paint: graphic.Paint, Stroke: graphic.Stroke,
		Fill: graphic.Fill, EvenOdd: graphic.EvenOdd, Style: c.b.extractStyle(graphic.State),
	}
	if c.b.wantProvenance() {
		source := graphic.Source
		output.Source = &source
	}
	page := &c.b.extract.Pages[c.page]
	page.Graphics = append(page.Graphics, output)
}
