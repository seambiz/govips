package vips

// #cgo pkg-config: vips
// #include "foreign.h"
import "C"
import (
	"bytes"
	"encoding/xml"
	"fmt"
	"image/png"
	"math"
	"runtime"
	"unsafe"

	"golang.org/x/image/bmp"
	"golang.org/x/net/html/charset"
)

// ImageType represents an image type
type ImageType int

// ImageType enum
const (
	ImageTypeUnknown ImageType = C.UNKNOWN
	ImageTypeGIF     ImageType = C.GIF
	ImageTypeJPEG    ImageType = C.JPEG
	ImageTypeMagick  ImageType = C.MAGICK
	ImageTypePDF     ImageType = C.PDF
	ImageTypePNG     ImageType = C.PNG
	ImageTypeSVG     ImageType = C.SVG
	ImageTypeTIFF    ImageType = C.TIFF
	ImageTypeWEBP    ImageType = C.WEBP
	ImageTypeHEIF    ImageType = C.HEIF
	ImageTypeBMP     ImageType = C.BMP
)

var imageTypeExtensionMap = map[ImageType]string{
	ImageTypeGIF:    ".gif",
	ImageTypeJPEG:   ".jpeg",
	ImageTypeMagick: ".magick",
	ImageTypePDF:    ".pdf",
	ImageTypePNG:    ".png",
	ImageTypeSVG:    ".svg",
	ImageTypeTIFF:   ".tiff",
	ImageTypeWEBP:   ".webp",
	ImageTypeHEIF:   ".heic",
	ImageTypeBMP:    ".bmp",
}

// ImageTypes defines the various image types supported by govips
var ImageTypes = map[ImageType]string{
	ImageTypeGIF:    "gif",
	ImageTypeJPEG:   "jpeg",
	ImageTypeMagick: "magick",
	ImageTypePDF:    "pdf",
	ImageTypePNG:    "png",
	ImageTypeSVG:    "svg",
	ImageTypeTIFF:   "tiff",
	ImageTypeWEBP:   "webp",
	ImageTypeHEIF:   "heif",
	ImageTypeBMP:    "bmp",
}

// FileExt returns the canonical extension for the ImageType
func (i ImageType) FileExt() string {
	if ext, ok := imageTypeExtensionMap[i]; ok {
		return ext
	}
	return ""
}

// IsTypeSupported checks whether given image type is supported by govips
func IsTypeSupported(imageType ImageType) bool {
	startupIfNeeded()

	return supportedImageTypes[imageType]
}

// DetermineImageType attempts to determine the image type of the given buffer
func DetermineImageType(buf []byte) ImageType {
	if len(buf) < 12 {
		return ImageTypeUnknown
	} else if isJPEG(buf) {
		return ImageTypeJPEG
	} else if isPNG(buf) {
		return ImageTypePNG
	} else if isGIF(buf) {
		return ImageTypeGIF
	} else if isTIFF(buf) {
		return ImageTypeTIFF
	} else if isWEBP(buf) {
		return ImageTypeWEBP
	} else if isHEIF(buf) {
		return ImageTypeHEIF
	} else if isSVG(buf) {
		return ImageTypeSVG
	} else if isPDF(buf) {
		return ImageTypePDF
	} else if isBMP(buf) {
		return ImageTypeBMP
	} else {
		return ImageTypeUnknown
	}
}

var jpeg = []byte("\xFF\xD8\xFF")

func isJPEG(buf []byte) bool {
	return bytes.HasPrefix(buf, jpeg)
}

var gifHeader = []byte("\x47\x49\x46")

func isGIF(buf []byte) bool {
	return bytes.HasPrefix(buf, gifHeader)
}

var pngHeader = []byte("\x89\x50\x4E\x47")

func isPNG(buf []byte) bool {
	return bytes.HasPrefix(buf, pngHeader)
}

var tifII = []byte("\x49\x49\x2A\x00")
var tifMM = []byte("\x4D\x4D\x00\x2A")

func isTIFF(buf []byte) bool {
	return bytes.HasPrefix(buf, tifII) || bytes.HasPrefix(buf, tifMM)
}

var webpHeader = []byte("\x57\x45\x42\x50")

func isWEBP(buf []byte) bool {
	return bytes.Equal(buf[8:12], webpHeader)
}

// https://github.com/strukturag/libheif/blob/master/libheif/heif.cc
var ftyp = []byte("ftyp")
var heic = []byte("heic")
var mif1 = []byte("mif1")
var msf1 = []byte("msf1")
var avif = []byte("avif")

