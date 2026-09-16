package model

// ObjectOrigin describes where an indirect object was physically found.
//
// The same logical object can be stored in two very different ways, and a
// caller auditing a file needs to tell them apart: a top-level object occupies
// a byte range of the file itself, while a compressed object exists only
// inside another object's decoded payload and has no file range of its own.
//
// The interface is sealed by an unexported method, so the two cases below are
// exhaustive and a type switch over them needs no open-ended default.
type ObjectOrigin interface {
	pdfObjectOrigin()
}

// FileObjectOrigin locates an object written directly in the file as
// "n g obj ... endobj".
type FileObjectOrigin struct {
	// Whole spans the entire definition, from the object header through the
	// confirmed endobj keyword.
	Whole Span

	// Header spans just the "n g obj" introducer.
	Header Span

	// EndObj spans the endobj keyword, or is nil when damage prevented
	// confirming it.
	EndObj *Span
}

// ObjectStreamOrigin locates an object stored inside an object stream
// (/Type /ObjStm), where many objects share one compressed payload.
type ObjectStreamOrigin struct {
	// Container identifies the object stream holding this object.
	Container ObjectID

	// ContainerSpan is where the container occurs in the original file,
	// which is the only byte range in this struct that addresses the file.
	ContainerSpan Span

	// Index is the object's ordinal within the container. It is not a
	// generation number: objects in an object stream always have generation
	// zero, and the two are easy to confuse.
	Index uint32

	// HeaderPair spans the object number and relative offset pair, located
	// in the container's decoded source rather than in the file.
	HeaderPair Span
}

// IndirectObject is one numbered object together with its body and the
// location it was read from.
type IndirectObject struct {
	ID     ObjectID
	Body   Object
	Origin ObjectOrigin
}

func (FileObjectOrigin) pdfObjectOrigin()   {}
func (ObjectStreamOrigin) pdfObjectOrigin() {}
