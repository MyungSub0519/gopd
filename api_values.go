package gopd

import (
	"github.com/MyungSub0519/gopd/internal/model"
)

// Int converts a direct PDF Integer into int64, rejecting other types and
// out-of-range values. Resolve indirect references before calling Int.
func Int(object Object) (int64, error) { return model.Int(object) }

// Number converts a direct Integer or Real into a finite float64. It does not
// resolve indirect references or preserve exact decimal precision.
func Number(object Object) (float64, error) { return model.Number(object) }

// IsStream reports whether object directly contains a Stream. It does not
// follow references; use Document.ResolveObject first when necessary.
func IsStream(object Object) bool { return model.IsStream(object) }

// IdentityMatrix returns a transformation that leaves coordinates unchanged.
func IdentityMatrix() Matrix { return model.IdentityMatrix() }
