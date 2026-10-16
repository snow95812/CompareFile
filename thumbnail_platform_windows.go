//go:build windows

package main

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"syscall"
	"unsafe"
)

type shellItemImageFactory struct {
	lpVtbl *shellItemImageFactoryVTable
}

type shellItemImageFactoryVTable struct {
	queryInterface uintptr
	addRef         uintptr
	release        uintptr
	getImage       uintptr
}

type shellSize struct {
	cx int32
	cy int32
}

type winBitmap struct {
	bmType       int32
	bmWidth      int32
	bmHeight     int32
	bmWidthBytes int32
	bmPlanes     uint16
	bmBitsPixel  uint16
	bmBits       uintptr
}

type bitmapInfoHeader struct {
	biSize          uint32
	biWidth         int32
	biHeight        int32
	biPlanes        uint16
	biBitCount      uint16
	biCompression   uint32
	biSizeImage     uint32
	biXPelsPerMeter int32
	biYPelsPerMeter int32
	biClrUsed       uint32
	biClrImportant  uint32
}

type rgbQuad struct {
	rgbBlue     byte
	rgbGreen    byte
	rgbRed      byte
	rgbReserved byte
}

type bitmapInfo struct {
	bmiHeader bitmapInfoHeader
	bmiColors [1]rgbQuad
}

type guid struct {
	data1 uint32
	data2 uint16
	data3 uint16
	data4 [8]byte
}

var (
	ole32DLL                     = syscall.NewLazyDLL("ole32.dll")
	coInitializeExProc           = ole32DLL.NewProc("CoInitializeEx")
	coUninitializeProc           = ole32DLL.NewProc("CoUninitialize")
	shell32ThumbnailDLL          = syscall.NewLazyDLL("shell32.dll")
	shCreateItemFromParsingName  = shell32ThumbnailDLL.NewProc("SHCreateItemFromParsingName")
	gdi32DLL                     = syscall.NewLazyDLL("gdi32.dll")
	getObjectWProc               = gdi32DLL.NewProc("GetObjectW")
	createCompatibleDCProc       = gdi32DLL.NewProc("CreateCompatibleDC")
	deleteDCProc                 = gdi32DLL.NewProc("DeleteDC")
	getDIBitsProc                = gdi32DLL.NewProc("GetDIBits")
	deleteObjectThumbnailProc    = gdi32DLL.NewProc("DeleteObject")
	iidShellItemImageFactoryGuid = guid{
		data1: 0xbcc18b79,
		data2: 0xba16,
		data3: 0x442f,
		data4: [8]byte{0x80, 0xc4, 0x8a, 0x59, 0xc3, 0x0c, 0x46, 0x3b},
	}
)

const (
	coInitApartmentThreaded = 0x2
	biRGB                   = 0
	dibRGBColors            = 0
	siigbfBiggerSizeOK      = 0x1
	siigbfThumbnailOnly     = 0x8
	siigbfCropToSquare      = 0x20
	rpcEChangedMode         = 0x80010106
)

func generatePlatformThumbnail(filePath string, size int, cacheDir string) ([]byte, string, error) {
	_ = cacheDir
	if size <= 0 {
		size = 100
	}

	uninitialize, err := initializeThumbnailCOM()
	if err != nil {
		return nil, "", err
	}
	if uninitialize {
		defer coUninitializeProc.Call()
	}

	pathUTF16, err := syscall.UTF16PtrFromString(filePath)
	if err != nil {
		return nil, "", err
	}

	var factory *shellItemImageFactory
	hr, _, _ := shCreateItemFromParsingName.Call(
		uintptr(unsafe.Pointer(pathUTF16)),
		0,
		uintptr(unsafe.Pointer(&iidShellItemImageFactoryGuid)),
		uintptr(unsafe.Pointer(&factory)),
	)
	if failedHRESULT(hr) || factory == nil {
		return nil, "", errors.New("windows shell thumbnail unavailable")
	}
	defer releaseShellItemImageFactory(factory)

	hBitmap, err := factory.getImage(shellSize{cx: int32(size), cy: int32(size)}, siigbfThumbnailOnly|siigbfBiggerSizeOK|siigbfCropToSquare)
	if err != nil {
		return nil, "", err
	}
	defer deleteObjectThumbnailProc.Call(uintptr(hBitmap))

	thumbnailData, err := encodeBitmapToPNG(hBitmap)
	if err != nil {
		return nil, "", err
	}

	return thumbnailData, "image/png", nil
}

