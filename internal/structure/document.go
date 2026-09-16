package structure

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/MyungSub0519/gopd/internal/model"
)

// Document owns a snapshot of the input and provides object access over it.
//
// The whole file is read into memory once, at construction, and never read
// again; that is what makes spans stable and lets the caller close the file
// immediately. Everything beyond the header and the cross-reference chain is
// resolved lazily, so opening a large document is cheap and only the objects
// actually touched are parsed.
//
// Treat returned objects and slices as read-only: they alias cached state.
// The lazy methods mutate that cache, so a Document is not safe for concurrent
// use.
type Document struct {
	// Structure is what has been learned about the file so far. It grows as
	// objects are resolved.
	Structure model.Structure

	// Sources holds the file (always ID 1) and every decoded stream payload
	// produced so far.
	Sources map[model.SourceID]model.Source

	Options ReadOptions

	// Encrypted reports that the trailer has an /Encrypt entry. Decryption
	// is not implemented, so semantic reads refuse such a file rather than
	// returning bytes that are still ciphertext.
	Encrypted bool

	// data is the input snapshot; source 1 reads from it.
	data []byte

	// entries is the effective cross-reference: the merged view of every
	// section, where the newest definition of an object number wins.
	entries map[uint32]model.XRefRecord

	// cache holds objects already loaded, keyed by identity.
	cache map[model.ObjectID]*model.IndirectObject

	// physical holds objects already parsed, keyed by file offset. It exists
	// because two xref entries can point at the same bytes, and reparsing
	// them would duplicate the occurrence recorded in Structure.Objects.
	physical map[int64]*model.IndirectObject

	// loading marks objects currently being loaded, which is how a cycle
	// through an object stream container is detected.
	loading map[model.ObjectID]bool

	// decoded maps an encoded payload range to the source holding its
	// decoded bytes, so a stream is never decoded twice.
	decoded map[model.Span]model.SourceID

	// Running totals charged against Options.Limits for the whole session,
	// rather than per object, so that many small streams cannot together
	// exceed what one large stream is denied.
	decodedBytes int64
	xrefRecords  int
	xrefRanges   int

	// trailer is the effective trailer: the newest section's, which is the
	// one whose /Root governs the document.
	trailer model.Object
}

