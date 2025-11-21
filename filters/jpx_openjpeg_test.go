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

static int pdfkit_jpx_test_encode_rgba(const char *path, const uint8_t *pixels, int width, int height) {
	opj_cparameters_t params;
	opj_set_default_encoder_parameters(&params);
	params.tcp_numlayers = 1;
	params.cp_disto_alloc = 1;
	params.tcp_rates[0] = 0;
	params.irreversible = 0;

	opj_image_cmptparm_t cmptparm[4];
	memset(&cmptparm, 0, sizeof(cmptparm));
	for (int i = 0; i < 4; i++) {
		cmptparm[i].prec = 8;
		cmptparm[i].bpp = 8;
		cmptparm[i].sgnd = OPJ_FALSE;
		cmptparm[i].dx = 1;
		cmptparm[i].dy = 1;
		cmptparm[i].w = width;
		cmptparm[i].h = height;
	}
	opj_image_t *image = opj_image_create(4, cmptparm, OPJ_CLRSPC_SRGB);
	if (!image) {
		return 0;
	}
	image->x1 = width;
	image->y1 = height;
	int count = width * height;
	for (int i = 0; i < count; i++) {
		image->comps[0].data[i] = pixels[i*4 + 0];
		image->comps[1].data[i] = pixels[i*4 + 1];
		image->comps[2].data[i] = pixels[i*4 + 2];
		image->comps[3].data[i] = pixels[i*4 + 3];
	}

	opj_codec_t *codec = opj_create_compress(OPJ_CODEC_JP2);
	if (!codec) {
		opj_image_destroy(image);
		return 0;
	}
	opj_stream_t *stream = opj_stream_create_default_file_stream(path, OPJ_FALSE);
	if (!stream) {
		opj_destroy_codec(codec);
		opj_image_destroy(image);
		return 0;
	}
	int ok = opj_setup_encoder(codec, &params, image) &&
		opj_start_compress(codec, image, stream) &&
		opj_encode(codec, stream) &&
		opj_end_compress(codec, stream);
	opj_stream_destroy(stream);
	opj_destroy_codec(codec);
	opj_image_destroy(image);
	return ok;
}
*/
import "C"

import (
	"bytes"
	"context"
	"os"
	"testing"
	"unsafe"
)

func TestJPXOpenJPEGNativeDecode(t *testing.T) {
	rgba := []byte{
		255, 0, 0, 255,
		0, 255, 0, 255,
		0, 0, 255, 255,
		255, 255, 255, 255,
	}
	encoded := encodeTestJPX(t, rgba, 2, 2)
	decoded, err := decodeJPXOpenJPEG(context.Background(), encoded)
	if err != nil {
		t.Fatalf("native decode failed: %v", err)
	}
	if !bytes.Equal(decoded, rgba) {
		t.Fatalf("decoded pixels mismatch: %v", decoded)
	}

	pipeline := NewPipeline([]Decoder{NewJPXDecoder()}, Limits{})
	out, err := pipeline.Decode(context.Background(), encoded, []string{"JPXDecode"}, nil)
	if err != nil {
		t.Fatalf("pipeline decode failed: %v", err)
	}
	if !bytes.Equal(out, rgba) {
		t.Fatalf("pipeline output mismatch: %v", out)
	}
}

func encodeTestJPX(t *testing.T, rgba []byte, width, height int) []byte {
	t.Helper()
	file, err := os.CreateTemp("", "pdfkit-jpx-encode-*.jp2")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	path := file.Name()
	file.Close()
	defer os.Remove(path)
	var pixelPtr *C.uint8_t
	if len(rgba) > 0 {
		pixelPtr = (*C.uint8_t)(unsafe.Pointer(&rgba[0]))
	}
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))
	if C.pdfkit_jpx_test_encode_rgba(cPath, pixelPtr, C.int(width), C.int(height)) == 0 {
		t.Fatalf("openjpeg encode failed")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read temp: %v", err)
	}
	return data
}
