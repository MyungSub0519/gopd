package gopd

import (
	"github.com/MyungSub0519/gopd/internal/common/document"
	"github.com/MyungSub0519/gopd/internal/common/pdfmodel"
	"github.com/MyungSub0519/gopd/internal/parser"
)

// Document owns an immutable input snapshot and lazily decoded sources.
// Treat returned objects/slices as read-only. Lazy methods are not concurrent-safe.
type Document = document.Document

// ReadOptions bounds input, recursive parsing, xrefs and decoded stream data.
// Zero fields use defaults. Negative values are invalid.
type ReadOptions = document.ReadOptions

type Point = pdfmodel.Point

type Rect = pdfmodel.Rect

// Matrix is [a b c d e f], mapping (x,y) to (a*x+c*y+e,b*x+d*y+f).
// Coordinates use PDF user space; page rotation is retained separately.
type Matrix = pdfmodel.Matrix

type SourceID = pdfmodel.SourceID

// Source == 0은 위치 없음. 오프셋은 항상 바이트 단위다.
type Position = pdfmodel.Position

// [Start, End): Start를 포함하고 End를 제외한다.
type Span = pdfmodel.Span

type Source = pdfmodel.Source

type Derivation = pdfmodel.Derivation

type TransformKind = pdfmodel.TransformKind

type Transform = pdfmodel.Transform

const (
	TransformFilter  = pdfmodel.TransformFilter
	TransformDecrypt = pdfmodel.TransformDecrypt
)

type Value = pdfmodel.Value

type Object = pdfmodel.Object

type Null = pdfmodel.Null

type Boolean = pdfmodel.Boolean

// PDF 숫자 문법을 검증한 십진 표기. 자동으로 float64로 바꾸지 않는다.
type Integer = pdfmodel.Integer

type Real = pdfmodel.Real

// '/'를 제외하고 #xx escape를 해제한 바이트열. UTF-8을 보장하지 않는다.
type Name = pdfmodel.Name

type StringForm = pdfmodel.StringForm

type PDFString = pdfmodel.PDFString

type Array = pdfmodel.Array

type DictionaryEntry = pdfmodel.DictionaryEntry

type Dictionary = pdfmodel.Dictionary

type ObjectID = pdfmodel.ObjectID

type Reference = pdfmodel.Reference

// 손상된 구문을 표현하는 분석기 전용 값. PDF의 null과 구별한다.
type InvalidValue = pdfmodel.InvalidValue

const (
	StringLiteral = pdfmodel.StringLiteral
	StringHex     = pdfmodel.StringHex
)

type StreamBoundary = pdfmodel.StreamBoundary

type Stream = pdfmodel.Stream

const (
	StreamUnresolved = pdfmodel.StreamUnresolved
	StreamFromLength = pdfmodel.StreamFromLength
	StreamRecovered  = pdfmodel.StreamRecovered
)

type ObjectOrigin = pdfmodel.ObjectOrigin

type FileObjectOrigin = pdfmodel.FileObjectOrigin

type ObjectStreamOrigin = pdfmodel.ObjectStreamOrigin

type IndirectObject = pdfmodel.IndirectObject

type XRefEntry = pdfmodel.XRefEntry

type FreeEntry = pdfmodel.FreeEntry

type InUseEntry = pdfmodel.InUseEntry

type CompressedEntry = pdfmodel.CompressedEntry

type UnknownXRefEntry = pdfmodel.UnknownXRefEntry

type XRefRecord = pdfmodel.XRefRecord

type XRefRange = pdfmodel.XRefRange

type XRefForm = pdfmodel.XRefForm

type SectionID = pdfmodel.SectionID

type XRefSection = pdfmodel.XRefSection

const (
	XRefTable  = pdfmodel.XRefTable
	XRefStream = pdfmodel.XRefStream
)

type TokenKind = pdfmodel.TokenKind

type Token = pdfmodel.Token

const (
	TokenInvalid       = pdfmodel.TokenInvalid
	TokenEOF           = pdfmodel.TokenEOF
	TokenWhitespace    = pdfmodel.TokenWhitespace
	TokenComment       = pdfmodel.TokenComment
	TokenInteger       = pdfmodel.TokenInteger
	TokenReal          = pdfmodel.TokenReal
	TokenName          = pdfmodel.TokenName
	TokenLiteralString = pdfmodel.TokenLiteralString
	TokenHexString     = pdfmodel.TokenHexString
	TokenArrayOpen     = pdfmodel.TokenArrayOpen
	TokenArrayClose    = pdfmodel.TokenArrayClose
	TokenDictOpen      = pdfmodel.TokenDictOpen
	TokenDictClose     = pdfmodel.TokenDictClose
	TokenKeyword       = pdfmodel.TokenKeyword
)

type Version = pdfmodel.Version

type Header = pdfmodel.Header

type FileTail = pdfmodel.FileTail

type RegionKind = pdfmodel.RegionKind

type FileRegion = pdfmodel.FileRegion

type Severity = pdfmodel.Severity

type Diagnostic = pdfmodel.Diagnostic

type Limits = pdfmodel.Limits

type Structure = pdfmodel.Structure

