package gopd

import (
	"fmt"
	"math"
)

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
