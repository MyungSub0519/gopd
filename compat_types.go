package gopd

// Page is a legacy page view retained for source compatibility.
// Current basic results group Texts and Graphics directly by page.
// Complete reports interpreter diagnostics, not full rendering support.
//
// Deprecated: Use PDF.Details().Pages and DetailedPage for page metadata.
type Page struct {
	Index    int
	MediaBox Rect
	CropBox  Rect
	Rotate   int
	UserUnit float64
	Items    []ElementRef
	Complete bool
}

// Image is a legacy basic image placement retained for source compatibility.
//
// Deprecated: Use PDF.Details().Images and DetailedImage for image placements.
type Image struct {
	Page     int
	Resource *ImageInfo
	Matrix   Matrix
	Style    PaintStyle
}

// ImageInfo is legacy image metadata retained for source compatibility.
// Width and Height describe pixels, not page-space display dimensions.
// ColorSpace is a name or family; an empty value means no name was available,
// including image masks. Color-space parameters belong to detailed resources.
//
// Deprecated: Use PDF.Details().ImageResources and ImageResource for image data.
type ImageInfo struct {
	Width, Height    int
	BitsPerComponent int
	ColorSpace       Name
	ImageMask        bool
}

// ParseDiagnostic is a legacy compact diagnostic retained for source compatibility.
// Page is zero-based, or -1 for a document-wide issue. Byte locations belong
// to detailed Diagnostic spans.
//
// Deprecated: Use Diagnostic in PDF.Details().Diagnostics and Structure.Diagnostics.
type ParseDiagnostic struct {
	Page     int
	Severity Severity
	Code     string
	Message  string
}
