//go:build openjpeg && cgo

package filters

/*
#cgo pkg-config: libopenjp2
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#ifndef __has_include
#define __has_include(x) 0
#endif
#if __has_include(<openjpeg-2.5/openjpeg.h>)
#include <openjpeg-2.5/openjpeg.h>
#elif __has_include(<openjpeg-2.4/openjpeg.h>)
#include <openjpeg-2.4/openjpeg.h>
#elif __has_include(<openjpeg-2.3/openjpeg.h>)
#include <openjpeg-2.3/openjpeg.h>
#else
#include <openjpeg.h>
#endif

typedef struct {
	uint8_t *data;
	size_t length;
	size_t offset;
} pdflib_jpx_buffer;

static void pdflib_jpx_buffer_free(void *user_data) {
	pdflib_jpx_buffer *buffer = (pdflib_jpx_buffer*)user_data;
	if (!buffer) {
		return;
	}
	if (buffer->data) {
		free(buffer->data);
	}
	free(buffer);
}

static pdflib_jpx_buffer* pdflib_jpx_buffer_from_cmem(uint8_t *data, size_t len) {
	pdflib_jpx_buffer *buffer = (pdflib_jpx_buffer*)malloc(sizeof(pdflib_jpx_buffer));
	if (!buffer) {
		return NULL;
	}
	buffer->data = data;
	buffer->length = len;
	buffer->offset = 0;
	return buffer;
}

static OPJ_SIZE_T pdflib_jpx_stream_read(void *p_buffer, OPJ_SIZE_T nb_bytes, void *p_user_data) {
	pdflib_jpx_buffer *buffer = (pdflib_jpx_buffer*)p_user_data;
	if (!buffer || nb_bytes == 0) {
		return 0;
	}
	size_t remaining = 0;
	if (buffer->length > buffer->offset) {
		remaining = buffer->length - buffer->offset;
	}
	if ((size_t)nb_bytes > remaining) {
		nb_bytes = (OPJ_SIZE_T)remaining;
	}
	if (nb_bytes > 0) {
		memcpy(p_buffer, buffer->data + buffer->offset, (size_t)nb_bytes);
		buffer->offset += (size_t)nb_bytes;
	}
	return nb_bytes;
}

static OPJ_OFF_T pdflib_jpx_stream_skip(OPJ_OFF_T nb_bytes, void *p_user_data) {
	pdflib_jpx_buffer *buffer = (pdflib_jpx_buffer*)p_user_data;
	if (!buffer || nb_bytes <= 0) {
		return 0;
	}
	size_t available = 0;
	if (buffer->length > buffer->offset) {
		available = buffer->length - buffer->offset;
	}
	size_t request = (size_t)nb_bytes;
	if (request > available) {
		request = available;
	}
	buffer->offset += request;
	return (OPJ_OFF_T)request;
}

static OPJ_BOOL pdflib_jpx_stream_seek(OPJ_OFF_T nb_bytes, void *p_user_data) {
	pdflib_jpx_buffer *buffer = (pdflib_jpx_buffer*)p_user_data;
	if (!buffer || nb_bytes < 0) {
		return OPJ_FALSE;
	}
	size_t target = (size_t)nb_bytes;
	if (target > buffer->length) {
		return OPJ_FALSE;
	}
	buffer->offset = target;
	return OPJ_TRUE;
}

static opj_stream_t* pdflib_jpx_stream_create_reader(pdflib_jpx_buffer *buffer) {
	if (!buffer) {
		return NULL;
	}
	opj_stream_t *stream = opj_stream_create(OPJ_J2K_STREAM_CHUNK_SIZE, OPJ_TRUE);
	if (!stream) {
		return NULL;
	}
	opj_stream_set_user_data(stream, buffer, pdflib_jpx_buffer_free);
	opj_stream_set_user_data_length(stream, buffer->length);
	opj_stream_set_read_function(stream, pdflib_jpx_stream_read);
	opj_stream_set_skip_function(stream, pdflib_jpx_stream_skip);
	opj_stream_set_seek_function(stream, pdflib_jpx_stream_seek);
	return stream;
}

void goJPXNativeLog(void *handle_ptr, const char *message);

static void pdflib_jpx_error_callback(const char *msg, void *client_data) {
	goJPXNativeLog(client_data, msg);
}

static opj_event_mgr_t* pdflib_jpx_create_event_mgr(void) {
	opj_event_mgr_t *mgr = (opj_event_mgr_t*)malloc(sizeof(opj_event_mgr_t));
	if (!mgr) {
		return NULL;
	}
	memset(mgr, 0, sizeof(opj_event_mgr_t));
	mgr->error_handler = pdflib_jpx_error_callback;
	return mgr;
}

static void pdflib_jpx_install_event_mgr(opj_codec_t *codec, opj_event_mgr_t *mgr, void *handle_ptr) {
	if (!codec || !mgr) {
		return;
	}
	opj_set_event_mgr(codec, mgr, handle_ptr);
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

type jpxNativeState struct{ message string }

//export goJPXNativeLog
func goJPXNativeLog(handlePtr unsafe.Pointer, message *C.char) {
	if handlePtr == nil || message == nil {
		return
	}
	handle := *(*cgo.Handle)(handlePtr)
	if state, ok := handle.Value().(*jpxNativeState); ok {
		if state.message == "" {
			state.message = C.GoString(message)
		}
	}
}

func (s *jpxNativeState) error(label string) error {
	if s != nil && s.message != "" {
		return fmt.Errorf("jpx %s decode failed: %s", label, s.message)
	}
	return fmt.Errorf("jpx %s decode failed", label)
}

func decodeJPXOpenJPEG(ctx context.Context, data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("JPX stream empty")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	primary := C.OPJ_CODEC_JP2
	if looksLikeJP2Codestream(data) {
		primary = C.OPJ_CODEC_J2K
	}
	pix, err := jpxDecodeWithCodec(ctx, data, primary)
	if err == nil {
		return pix, nil
	}
	if primary == C.OPJ_CODEC_J2K {
		return nil, err
	}
	fallback, fallbackErr := jpxDecodeWithCodec(ctx, data, C.OPJ_CODEC_J2K)
	if fallbackErr == nil {
		return fallback, nil
	}
	return nil, err
}

func looksLikeJP2Codestream(data []byte) bool {
	return len(data) > 1 && data[0] == 0xFF && data[1] == 0x4F
}

func jpxDecodeWithCodec(ctx context.Context, data []byte, format C.OPJ_CODEC_FORMAT) ([]byte, error) {
	cBuf := C.CBytes(data)
	if cBuf == nil && len(data) > 0 {
		return nil, errors.New("allocate JPX buffer")
	}
	buffer := C.pdflib_jpx_buffer_from_cmem((*C.uint8_t)(cBuf), C.size_t(len(data)))
	if buffer == nil {
		if cBuf != nil {
			C.free(cBuf)
		}
		return nil, errors.New("initialize JPX buffer")
	}
	stream := C.pdflib_jpx_stream_create_reader(buffer)
	if stream == nil {
		return nil, errors.New("create JPX stream")
	}
	defer C.opj_stream_destroy(stream)

	codec := C.opj_create_decompress(format)
	if codec == nil {
		return nil, errors.New("create JPX codec")
	}
	defer C.opj_destroy_codec(codec)

	var state jpxNativeState
	handle := cgo.NewHandle(&state)
	defer handle.Delete()

	handlePtr := C.malloc(C.size_t(unsafe.Sizeof(handle)))
	if handlePtr == nil {
		return nil, errors.New("allocate JPX handle")
	}
	defer C.free(handlePtr)
	*(*cgo.Handle)(handlePtr) = handle

	eventMgr := C.pdflib_jpx_create_event_mgr()
	if eventMgr == nil {
		return nil, errors.New("create JPX event manager")
	}
	defer C.free(unsafe.Pointer(eventMgr))
	C.pdflib_jpx_install_event_mgr(codec, eventMgr, handlePtr)

	var params C.opj_dparameters_t
	C.opj_set_default_decoder_parameters(&params)

	if C.opj_setup_decoder(codec, &params) == 0 {
		return nil, state.error("setup")
	}

	var img *C.opj_image_t
	if C.opj_read_header(stream, codec, &img) == 0 || img == nil {
		return nil, state.error("header")
	}
	defer func() {
		if img != nil {
			C.opj_image_destroy(img)
		}
	}()

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if C.opj_decode(codec, stream, img) == 0 {
		return nil, state.error("decode")
	}
	if C.opj_end_decompress(codec, stream) == 0 {
		return nil, state.error("finalize")
	}

	return convertOPJImage(img)
}

func convertOPJImage(img *C.opj_image_t) ([]byte, error) {
	if img == nil {
		return nil, errors.New("nil JPX image")
	}
	width := int(img.x1 - img.x0)
	height := int(img.y1 - img.y0)
	if err := validateNativeImageBounds(width, height); err != nil {
		return nil, err
	}
	pixelCount := int64(width) * int64(height)
	const maxInt = int64(^uint(0) >> 1)
	if pixelCount <= 0 || pixelCount > maxInt {
		return nil, errors.New("JPX image exceeds supported size")
	}
	components, err := buildJPXComponents(img, width, height)
	if err != nil {
		return nil, err
	}
	space := mapJPXColorSpace(img.color_space, len(components))
	return composeJPXPixelBuffer(components, width, height, space)
}

func buildJPXComponents(img *C.opj_image_t, width, height int) ([]jpxComponent, error) {
	compCount := int(img.numcomps)
	if compCount == 0 {
		return nil, errors.New("JPX image missing components")
	}
	components := unsafe.Slice(img.comps, compCount)
	result := make([]jpxComponent, compCount)
	for i, comp := range components {
		w := int(comp.w)
		h := int(comp.h)
		if w != width || h != height {
			return nil, fmt.Errorf("JPX component %d uses unsupported subsampling (%dx%d vs %dx%d)", i, w, h, width, height)
		}
		count := w * h
		if count == 0 {
			return nil, fmt.Errorf("JPX component %d empty", i)
		}
		src := unsafe.Slice(comp.data, count)
		values := make([]int32, count)
		for idx := 0; idx < count; idx++ {
			values[idx] = int32(src[idx])
		}
		result[i] = jpxComponent{
			samples:   values,
			precision: int(comp.prec),
			signed:    comp.sgnd != 0,
		}
	}
	return result, nil
}

func mapJPXColorSpace(space C.OPJ_COLOR_SPACE, compCount int) jpxColorSpace {
	switch space {
	case C.OPJ_CLRSPC_GRAY:
		return jpxColorSpaceGray
	case C.OPJ_CLRSPC_SRGB:
		return jpxColorSpaceRGB
	case C.OPJ_CLRSPC_SYCC, C.OPJ_CLRSPC_EYCC:
		return jpxColorSpaceSYCC
	case C.OPJ_CLRSPC_CMYK:
		return jpxColorSpaceCMYK
	default:
		if compCount == 1 {
			return jpxColorSpaceGray
		}
		if compCount == 4 {
			return jpxColorSpaceRGB
		}
		return jpxColorSpaceRGB
	}
}
