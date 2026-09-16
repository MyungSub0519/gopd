package model

// StreamBoundary records how the end of a stream's payload was established.
//
// It matters because /Length is frequently wrong in real files. A reader that
// silently repairs the boundary would hide that damage, so the repair is
// recorded instead of being made invisible.
type StreamBoundary uint8

const (
	// StreamUnresolved means the payload extent is unknown: /Length was
	// missing, unusable, or the data ran past the end of the file. Stream
	// Encoded and EndKeyword are nil in this state.
	StreamUnresolved StreamBoundary = iota

	// StreamFromLength means /Length was trusted and the endstream keyword
	// was found exactly where it predicted. This is the healthy case.
	StreamFromLength

	// StreamRecovered means /Length disagreed with the file and the
	// boundary was recovered by locating the endstream keyword.
	//
	// Not yet produced: an object whose /Length does not match is currently
	// rejected. The value exists so that adding recovery does not have to
	// change this type or its callers.
	StreamRecovered
)

// Stream is a stream object: a dictionary followed by a raw byte payload.
//
// Stream holds no bytes of its own. Every field is a location in the source,
// so a Stream stays valid and cheap no matter how large the payload is, and
// the original bytes remain available for callers that need provenance rather
// than content. Use Document.DecodeStream to obtain the decoded payload.
type Stream struct {
	// Dictionary is the stream's dictionary, which supplies /Length,
	// /Filter, /DecodeParms and any type-specific keys.
	Dictionary     Dictionary
	DictionarySpan Span

	// StartKeyword covers the stream keyword itself, not the payload.
	StartKeyword Span

	// DataStart is the first payload byte, after the end-of-line that the
	// specification requires to follow the stream keyword.
	DataStart Position

	// Encoded covers the payload as stored, before any filter is applied.
	// It is nil when Boundary is StreamUnresolved.
	Encoded *Span

	// EndKeyword covers the endstream keyword, or is nil if it was never
	// confirmed.
	EndKeyword *Span

	Boundary StreamBoundary
}

func (Stream) pdfValue() {}
