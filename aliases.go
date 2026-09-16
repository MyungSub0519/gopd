package gopd

// Code in this file re-exports the internal layer packages as the public API.
// The aliases keep the gopd package the single import for users while the
// implementation stays split by responsibility.

import (
	"github.com/MyungSub0519/gopd/internal/content"
	"github.com/MyungSub0519/gopd/internal/model"
	"github.com/MyungSub0519/gopd/internal/structure"
)

// --- Core PDF object and file-structure model (internal/model) ---

type (
	Array              = model.Array
	Boolean            = model.Boolean
	CompressedEntry    = model.CompressedEntry
	Derivation         = model.Derivation
	Diagnostic         = model.Diagnostic
	Dictionary         = model.Dictionary
	DictionaryEntry    = model.DictionaryEntry
	FileObjectOrigin   = model.FileObjectOrigin
	FileRegion         = model.FileRegion
	FileTail           = model.FileTail
	FreeEntry          = model.FreeEntry
	Header             = model.Header
	InUseEntry         = model.InUseEntry
	IndirectObject     = model.IndirectObject
	Integer            = model.Integer
	InvalidValue       = model.InvalidValue
	Limits             = model.Limits
	Matrix             = model.Matrix
	Name               = model.Name
	Null               = model.Null
	Object             = model.Object
	ObjectID           = model.ObjectID
	ObjectOrigin       = model.ObjectOrigin
	ObjectStreamOrigin = model.ObjectStreamOrigin
	PDFString          = model.PDFString
	Point              = model.Point
	Position           = model.Position
	Real               = model.Real
	Rect               = model.Rect
	Reference          = model.Reference
	RegionKind         = model.RegionKind
	SectionID          = model.SectionID
	Severity           = model.Severity
	Source             = model.Source
	SourceID           = model.SourceID
	Span               = model.Span
	Stream             = model.Stream
	StreamBoundary     = model.StreamBoundary
	StringForm         = model.StringForm
	Structure          = model.Structure
	Token              = model.Token
	TokenKind          = model.TokenKind
	Transform          = model.Transform
	TransformKind      = model.TransformKind
	UnknownXRefEntry   = model.UnknownXRefEntry
	Value              = model.Value
	Version            = model.Version
	XRefEntry          = model.XRefEntry
	XRefForm           = model.XRefForm
	XRefRange          = model.XRefRange
	XRefRecord         = model.XRefRecord
	XRefSection        = model.XRefSection
)

const (
	RegionFileTail       = model.RegionFileTail
	RegionHeader         = model.RegionHeader
	RegionIndirectObject = model.RegionIndirectObject
	RegionTrailer        = model.RegionTrailer
	RegionTrivia         = model.RegionTrivia
	RegionUnknown        = model.RegionUnknown
	RegionXRef           = model.RegionXRef
	SeverityError        = model.SeverityError
	SeverityInfo         = model.SeverityInfo
	SeverityWarning      = model.SeverityWarning
	StreamFromLength     = model.StreamFromLength
	StreamRecovered      = model.StreamRecovered
	StreamUnresolved     = model.StreamUnresolved
	StringHex            = model.StringHex
	StringLiteral        = model.StringLiteral
	TokenArrayClose      = model.TokenArrayClose
	TokenArrayOpen       = model.TokenArrayOpen
	TokenComment         = model.TokenComment
	TokenDictClose       = model.TokenDictClose
	TokenDictOpen        = model.TokenDictOpen
	TokenEOF             = model.TokenEOF
	TokenHexString       = model.TokenHexString
	TokenInteger         = model.TokenInteger
	TokenInvalid         = model.TokenInvalid
	TokenKeyword         = model.TokenKeyword
	TokenLiteralString   = model.TokenLiteralString
	TokenName            = model.TokenName
	TokenReal            = model.TokenReal
	TokenWhitespace      = model.TokenWhitespace
	TransformDecrypt     = model.TransformDecrypt
	TransformFilter      = model.TransformFilter
	XRefStream           = model.XRefStream
	XRefTable            = model.XRefTable
)

var (
	ErrMissingKey = model.ErrMissingKey
)

// --- Document-level object access (internal/structure) ---

type (
	Document    = structure.Document
	ReadOptions = structure.ReadOptions
)

// --- Semantic content model (internal/content) ---

type (
	Annotation          = content.Annotation
	CMap                = content.CMap
	ClipPath            = content.ClipPath
	CodeSpace           = content.CodeSpace
	Color               = content.Color
	DetailedGraphic     = content.DetailedGraphic
	DetailedImage       = content.DetailedImage
	DetailedPDF         = content.DetailedPDF
	DetailedPage        = content.DetailedPage
	DetailedPathSegment = content.DetailedPathSegment
	DetailedText        = content.DetailedText
	ElementKind         = content.ElementKind
	ElementRef          = content.ElementRef
	ElementSource       = content.ElementSource
	Font                = content.Font
	FontInfo            = content.FontInfo
	FormCall            = content.FormCall
	Glyph               = content.Glyph
	Graphic             = content.Graphic
	GraphicsState       = content.GraphicsState
	Image               = content.Image
	ImageInfo           = content.ImageInfo
	ImageResource       = content.ImageResource
	Operation           = content.Operation
	PDF                 = content.PDF
	Page                = content.Page
	PaintStyle          = content.PaintStyle
	ParseDiagnostic     = content.ParseDiagnostic
	PathSegment         = content.PathSegment
	Text                = content.Text
)

const (
	ElementGraphic = content.ElementGraphic
	ElementImage   = content.ElementImage
	ElementText    = content.ElementText
)
