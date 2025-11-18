//go:build !cgo

package filters

import (
	"context"
)

func decodeJBIG2Native(ctx context.Context, pageData, globalData []byte) ([]byte, error) {
	return nil, errJBIG2NativeUnsupported
}
