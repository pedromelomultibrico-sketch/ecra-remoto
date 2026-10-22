//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	advapi32 = syscall.NewLazyDLL("advapi32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")

	pGetDC                          = user32.NewProc("GetDC")
	pReleaseDC                      = user32.NewProc("ReleaseDC")
	pEnumDisplayMonitors            = user32.NewProc("EnumDisplayMonitors")
	pGetMonitorInfoW                = user32.NewProc("GetMonitorInfoW")
	pGetSystemMetrics               = user32.NewProc("GetSystemMetrics")
	pSetCursorPos                   = user32.NewProc("SetCursorPos")
	pGetCursorPos                   = user32.NewProc("GetCursorPos")
	pSendInput                      = user32.NewProc("SendInput")
	pMapVirtualKeyW                 = user32.NewProc("MapVirtualKeyW")
	pVkKeyScanW                     = user32.NewProc("VkKeyScanW")
	pMessageBoxW                    = user32.NewProc("MessageBoxW")
	pLockWorkStation                = user32.NewProc("LockWorkStation")
	pOpenClipboard                  = user32.NewProc("OpenClipboard")
	pCloseClipboard                 = user32.NewProc("CloseClipboard")
	pEmptyClipboard                 = user32.NewProc("EmptyClipboard")
	pGetClipboardData               = user32.NewProc("GetClipboardData")
	pSetClipboardData               = user32.NewProc("SetClipboardData")
	pGetClipboardSequenceNumber     = user32.NewProc("GetClipboardSequenceNumber")
	pSetProcessDpiAwarenessContext  = user32.NewProc("SetProcessDpiAwarenessContext")
	pSetProcessDPIAware             = user32.NewProc("SetProcessDPIAware")
	pOpenInputDesktop               = user32.NewProc("OpenInputDesktop")
	pSetThreadDesktop               = user32.NewProc("SetThreadDesktop")
	pCloseDesktop                   = user32.NewProc("CloseDesktop")

	pCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	pCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	pSelectObject       = gdi32.NewProc("SelectObject")
	pDeleteObject       = gdi32.NewProc("DeleteObject")
	pDeleteDC           = gdi32.NewProc("DeleteDC")
	pBitBlt             = gdi32.NewProc("BitBlt")
	pStretchBlt         = gdi32.NewProc("StretchBlt")
	pSetStretchBltMode  = gdi32.NewProc("SetStretchBltMode")
	pSetBrushOrgEx      = gdi32.NewProc("SetBrushOrgEx")
	pGdiFlush           = gdi32.NewProc("GdiFlush")

	pCreateMutexW              = kernel32.NewProc("CreateMutexW")
	pCloseHandle               = kernel32.NewProc("CloseHandle")
	pGlobalAlloc               = kernel32.NewProc("GlobalAlloc")
	pGlobalLock                = kernel32.NewProc("GlobalLock")
	pGlobalUnlock              = kernel32.NewProc("GlobalUnlock")
	pGlobalSize                = kernel32.NewProc("GlobalSize")
	pGetLogicalDriveStringsW   = kernel32.NewProc("GetLogicalDriveStringsW")
	pSetThreadExecutionState   = kernel32.NewProc("SetThreadExecutionState")
	pGetSystemPowerStatus      = kernel32.NewProc("GetSystemPowerStatus")

	pRegCreateKeyExW  = advapi32.NewProc("RegCreateKeyExW")
	pRegOpenKeyExW    = advapi32.NewProc("RegOpenKeyExW")
	pRegSetValueExW   = advapi32.NewProc("RegSetValueExW")
	pRegQueryValueExW = advapi32.NewProc("RegQueryValueExW")
	pRegDeleteValueW  = advapi32.NewProc("RegDeleteValueW")
	pRegCloseKey      = advapi32.NewProc("RegCloseKey")

	pShellExecuteW = shell32.NewProc("ShellExecuteW")
)

type rect struct{ Left, Top, Right, Bottom int32 }
type point struct{ X, Y int32 }

type monitorInfo struct {
	CbSize    uint32
	RcMonitor rect
	RcWork    rect
	DwFlags   uint32
}

type bitmapInfoHeader struct {
	BiSize          uint32
	BiWidth         int32
	BiHeight        int32
	BiPlanes        uint16
	BiBitCount      uint16
	BiCompression   uint32
	BiSizeImage     uint32
	BiXPelsPerMeter int32
	BiYPelsPerMeter int32
	BiClrUsed       uint32
	BiClrImportant  uint32
}

type bitmapInfo struct {
	Header bitmapInfoHeader
	Colors [4]byte
}

// INPUT (64 bits): tipo + união de 32 bytes = 40 bytes
type mouseInput struct {
	Typ       uint32
	_         uint32
	Dx, Dy    int32
	MouseData uint32
	Flags     uint32
	Time      uint32
	_         uint32
	Extra     uintptr
}

type keybdInput struct {
	Typ   uint32
	_     uint32
	Vk    uint16
	Scan  uint16
	Flags uint32
	Time  uint32
	_     uint32
	Extra uintptr
	_     [8]byte
}

type powerStatus struct {
	ACLineStatus        byte
	BatteryFlag         byte
	BatteryLifePercent  byte
	SystemStatusFlag    byte
	BatteryLifeTime     uint32
	BatteryFullLifeTime uint32
}