func initializeThumbnailCOM() (bool, error) {
	hr, _, _ := coInitializeExProc.Call(0, uintptr(coInitApartmentThreaded))
	switch uint32(hr) {
	case 0, 1:
		return true, nil
	case rpcEChangedMode:
		return false, nil
	default:
		return false, errors.New("initialize COM failed")
	}
}

func failedHRESULT(hr uintptr) bool {
	return int32(uint32(hr)) < 0
}

func releaseShellItemImageFactory(factory *shellItemImageFactory) {
	if factory == nil || factory.lpVtbl == nil || factory.lpVtbl.release == 0 {
		return
	}

	syscall.SyscallN(
		factory.lpVtbl.release,
		uintptr(unsafe.Pointer(factory)),
	)
}

func (factory *shellItemImageFactory) getImage(size shellSize, flags uint32) (syscall.Handle, error) {
	if factory == nil || factory.lpVtbl == nil || factory.lpVtbl.getImage == 0 {
		return 0, errors.New("windows shell thumbnail unavailable")
	}

	var hBitmap syscall.Handle
	packedSize := *(*int64)(unsafe.Pointer(&size))
	hr, _, _ := syscall.SyscallN(
		factory.lpVtbl.getImage,
		uintptr(unsafe.Pointer(factory)),
		uintptr(packedSize),
		uintptr(flags),
		uintptr(unsafe.Pointer(&hBitmap)),
	)
	if failedHRESULT(hr) || hBitmap == 0 {
		return 0, errors.New("get thumbnail image failed")
	}

	return hBitmap, nil
}

func encodeBitmapToPNG(hBitmap syscall.Handle) ([]byte, error) {
	var bitmap winBitmap
	result, _, _ := getObjectWProc.Call(
		uintptr(hBitmap),
		unsafe.Sizeof(bitmap),
		uintptr(unsafe.Pointer(&bitmap)),
	)
	if result == 0 {
		return nil, errors.New("read bitmap failed")
	}

	if bitmap.bmWidth <= 0 || bitmap.bmHeight <= 0 {
		return nil, errors.New("bitmap has invalid size")
	}

	hdc, _, _ := createCompatibleDCProc.Call(0)
	if hdc == 0 {
		return nil, errors.New("create bitmap context failed")
	}
	defer deleteDCProc.Call(hdc)

	info := bitmapInfo{
		bmiHeader: bitmapInfoHeader{
			biSize:        uint32(unsafe.Sizeof(bitmapInfoHeader{})),
			biWidth:       bitmap.bmWidth,
			biHeight:      -bitmap.bmHeight,
			biPlanes:      1,
			biBitCount:    32,
			biCompression: biRGB,
		},
	}

	pixelBytes := int(bitmap.bmWidth) * int(bitmap.bmHeight) * 4
	pixels := make([]byte, pixelBytes)

	lines, _, _ := getDIBitsProc.Call(
		hdc,
		uintptr(hBitmap),
		0,
		uintptr(bitmap.bmHeight),
		uintptr(unsafe.Pointer(&pixels[0])),
		uintptr(unsafe.Pointer(&info)),
		uintptr(dibRGBColors),
	)
	if lines == 0 {
		return nil, errors.New("extract bitmap pixels failed")
	}

	imageBuffer := image.NewNRGBA(image.Rect(0, 0, int(bitmap.bmWidth), int(bitmap.bmHeight)))
	for index := 0; index < pixelBytes; index += 4 {
		imageBuffer.Pix[index+0] = pixels[index+2]
		imageBuffer.Pix[index+1] = pixels[index+1]
		imageBuffer.Pix[index+2] = pixels[index+0]
		imageBuffer.Pix[index+3] = pixels[index+3]
	}

	buffer := bytes.NewBuffer(nil)
	if err := png.Encode(buffer, imageBuffer); err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}
