package ocr

import "context"

// ImageFormat identifies the content type of an OCR input image.
type ImageFormat string

const (
	ImageFormatPNG  ImageFormat = "image/png"
	ImageFormatJPEG ImageFormat = "image/jpeg"
	ImageFormatTIFF ImageFormat = "image/tiff"
)

// Region describes a rectangular area in pixel coordinates with the origin in
// the upper-left corner of the image.
type Region struct {
	X      float64
	Y      float64
	Width  float64
	Height float64
}

// IsEmpty reports whether the region has non-positive dimensions.
func (r Region) IsEmpty() bool { return r.Width <= 0 || r.Height <= 0 }

// Input encapsulates a single image submitted for OCR.
type Input struct {
	// ID is an optional caller-provided identifier that is echoed back in the
	// corresponding Result.
	ID string
	// Image is the encoded image payload in the format specified by Format.
	Image []byte
	// Format declares the image content type (e.g., image/png).
	Format ImageFormat
	// PageIndex links the input back to the zero-based PDF page index where the
	// image originated.
	PageIndex int
	// DPI carries the effective dots-per-inch for the image. Providers such as
	// Tesseract use this for scaling and layout heuristics; zero means unknown.
	DPI int
	// Languages is a list of BCP-47 language hints (e.g., "eng", "deu") that
	// providers can use to select trained data.
	Languages []string
	// Region restricts recognition to a subsection of the image. Nil means the
	// full image should be processed.
	Region *Region
	// Metadata allows callers to pass through engine-specific knobs (e.g.,
	// "psm" for Tesseract) without hard-coding them into the API surface.
	Metadata map[string]string
}

// TextWord represents a single recognized token.
type TextWord struct {
	Text       string
	Bounds     Region
	Confidence float64
}

// TextLine groups words that share a baseline.
type TextLine struct {
	Text       string
	Bounds     Region
	Words      []TextWord
	Confidence float64
}

// TextBlock aggregates lines that form a logical block (paragraph, heading, etc).
type TextBlock struct {
	Text       string
	Bounds     Region
	Lines      []TextLine
	Confidence float64
}

// Result captures OCR output for a single input image.
type Result struct {
	// InputID mirrors the Input.ID that produced this result.
	InputID string
	// PlainText contains the linearized text extracted from the image.
	PlainText string
	// Blocks carries the structured layout with positional metadata.
	Blocks []TextBlock
	// Language indicates the dominant language detected, if known.
	Language string
}

// Engine is the simplest OCR provider contract: one image in, one result out.
type Engine interface {
	Name() string
	Recognize(ctx context.Context, input Input) (Result, error)
}

// BatchEngine handles multiple images in a single call, enabling providers that
// amortize setup costs or remote round-trips.
type BatchEngine interface {
	Engine
	RecognizeBatch(ctx context.Context, inputs []Input) ([]Result, error)
}

// JobState models the lifecycle of an asynchronous OCR request.
type JobState string

const (
	JobStatePending   JobState = "pending"
	JobStateRunning   JobState = "running"
	JobStateSucceeded JobState = "succeeded"
	JobStateFailed    JobState = "failed"
	JobStateCanceled  JobState = "canceled"
)

// JobStatus reports incremental progress for long-running jobs.
type JobStatus struct {
	State    JobState
	Message  string
	Progress float64
}

// Job represents an asynchronous OCR submission that can be polled or canceled.
type Job interface {
	ID() string
	Status(ctx context.Context) (JobStatus, error)
	Results(ctx context.Context) ([]Result, error)
	Cancel(ctx context.Context) error
}

// AsyncEngine submits OCR requests that may complete later (useful for remote
// providers that process batches asynchronously).
type AsyncEngine interface {
	Name() string
	Start(ctx context.Context, inputs []Input) (Job, error)
}