const (
	RegionUnknown        = pdfmodel.RegionUnknown
	RegionHeader         = pdfmodel.RegionHeader
	RegionTrivia         = pdfmodel.RegionTrivia
	RegionIndirectObject = pdfmodel.RegionIndirectObject
	RegionXRef           = pdfmodel.RegionXRef
	RegionTrailer        = pdfmodel.RegionTrailer
	RegionFileTail       = pdfmodel.RegionFileTail
)

const (
	SeverityInfo    = pdfmodel.SeverityInfo
	SeverityWarning = pdfmodel.SeverityWarning
	SeverityError   = pdfmodel.SeverityError
)

// ErrMissingKey identifies absent dictionary keys. Use errors.Is to distinguish
// a missing key from a duplicate key or another lookup failure.
var ErrMissingKey = pdfmodel.ErrMissingKey

// ErrLimit identifies exhausted resource budgets. Use errors.Is even when
// parsing also returns a partial result.
var ErrLimit = pdfmodel.ErrLimit

// PDF is the basic result returned by ParsePDF. The outer slice index is the
// zero-based page index; empty pages contain non-nil empty slices. Each inner
// slice follows content execution order for its kind, not reading order.
// Treat results and shared resources as read-only. Detailed analysis is retained
// privately and excluded from JSON encoding.
type PDF = parser.PDF

// Text is one text-show operation. Matrix is in unrotated page user space,
// before font scaling. Glyph positions and source bytes are available in Details.
type Text = parser.Text

// Graphic is one path painting operation; it is not a whole chart or figure.
type Graphic = parser.Graphic

// PathSegment points are already transformed into unrotated page user space.
type PathSegment = parser.PathSegment

// FontInfo contains display metadata, shared by original font resource identity.
// FontSize belongs to Text because the same font can be used at different sizes.
type FontInfo = parser.FontInfo

type CodeSpace = parser.CodeSpace

type CMap = parser.CMap

// DetailedPDF is a semantic snapshot. Slices and the underlying Document are read-only
// by convention. Elements follow content execution order, not reading order.
type DetailedPDF = parser.DetailedPDF

type ElementKind = parser.ElementKind

const (
	ElementText    = parser.ElementText
	ElementGraphic = parser.ElementGraphic
	ElementImage   = parser.ElementImage
)

type ElementRef = parser.ElementRef

type DetailedPage = parser.DetailedPage

type FormCall = parser.FormCall

type ElementSource = parser.ElementSource

type Operation = parser.Operation

type DetailedPathSegment = parser.DetailedPathSegment

type DetailedGraphic = parser.DetailedGraphic

type Glyph = parser.Glyph

type DetailedText = parser.DetailedText

// DetailedImage is one execution of an image XObject; bytes belong to ImageResource.
type DetailedImage = parser.DetailedImage

type ImageResource = parser.ImageResource

type Annotation = parser.Annotation

// ContentKind selects independently emitted content categories.
type ContentKind = parser.ContentKind

const (
	ContentText        = parser.ContentText
	ContentGraphics    = parser.ContentGraphics
	ContentImages      = parser.ContentImages
	ContentAnnotations = parser.ContentAnnotations
	ContentAll         = parser.ContentAll
)

// ParseOptions controls work and retained output. Zero options extract Unicode
// text and compact font metadata. Glyphs requires text and enables Positions.
// Positions use unrotated page user space, as in the detailed API.
type ParseOptions = parser.ParseOptions

// Result contains only requested output, in content execution order within
// each kind. It does not reconstruct reading order or render pixels. Treat
// results as read-only. Without Provenance, no Document or DetailedPDF is retained.
type Result = parser.Result

// ExtractedPage groups selected elements. An omitted category was not requested
// or has no elements; Result.Content distinguishes those cases.
type ExtractedPage = parser.ExtractedPage

// ExtractedText is one text-show operation, with independently selected details.
type ExtractedText = parser.ExtractedText

// TextPosition describes text before the show operation, before font scaling.
type TextPosition = parser.TextPosition

// ExtractedGraphic is a painted path, not a complete chart or figure. Geometry
// is always in unrotated page user space, independent of Positions.
type ExtractedGraphic = parser.ExtractedGraphic

// ExtractedImage is one placement of a shared image resource.
type ExtractedImage = parser.ExtractedImage

// ExtractedImageResource retains image metadata, not decoded pixels. ColorSpace
// preserves the PDF value (including complex color-space parameters). Object is
// present only with Provenance; its Stream can be used with Result.Document.
type ExtractedImageResource = parser.ExtractedImageResource

// ExtractedAnnotation contains annotation metadata; appearance streams are not executed.
type ExtractedAnnotation = parser.ExtractedAnnotation

// ResultDiagnostic reports a requested interpretation limitation. Page is
// zero based, or -1 for a document-level issue. Span needs Provenance to resolve.
type ResultDiagnostic = parser.ResultDiagnostic

type Font = parser.Font

type Color = parser.Color

// PaintStyle omits transformation and clipping geometry. Clipped indicates that
// clipping paths exist in the detailed state. Complete retains the interpreter's
// effect support flag; neither flag promises a complete rendering description.
type PaintStyle = parser.PaintStyle

type GraphicsState = parser.GraphicsState

type ClipPath = parser.ClipPath
