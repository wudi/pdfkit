package contentstream

// TextRenderMode matches PDF text rendering modes set via Tr operator.
type TextRenderMode int

const (
	TextFill TextRenderMode = iota
	TextStroke
	TextFillStroke
	TextInvisible
	TextFillClip
	TextStrokeClip
	TextFillStrokeClip
	TextClip
)

// LineCap represents the line cap style (J operator).
type LineCap int

const (
	LineCapButt LineCap = iota
	LineCapRound
	LineCapSquare
)

// LineJoin represents the line join style (j operator).
type LineJoin int

const (
	LineJoinMiter LineJoin = iota
	LineJoinRound
	LineJoinBevel
)

// Path describes a graphics path made of subpaths.
type Path struct {
	Subpaths []Subpath
}

// Subpath describes a portion of a path.
type Subpath struct {
	Points []PathPoint
	Closed bool
}

// PathPoint identifies a path segment and its coordinates.
type PathPoint struct {
	X, Y                 float64
	Type                 PathPointType
	Control1X, Control1Y float64
	Control2X, Control2Y float64
}

// PathPointType enumerates path segment types.
type PathPointType int

const (
	PathMoveTo PathPointType = iota
	PathLineTo
	PathCurveTo
	PathClose
)
