package filters

import "errors"

var (
	errJBIG2NativeUnsupported = errors.New("jbig2 native decoder unavailable")
	jbig2NativeDecode         = decodeJBIG2Native
)
