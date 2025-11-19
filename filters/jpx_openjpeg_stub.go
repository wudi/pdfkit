//go:build !openjpeg || !cgo

package filters

import "context"

func decodeJPXOpenJPEG(ctx context.Context, data []byte) ([]byte, error) {
	return nil, errJPXNativeUnsupported
}