// Bytes returns a copy of the requested range.
//
// The span's Source selects what the offsets address: the file itself, or the
// decoded output of some stream. A copy is returned so that a caller cannot
// reach into the snapshot and mutate state other objects alias.
func (d *Document) Bytes(span model.Span) ([]byte, error) {
	source, ok := d.Sources[span.Source]
	if !ok || span.Start < 0 || span.End < span.Start || span.End > source.Size || uint64(span.End-span.Start) > uint64(^uint(0)>>1) {
		return nil, fmt.Errorf("invalid source span %+v", span)
	}
	data := make([]byte, int(span.End-span.Start))
	if len(data) == 0 {
		return data, nil
	}
	n, err := source.Reader.ReadAt(data, span.Start)
	if n != len(data) {
		if err == nil {
			err = io.ErrUnexpectedEOF
		}
		return nil, err
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return data, nil
}

// Catalog resolves the document catalog, the root of the object graph that
// every page is reached through. It is taken from the newest trailer's /Root.
func (d *Document) Catalog() (model.Object, error) {
	dict, ok := d.trailer.Value.(model.Dictionary)
	if !ok {
		return model.Object{}, errors.New("missing trailer dictionary")
	}
	root, err := dict.Get("Root")
	if err != nil {
		return model.Object{}, err
	}
	return d.ResolveObject(root)
}

// Resolve loads the object a reference points at and returns its body.
//
// It follows exactly one reference. Use ResolveObject when the target may
// itself be a reference.
func (d *Document) Resolve(ref model.Reference) (model.Object, error) {
	obj, err := d.Load(ref.ID)
	if err != nil {
		return model.Object{}, err
	}
	return obj.Body, nil
}

// ResolveObject follows a chain of indirect references to the value at its end,
// and returns a direct object unchanged.
//
// A file may legally point one reference at another, and a damaged or hostile
// one may point a reference back at itself, so the chain is both cycle-checked
// and depth-bounded.
func (d *Document) ResolveObject(object model.Object) (model.Object, error) {
	seen := make(map[model.ObjectID]bool)
	for depth := 0; depth < d.Options.Limits.MaxDepth; depth++ {
		ref, ok := object.Value.(model.Reference)
		if !ok {
			return object, nil
		}
		if seen[ref.ID] {
			return model.Object{}, fmt.Errorf("cyclic indirect reference %v", ref.ID)
		}
		seen[ref.ID] = true
		var err error
		object, err = d.Resolve(ref)
		if err != nil {
			return model.Object{}, err
		}
	}
	return model.Object{}, errors.New("indirect reference depth limit exceeded")
}

// dictInt reads an integer dictionary entry, resolving it first.
//
// The indirection matters: /Length in particular is very often written as a
// reference, because a writer does not know a stream's length until it has
// finished emitting it.
func (d *Document) dictInt(dict model.Dictionary, key model.Name) (int64, error) {
	o, err := dict.Get(key)
	if err != nil {
		return 0, err
	}
	o, err = d.ResolveObject(o)
	if err != nil {
		return 0, err
	}
	return model.Int(o)
}

// markRegion records that a range of the file belongs to kind.
//
// Regions tile the file without gaps or overlaps: it starts as one
// RegionUnknown span covering everything, and each call splits whatever it
// overlaps, replacing the covered part and keeping the fragments on either
// side. Whatever is still RegionUnknown at the end is input the reader never
// accounted for.
//
// Only the file has regions, so spans in a decoded source are ignored.
func (d *Document) markRegion(kind model.RegionKind, span model.Span) {
	if span.Source != 1 || span.Start >= span.End {
		return
	}
	var regions []model.FileRegion
	for _, r := range d.Structure.Regions {
		if r.Span.End <= span.Start || r.Span.Start >= span.End {
			regions = append(regions, r)
			continue
		}
		lo, hi := max(r.Span.Start, span.Start), min(r.Span.End, span.End)
		if r.Span.Start < lo {
			regions = append(regions, model.FileRegion{Kind: r.Kind, Span: model.Span{Source: 1, Start: r.Span.Start, End: lo}})
		}
		regions = append(regions, model.FileRegion{Kind: kind, Span: model.Span{Source: 1, Start: lo, End: hi}})
		if hi < r.Span.End {
			regions = append(regions, model.FileRegion{Kind: r.Kind, Span: model.Span{Source: 1, Start: hi, End: r.Span.End}})
		}
	}
	d.Structure.Regions = regions
}

// The doc* helpers scan the file's own framing — object headers, keywords and
// cross-reference tables — which is deliberately not done with the token
// scanner. That framing has to be read at byte offsets the xref supplies,
// often in files where those offsets are wrong, so it needs to fail locally
// rather than tokenise a whole region to discover a problem.

// docSpace reports whether c is PDF whitespace, NUL included.
func docSpace(c byte) bool { return c == 0 || c == 9 || c == 10 || c == 12 || c == 13 || c == 32 }

// skipDocSpace advances past whitespace and comments, which may be freely
// interleaved between the keywords that make up the file's framing.
func skipDocSpace(data []byte, pos int) int {
	for pos < len(data) {
		if docSpace(data[pos]) {
			pos++
			continue
		}
		if data[pos] == '%' {
			for pos < len(data) && data[pos] != '\r' && data[pos] != '\n' {
				pos++
			}
			continue
		}
		break
	}
	return pos
}

// docDelimiter reports whether c ends a bare word: whitespace or one of the
// self-delimiting characters.
func docDelimiter(c byte) bool { return docSpace(c) || bytes.IndexByte([]byte("()<>[]{}/%"), c) >= 0 }

// docWord reads the next bare word, such as obj, endstream or xref.
//
// It advances *pos past the word and returns the word's start offset, which
// callers use to report where a malformed construct began.
func docWord(data []byte, pos *int) (string, int, error) {
	*pos = skipDocSpace(data, *pos)
	start := *pos
	for *pos < len(data) && !docDelimiter(data[*pos]) {
		*pos++
	}
	if start == *pos {
		return "", start, fmt.Errorf("expected token at byte %d", start)
	}
	return string(data[start:*pos]), start, nil
}

// docUint reads the next word as an unsigned integer of at most bits wide,
// rejecting a value too large for the field it is destined for.
func docUint(data []byte, pos *int, bits int) (uint64, int, error) {
	s, start, err := docWord(data, pos)
	if err != nil {
		return 0, start, err
	}
	n, err := strconv.ParseUint(s, 10, bits)
	if err != nil {
		return 0, start, fmt.Errorf("invalid unsigned integer at byte %d: %w", start, err)
	}
	return n, start, nil
}

// RawObject returns the bytes an object was parsed from, including its
// delimiters, for callers that need the original syntax rather than the value.
func (d *Document) RawObject(object model.Object) ([]byte, error) { return d.Bytes(object.Span) }
