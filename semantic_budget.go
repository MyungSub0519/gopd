package gopd

import "fmt"

// Visits count even when the underlying object or decoded source is cached.
func (b *semanticBuilder) chargeSemantic(kind string, span Span) error {
	return b.chargeSemanticWork(1, kind, span)
}

func (b *semanticBuilder) chargeSemanticWork(count int, kind string, span Span) error {
	if count > b.maxSemanticObjects-b.semanticObjects {
		return fmt.Errorf("%w: semantic object limit while visiting %s at %+v", ErrLimit, kind, span)
	}
	b.semanticObjects += count
	return nil
}

// Content operands, expanded numeric resources and retained style components
// share the BuildPDF value budget; the Document's syntax budget is independent.
func (b *semanticBuilder) chargeValues(count int, kind string, span Span) error {
	if count > b.maxValues-b.semanticValues {
		return fmt.Errorf("%w: semantic value count limit for %s at %+v", ErrLimit, kind, span)
	}
	b.semanticValues += count
	return nil
}

func (b *semanticBuilder) chargeStyle(state GraphicsState, span Span) error {
	// Charge every occurrence even when detailed results share the slices.
	// The basic projection copies these components into each element's style.
	for _, values := range [][]float64{state.Dash, state.Stroke.Components, state.Fill.Components} {
		if err := b.chargeValues(len(values), "retained graphics style", span); err != nil {
			return err
		}
	}
	return nil
}