func isHEIF(buf []byte) bool {
	return bytes.Equal(buf[4:8], ftyp) && (bytes.Equal(buf[8:12], heic) ||
		bytes.Equal(buf[8:12], avif) ||
		bytes.Equal(buf[8:12], mif1) ||
		bytes.Equal(buf[8:12], msf1))
}

var svg = []byte("<svg")

func isSVG(buf []byte) bool {
	sub := buf[:int(math.Min(1024.0, float64(len(buf))))]
	if bytes.Contains(sub, svg) {
		data := &struct {
			XMLName xml.Name `xml:"svg"`
		}{}
		reader := bytes.NewReader(buf)
		decoder := xml.NewDecoder(reader)
		decoder.Strict = false
		decoder.CharsetReader = charset.NewReaderLabel

		err := decoder.Decode(data)

		return err == nil && data.XMLName.Local == "svg"
	}

	return false
}

var pdf = []byte("\x25\x50\x44\x46")

func isPDF(buf []byte) bool {
	return bytes.HasPrefix(buf, pdf)
}

var bmpHeader = []byte("BM")

func isBMP(buf []byte) bool {
	return bytes.HasPrefix(buf, bmpHeader)
}

func vipsLoadFromBuffer(buf []byte, o ...ImportOption) (*C.VipsImage, ImageType, error) {
	src := buf
	// Reference src here so it's not garbage collected during image initialization.
	defer runtime.KeepAlive(src)
	imageType := DetermineImageType(buf)

	options := ImportOptions{
		imageType: ImageTypeUnknown,
		params: importParams{
			shrink: 1,
			fail:   false,
			// set default autorotate = true for HEIF as currently it addresses orientation issues
			// https://github.com/libvips/libvips/pull/1680
			autorotate: imageType == ImageTypeHEIF,
			page:       0,
			n:          1,
			scale:      1,
			subifd:     -1,
			dpi:        72,
			unlimited:  false,
			thumbnail:  false,
			density:    "72x72",
		},
	}

	for _, option := range o {
		option(&options)
	}

	var err error
	var out *C.VipsImage

	// if image type is overridden by options
	if options.imageType != ImageTypeUnknown {
		imageType = options.imageType
	}

	if imageType == ImageTypeBMP {
		src, err = bmpToPNG(src)
		if err != nil {
			return nil, ImageTypeUnknown, err
		}

		imageType = ImageTypePNG
	}

	if !IsTypeSupported(imageType) {
		govipsLog("govips", LogLevelInfo, fmt.Sprintf("failed to understand image format size=%d", len(src)))
		return nil, ImageTypeUnknown, ErrUnsupportedImageFormat
	}

	var code C.int

	switch imageType {
	case ImageTypeJPEG:
		code = C.load_jpeg_buffer(unsafe.Pointer(&src[0]), C.size_t(len(src)), &out,
			C.int(options.params.shrink), C.int(boolToInt(options.params.fail)),
			C.int(boolToInt(options.params.autorotate)))
	case ImageTypePNG:
		code = C.load_png_buffer(unsafe.Pointer(&src[0]), C.size_t(len(src)), &out)
	case ImageTypeWEBP:
		code = C.load_webp_buffer(unsafe.Pointer(&src[0]), C.size_t(len(src)), &out,
			C.int(options.params.shrink))
	case ImageTypeTIFF:
		code = C.load_tiff_buffer(unsafe.Pointer(&src[0]), C.size_t(len(src)), &out,
			C.int(options.params.page), C.int(options.params.n), C.int(boolToInt(options.params.autorotate)),
			C.int(options.params.subifd))
	case ImageTypeGIF:
		code = C.load_gif_buffer(unsafe.Pointer(&src[0]), C.size_t(len(src)), &out,
			C.int(options.params.page), C.int(options.params.n))
	case ImageTypePDF:
		govipsLog("govips", LogLevelInfo, fmt.Sprintf("pdf options page=%d n=%d dpi=%f scale=%f", options.params.page, options.params.n, options.params.dpi, options.params.scale))
		code = C.load_pdf_buffer(unsafe.Pointer(&src[0]), C.size_t(len(src)), &out,
			C.int(options.params.page), C.int(options.params.n), C.double(options.params.dpi),
			C.double(options.params.scale))
	case ImageTypeSVG:
		code = C.load_svg_buffer(unsafe.Pointer(&src[0]), C.size_t(len(src)), &out,
			C.double(options.params.dpi), C.double(options.params.scale), C.int(boolToInt(options.params.unlimited)))
	case ImageTypeHEIF:
		code = C.load_heif_buffer(unsafe.Pointer(&src[0]), C.size_t(len(src)), &out,
			C.int(options.params.page), C.int(options.params.n), C.int(boolToInt(options.params.thumbnail)))
	case ImageTypeMagick:
		density := C.CString(options.params.density)
		defer C.free(unsafe.Pointer(density))
		code = C.load_magick_buffer(unsafe.Pointer(&src[0]), C.size_t(len(src)), &out,
			C.int(options.params.page), C.int(options.params.n), density)
	default:
		panic(ErrUnsupportedImageFormat) // unreachable, in theory
	}

	if code != 0 {
		return nil, ImageTypeUnknown, handleImageError(out)
	}

	return out, imageType, nil
}

