package filters

import "fmt"

const (
	// maxNativeImageDimension caps width/height for native decoders to avoid
	// excessive allocations when corrupted PDFs lie about image sizes.
	maxNativeImageDimension = 32768
	// maxNativeImagePixels bounds the total pixel count (roughly 64MP) which keeps
	// RGBA buffers under 256 MB and prevents resource exhaustion.
	maxNativeImagePixels int64 = 64 * 1024 * 1024
)

func validateNativeImageBounds(width, height int) error {
	if width <= 0 || height <= 0 {
		return fmt.Errorf("image bounds invalid (%d x %d)", width, height)
	}
	if width > maxNativeImageDimension || height > maxNativeImageDimension {
		return fmt.Errorf("image dimension exceeds limit (%d x %d)", width, height)
	}
	pixels := int64(width) * int64(height)
	if pixels > maxNativeImagePixels {
		return fmt.Errorf("image pixel count %d exceeds limit %d", pixels, maxNativeImagePixels)
	}
	return nil
}