const (
	SRCCOPY   = 0x00CC0020
	HALFTONE  = 4
	BI_RGB    = 0
	DIB_RGB   = 0
	SM_XVSCR  = 76
	SM_YVSCR  = 77
	SM_CXVSCR = 78
	SM_CYVSCR = 79

	INPUT_MOUSE    = 0
	INPUT_KEYBOARD = 1

	MOUSEEVENTF_LEFTDOWN   = 0x0002
	MOUSEEVENTF_LEFTUP     = 0x0004
	MOUSEEVENTF_RIGHTDOWN  = 0x0008
	MOUSEEVENTF_RIGHTUP    = 0x0010
	MOUSEEVENTF_MIDDLEDOWN = 0x0020
	MOUSEEVENTF_MIDDLEUP   = 0x0040
	MOUSEEVENTF_WHEEL      = 0x0800
	MOUSEEVENTF_HWHEEL     = 0x1000

	KEYEVENTF_EXTENDEDKEY = 0x0001
	KEYEVENTF_KEYUP       = 0x0002
	KEYEVENTF_UNICODE     = 0x0004
	KEYEVENTF_SCANCODE    = 0x0008

	CF_UNICODETEXT = 13
	GMEM_MOVEABLE  = 0x0002

	HKEY_CURRENT_USER = 0x80000001
	KEY_READ          = 0x20019
	KEY_WRITE         = 0x20006
	REG_SZ            = 1
	REG_DWORD         = 4

	MB_OK              = 0x0
	MB_YESNOCANCEL     = 0x3
	MB_ICONINFORMATION = 0x40
	MB_ICONWARNING     = 0x30
	MB_SETFOREGROUND   = 0x10000
	MB_TOPMOST         = 0x40000
	IDYES              = 6
	IDNO               = 7
	IDCANCEL           = 2

	ES_CONTINUOUS       = 0x80000000
	ES_SYSTEM_REQUIRED  = 0x00000001
	ES_DISPLAY_REQUIRED = 0x00000002

	ERROR_ALREADY_EXISTS = 183
)

func u16(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

func messageBox(titulo, texto string, flags uintptr) int {
	r, _, _ := pMessageBoxW.Call(0, uintptr(unsafe.Pointer(u16(texto))), uintptr(unsafe.Pointer(u16(titulo))), flags|MB_SETFOREGROUND|MB_TOPMOST)
	return int(r)
}

// --- registo do Windows (só HKEY_CURRENT_USER: não precisa de administrador) ---

func regSetString(chave, nome, valor string) error {
	var h syscall.Handle
	r, _, _ := pRegCreateKeyExW.Call(HKEY_CURRENT_USER, uintptr(unsafe.Pointer(u16(chave))), 0, 0, 0, KEY_WRITE, 0, uintptr(unsafe.Pointer(&h)), 0)
	if r != 0 {
		return syscall.Errno(r)
	}
	defer pRegCloseKey.Call(uintptr(h))
	v, _ := syscall.UTF16FromString(valor)
	r, _, _ = pRegSetValueExW.Call(uintptr(h), uintptr(unsafe.Pointer(u16(nome))), 0, REG_SZ, uintptr(unsafe.Pointer(&v[0])), uintptr(len(v)*2))
	if r != 0 {
		return syscall.Errno(r)
	}
	return nil
}

func regDelete(chave, nome string) {
	var h syscall.Handle
	r, _, _ := pRegOpenKeyExW.Call(HKEY_CURRENT_USER, uintptr(unsafe.Pointer(u16(chave))), 0, KEY_WRITE, uintptr(unsafe.Pointer(&h)))
	if r != 0 {
		return
	}
	defer pRegCloseKey.Call(uintptr(h))
	pRegDeleteValueW.Call(uintptr(h), uintptr(unsafe.Pointer(u16(nome))))
}

func regQuery(chave, nome string) (string, uint32, bool) {
	var h syscall.Handle
	r, _, _ := pRegOpenKeyExW.Call(HKEY_CURRENT_USER, uintptr(unsafe.Pointer(u16(chave))), 0, KEY_READ, uintptr(unsafe.Pointer(&h)))
	if r != 0 {
		return "", 0, false
	}
	defer pRegCloseKey.Call(uintptr(h))
	var tipo, tam uint32
	r, _, _ = pRegQueryValueExW.Call(uintptr(h), uintptr(unsafe.Pointer(u16(nome))), 0, uintptr(unsafe.Pointer(&tipo)), 0, uintptr(unsafe.Pointer(&tam)))
	if r != 0 || tam == 0 {
		return "", 0, false
	}
	buf := make([]byte, tam+2)
	r, _, _ = pRegQueryValueExW.Call(uintptr(h), uintptr(unsafe.Pointer(u16(nome))), 0, uintptr(unsafe.Pointer(&tipo)), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&tam)))
	if r != 0 {
		return "", 0, false
	}
	if tipo == REG_DWORD && tam >= 4 {
		return "", uint32(buf[0]) | uint32(buf[1])<<8 | uint32(buf[2])<<16 | uint32(buf[3])<<24, true
	}
	n := tam / 2
	w := make([]uint16, n)
	for i := uint32(0); i < n; i++ {
		w[i] = uint16(buf[2*i]) | uint16(buf[2*i+1])<<8
	}
	return syscall.UTF16ToString(w), 0, true
}

func shellOpen(alvo string) {
	pShellExecuteW.Call(0, uintptr(unsafe.Pointer(u16("open"))), uintptr(unsafe.Pointer(u16(alvo))), 0, 0, 1)
}