func bmpToPNG(src []byte) ([]byte, error) {
	i, err := bmp.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, err
	}

	var w bytes.Buffer
	err = png.Encode(&w, i)
	if err != nil {
		return nil, err
	}

	return w.Bytes(), nil
}

func vipsSavePNGToBuffer(in *C.VipsImage, stripMetadata bool, compression int, interlaced bool) ([]byte, error) {
	incOpCounter("save_png_buffer")
	var ptr unsafe.Pointer
	cLen := C.size_t(0)

	strip := C.int(boolToInt(stripMetadata))
	comp := C.int(compression)
	inter := C.int(boolToInt(interlaced))

	if err := C.save_png_buffer(in, &ptr, &cLen, strip, comp, inter); err != 0 {
		return nil, handleSaveBufferError(ptr)
	}

	return toBuff(ptr, cLen), nil
}

func vipsSaveWebPToBuffer(in *C.VipsImage, stripMetadata bool, quality int, lossless bool, effort int) ([]byte, error) {
	incOpCounter("save_webp_buffer")
	var ptr unsafe.Pointer
	cLen := C.size_t(0)

	strip := C.int(boolToInt(stripMetadata))
	qual := C.int(quality)
	loss := C.int(boolToInt(lossless))
	eff := C.int(effort)

	if err := C.save_webp_buffer(in, &ptr, &cLen, strip, qual, loss, eff); err != 0 {
		return nil, handleSaveBufferError(ptr)
	}

	return toBuff(ptr, cLen), nil
}

func vipsSaveTIFFToBuffer(in *C.VipsImage, stripMetadata bool, quality int, lossless bool) ([]byte, error) {
	incOpCounter("save_tiff_buffer")
	var ptr unsafe.Pointer
	cLen := C.size_t(0)

	strip := C.int(boolToInt(stripMetadata))
	qual := C.int(quality)
	loss := C.int(boolToInt(lossless))

	if err := C.save_tiff_buffer(in, &ptr, &cLen, strip, qual, loss); err != 0 {
		return nil, handleSaveBufferError(ptr)
	}

	return toBuff(ptr, cLen), nil
}

func vipsSaveHEIFToBuffer(in *C.VipsImage, quality int, lossless bool) ([]byte, error) {
	incOpCounter("save_heif_buffer")
	var ptr unsafe.Pointer
	cLen := C.size_t(0)

	qual := C.int(quality)
	loss := C.int(boolToInt(lossless))

	if err := C.save_heif_buffer(in, &ptr, &cLen, qual, loss); err != 0 {
		return nil, handleSaveBufferError(ptr)
	}

	return toBuff(ptr, cLen), nil
}

func vipsSaveJPEGToBuffer(in *C.VipsImage, quality int, stripMetadata, interlaced bool) ([]byte, error) {
	incOpCounter("save_jpeg_buffer")
	var ptr unsafe.Pointer
	cLen := C.size_t(0)

	strip := C.int(boolToInt(stripMetadata))
	qual := C.int(quality)
	inter := C.int(boolToInt(interlaced))

	if err := C.save_jpeg_buffer(in, &ptr, &cLen, strip, qual, inter); err != 0 {
		return nil, handleSaveBufferError(ptr)
	}

	return toBuff(ptr, cLen), nil
}

func toBuff(ptr unsafe.Pointer, cLen C.size_t) []byte {
	buf := C.GoBytes(ptr, C.int(cLen))
	gFreePointer(ptr)

	return buf
}
