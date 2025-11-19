//go:build cgo

package filters

/*
#cgo pkg-config: jbig2dec
#include <stdint.h>
#include <stdlib.h>
#include <jbig2.h>

void goJBIG2Error(void *data, char *msg, Jbig2Severity severity, uint32_t seg_idx);

static inline void pdflib_jbig2_error_thunk(void *data, const char *msg, Jbig2Severity severity, uint32_t seg_idx) {
	goJBIG2Error(data, (char*)msg, severity, seg_idx);
}

static inline Jbig2Ctx* pdflib_jbig2_new_ctx(Jbig2GlobalCtx *global, void *user) {
	return jbig2_ctx_new(NULL, JBIG2_OPTIONS_EMBEDDED, global, pdflib_jbig2_error_thunk, user);
}
*/
import "C"

import (
	"context"
	"errors"
	"fmt"
	"runtime/cgo"
	"unsafe"
)

type jbig2ErrorState struct {
	message  string
	severity C.Jbig2Severity
}

//export goJBIG2Error
func goJBIG2Error(data unsafe.Pointer, msg *C.char, severity C.Jbig2Severity, segIdx C.uint32_t) {
	if data == nil || msg == nil {
		return
	}
	stateHandle := *(*cgo.Handle)(data)
	if state, ok := stateHandle.Value().(*jbig2ErrorState); ok {
		if state.message == "" || severity == C.JBIG2_SEVERITY_FATAL {
			state.message = C.GoString(msg)
			state.severity = severity
		}
	}
}

func decodeJBIG2Native(ctx context.Context, pageData, globalData []byte) ([]byte, error) {
	if len(pageData) == 0 {
		return nil, errors.New("JBIG2 stream empty")
	}

	var state jbig2ErrorState
	handle := cgo.NewHandle(&state)
	defer handle.Delete()

	handlePtr := C.malloc(C.size_t(unsafe.Sizeof(handle)))
	if handlePtr == nil {
		return nil, errors.New("allocate JBIG2 handle")
	}
	defer C.free(handlePtr)
	*(*cgo.Handle)(handlePtr) = handle

	globalCtx := (*C.Jbig2GlobalCtx)(nil)
	pageCtx := C.pdflib_jbig2_new_ctx(nil, handlePtr)
	if pageCtx == nil {
		return nil, errors.New("create JBIG2 context")
	}
	defer func() {
		if pageCtx != nil {
			C.jbig2_ctx_free(pageCtx)
		}
		if globalCtx != nil {
			C.jbig2_global_ctx_free(globalCtx)
		}
	}()

	if len(globalData) > 0 {
		if err := feedJBIG2Data(ctx, pageCtx, &state, globalData, "global"); err != nil {
			return nil, err
		}
		globalCtx = C.jbig2_make_global_ctx(pageCtx)
		pageCtx = C.pdflib_jbig2_new_ctx(globalCtx, handlePtr)
		if pageCtx == nil {
			return nil, errors.New("create JBIG2 page context")
		}
	}

	if err := feedJBIG2Data(ctx, pageCtx, &state, pageData, "page"); err != nil {
		return nil, err
	}

	if code := C.jbig2_complete_page(pageCtx); code < 0 {
		return nil, state.error("complete page")
	}

	img := C.jbig2_page_out(pageCtx)
	if img == nil {
		return nil, errors.New("no JBIG2 image produced")
	}
	defer C.jbig2_release_page(pageCtx, img)

	width := int(img.width)
	height := int(img.height)
	stride := int(img.stride)

	if err := validateNativeImageBounds(width, height); err != nil {
		return nil, err
	}
	if stride <= 0 {
		return nil, errors.New("invalid JBIG2 image dimensions")
	}
	rawLen := stride * height
	pix := C.GoBytes(unsafe.Pointer(img.data), C.int(rawLen))
	return jbig2MonochromeToNRGBA(width, height, stride, pix)
}

func feedJBIG2Data(ctx context.Context, cctx *C.Jbig2Ctx, state *jbig2ErrorState, data []byte, label string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(data) == 0 {
		return fmt.Errorf("JBIG2 %s stream empty", label)
	}
	rc := C.jbig2_data_in(cctx, (*C.uchar)(unsafe.Pointer(&data[0])), C.size_t(len(data)))
	if rc < 0 {
		return state.error(label)
	}
	return nil
}

func (s *jbig2ErrorState) error(label string) error {
	if s != nil && s.message != "" {
		return fmt.Errorf("JBIG2 %s decode failed: %s", label, s.message)
	}
	return fmt.Errorf("JBIG2 %s decode failed", label)
}
