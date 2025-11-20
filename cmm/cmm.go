package cmm

// Profile represents a color profile (e.g., ICC).
type Profile interface {
	// Name returns the profile description or name.
	Name() string
	// ColorSpace returns the color space signature (e.g., "RGB ", "CMYK").
	ColorSpace() string
	// Class returns the profile class (e.g., "mntr", "prtr").
	Class() string
	// Data returns the raw profile bytes.
	Data() []byte
}

// Transform represents a color transformation between two profiles.
type Transform interface {
	// Convert transforms a color value from source to destination space.
	Convert(src []float64) ([]float64, error)
}

// Factory creates profiles and transforms.
type Factory interface {
	NewProfile(data []byte) (Profile, error)
	NewTransform(src, dst Profile, intent RenderingIntent) (Transform, error)
}

// RenderingIntent specifies the rendering intent for color conversion.
type RenderingIntent int

const (
	IntentPerceptual RenderingIntent = iota
	IntentRelativeColorimetric
	IntentSaturation
	IntentAbsoluteColorimetric
)
