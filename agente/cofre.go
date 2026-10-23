//go:build windows

package main

// Modo cofre: para o PC do escritório ficar fechado para quem está ao lado,
// mas aberto para o Pedro à distância (tablet / Jarvis).
//
// Como funciona, tudo sem precisar de administrador:
//   - um painel preto por cima de todo o ecrã, com uma caixa de palavra-passe;
//   - esse painel é marcado com SetWindowDisplayAffinity(WDA_EXCLUDEFROMCAPTURE):
//     quem está no escritório vê preto, mas a captura que segue para o tablet/Jarvis
//     continua a apanhar o ambiente de trabalho real POR TRÁS do painel;
//   - ganchos de baixo nível (WH_KEYBOARD_LL / WH_MOUSE_LL) travam o rato e o
//     teclado FÍSICOS, mas deixam passar o que vem de longe (SendInput, marcado
//     como "injetado") — por isso os comandos do Pedro continuam a funcionar;
//   - para desbloquear no PC, escreve-se a palavra-passe do próprio Windows, que
//     se valida com LogonUser (não precisa de admin).
//
// Limite honesto (já avisado ao Pedro): Ctrl+Alt+Del / a tecla Windows do
// sistema não se podem travar sem administrador. Se alguém fizer Ctrl+Alt+Del,
// o Windows mostra o seu ecrã próprio e o painel sai; o trabalho de bastidores
// continua, mas a imagem fica fora de alcance até alguém desbloquear.

import (
	"log"
	"os"
	"runtime"
	"sync"
	"syscall"
	"unsafe"
)

var (
	pCreateWindowExW          = user32.NewProc("CreateWindowExW")
	pDefWindowProcW           = user32.NewProc("DefWindowProcW")
	pRegisterClassExW         = user32.NewProc("RegisterClassExW")
	pShowWindow               = user32.NewProc("ShowWindow")
	pUpdateWindow             = user32.NewProc("UpdateWindow")
	pGetMessageW              = user32.NewProc("GetMessageW")
	pTranslateMessage         = user32.NewProc("TranslateMessage")
	pDispatchMessageW         = user32.NewProc("DispatchMessageW")
	pPostQuitMessage          = user32.NewProc("PostQuitMessage")
	pPostThreadMessageW       = user32.NewProc("PostThreadMessageW")
	pSetWindowsHookExW        = user32.NewProc("SetWindowsHookExW")
	pUnhookWindowsHookEx      = user32.NewProc("UnhookWindowsHookEx")
	pCallNextHookEx           = user32.NewProc("CallNextHookEx")
	pSetWindowPos             = user32.NewProc("SetWindowPos")
	pDestroyWindow            = user32.NewProc("DestroyWindow")
	pSetForegroundWindow      = user32.NewProc("SetForegroundWindow")
	pSetFocus                 = user32.NewProc("SetFocus")
	pSendMessageW             = user32.NewProc("SendMessageW")
	pSetWindowTextW           = user32.NewProc("SetWindowTextW")
	pGetWindowTextW           = user32.NewProc("GetWindowTextW")
	pSetTimer                 = user32.NewProc("SetTimer")
	pKillTimer                = user32.NewProc("KillTimer")
	pGetAsyncKeyState         = user32.NewProc("GetAsyncKeyState")
	pLoadCursorW              = user32.NewProc("LoadCursorW")
	pFillRect                 = user32.NewProc("FillRect")
	pBeginPaint               = user32.NewProc("BeginPaint")
	pEndPaint                 = user32.NewProc("EndPaint")
	pSetWindowDisplayAffinity = user32.NewProc("SetWindowDisplayAffinity")

	pCreateSolidBrush = gdi32.NewProc("CreateSolidBrush")

	pGetModuleHandleW  = kernel32.NewProc("GetModuleHandleW")
	pGetCurrentThreadId = kernel32.NewProc("GetCurrentThreadId")

	pLogonUserW = advapi32.NewProc("LogonUserW")
)

const (
	wsExTopmost   = 0x00000008
	wsExToolwin   = 0x00000080
	wsExLayered   = 0x00080000
	wsPopup       = 0x80000000
	wsVisible     = 0x10000000
	wsChild       = 0x40000000
	wsBorder      = 0x00800000

	swShow      = 5
	swpNoActive = 0x0010
	hwndTop     = 0

	whKeyboardLL = 13
	whMouseLL    = 14
	hcAction     = 0
	wmKeyDown    = 0x0100
	wmSysKeyDown = 0x0104

	llkhfInjected = 0x10
	llmhfInjected = 0x01

	wmDestroy = 0x0002
	wmClose   = 0x0010
	wmTimer   = 0x0113
	wmPaint   = 0x000F
	wmSetFont = 0x0030
	wmGetText = 0x000D

	esPassword   = 0x0020
	esCenter     = 0x0001
	esAutoHScroll = 0x0080

	vkReturn = 0x0D
	vkTab    = 0x09
	vkEscape = 0x1B
	vkLWin   = 0x5B
	vkRWin   = 0x5C
	vkMenu   = 0x12 // ALT

	wdaExcludeFromCapture = 0x00000011

	logon32LogonNetwork  = 3
	logon32ProviderDefault = 0

	idcArrow = 32512
)

type wndClassEx struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

type msg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
}

var (
	cofreMu    sync.Mutex
	cofreOn    bool
	cofreTID   uint32
	cofreHwnd  uintptr
	cofreEdit  uintptr
	cofreHookK uintptr
	cofreHookM uintptr
	cofreBrush uintptr
	classeReg  bool
)

// --- ganchos de baixo nível ---

var cbKeyboard = syscall.NewCallback(func(code int32, wparam, lparam uintptr) uintptr {
	if code == hcAction {
		k := (*struct {
			VkCode, ScanCode, Flags, Time uint32
			DwExtra                       uintptr
		})(unsafe.Pointer(lparam))
		// o que vem de longe (Jarvis/tablet) está marcado como injetado — deixa passar
		if k.Flags&llkhfInjected == 0 {
			vk := k.VkCode
			alt := getKey(vkMenu)
			// bloqueia as fugas, deixa o resto para poder escrever a palavra-passe
			if vk == vkLWin || vk == vkRWin || vk == vkEscape || (vk == vkTab && alt) {
				return 1
			}
		}
	}
	r, _, _ := pCallNextHookEx.Call(0, uintptr(code), wparam, lparam)
	return r
})

var cbMouse = syscall.NewCallback(func(code int32, wparam, lparam uintptr) uintptr {
	if code == hcAction {
		m := (*struct {
			Pt                     point
			MouseData, Flags, Time uint32
			DwExtra                uintptr
		})(unsafe.Pointer(lparam))
		if m.Flags&llmhfInjected == 0 {
			return 1 // trava todo o rato físico
		}
	}
	r, _, _ := pCallNextHookEx.Call(0, uintptr(code), wparam, lparam)
	return r
})

func getKey(vk uint16) bool {
	r, _, _ := pGetAsyncKeyState.Call(uintptr(vk))
	return r&0x8000 != 0
}

// WNDPROC do painel preto
var cofreWndProc = syscall.NewCallback(func(hwnd uintptr, m uint32, w, l uintptr) uintptr {
	switch m {
	case wmTimer:
		// Enter na caixa de palavra-passe -> validar com o Windows
		if getKey(vkReturn) {
			buf := make([]uint16, 256)
			n, _, _ := pSendMessageW.Call(cofreEdit, wmGetText, uintptr(len(buf)), uintptr(unsafe.Pointer(&buf[0])))
			senha := syscall.UTF16ToString(buf[:n])
			if senha != "" && validarWindows(senha) {
				pPostQuitMessage.Call(0)
			} else {
				pSetWindowTextW.Call(cofreEdit, uintptr(unsafe.Pointer(u16("")))) // limpa
			}
		}
		return 0
	case wmDestroy:
		pPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(m), w, l)
	return r
})

// validarWindows confirma a palavra-passe do utilizador atual via LogonUser
// (tipo "rede": não precisa de administrador nem do direito de início de sessão local).
func validarWindows(senha string) bool {
	user := u16(usernameAtual())
	dom := u16(".")
	pw := u16(senha)
	var token uintptr
	r, _, _ := pLogonUserW.Call(
		uintptr(unsafe.Pointer(user)),
		uintptr(unsafe.Pointer(dom)),
		uintptr(unsafe.Pointer(pw)),
		logon32LogonNetwork,
		logon32ProviderDefault,
		uintptr(unsafe.Pointer(&token)),
	)
	if token != 0 {
		pCloseHandle.Call(token)
	}
	return r != 0
}

func usernameAtual() string {
	return os.Getenv("USERNAME")
}

func cofreAtivo() bool {
	cofreMu.Lock()
	defer cofreMu.Unlock()
	return cofreOn
}

// cofreLigar liga/desliga o modo cofre. Thread-safe; chamável de qualquer sítio.
func cofreLigar(on bool) {
	cofreMu.Lock()
	if on == cofreOn {
		cofreMu.Unlock()
		return
	}
	if on {
		cofreOn = true
		cofreMu.Unlock()
		go cofreThread()
	} else {
		tid := cofreTID
		cofreOn = false
		cofreMu.Unlock()
		if tid != 0 {
			// WM_QUIT = 0x0012
			pPostThreadMessageW.Call(uintptr(tid), 0x0012, 0, 0)
		}
	}
}

func cofreThread() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	tid, _, _ := pGetCurrentThreadId.Call()
	cofreMu.Lock()
	cofreTID = uint32(tid)
	cofreMu.Unlock()

	hinst, _, _ := pGetModuleHandleW.Call(0)
	classe := u16("EcraRemotoCofre")

	if !classeReg {
		if cofreBrush == 0 {
			cofreBrush, _, _ = pCreateSolidBrush.Call(0x000000) // preto
		}
		cur, _, _ := pLoadCursorW.Call(0, idcArrow)
		wc := wndClassEx{
			Style:         0,
			LpfnWndProc:   cofreWndProc,
			HInstance:     hinst,
			HCursor:       cur,
			HbrBackground: cofreBrush,
			LpszClassName: classe,
		}
		wc.CbSize = uint32(unsafe.Sizeof(wc))
		pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
		classeReg = true
	}

	// dimensão do ecrã virtual (todos os monitores)
	ev := ecraVirtual()

	hwnd, _, _ := pCreateWindowExW.Call(
		wsExTopmost|wsExToolwin,
		uintptr(unsafe.Pointer(classe)),
		uintptr(unsafe.Pointer(u16("Cofre"))),
		wsPopup,
		uintptr(int32(ev.X)), uintptr(int32(ev.Y)), uintptr(int32(ev.W)), uintptr(int32(ev.H)),
		0, 0, hinst, 0,
	)
	if hwnd == 0 {
		log.Printf("cofre: não consegui criar o painel")
		cofreMu.Lock()
		cofreOn = false
		cofreMu.Unlock()
		return
	}
	cofreHwnd = hwnd

	// ESCONDER DA CAPTURA: quem está no PC vê preto; o tablet/Jarvis vê o que está por trás.
	ok, _, _ := pSetWindowDisplayAffinity.Call(hwnd, wdaExcludeFromCapture)
	if ok == 0 {
		log.Printf("cofre: SetWindowDisplayAffinity falhou (Windows antigo?); a captura pode sair preta")
	}

	// caixa de palavra-passe centrada
	ew, eh := 360, 44
	ex := ev.W/2 - ew/2
	ey := ev.H/2 - eh/2
	edit, _, _ := pCreateWindowExW.Call(
		wsExTopmost,
		uintptr(unsafe.Pointer(u16("EDIT"))),
		0,
		wsChild|wsVisible|wsBorder|esPassword|esCenter|esAutoHScroll,
		uintptr(int32(ex)), uintptr(int32(ey)), uintptr(int32(ew)), uintptr(int32(eh)),
		hwnd, 0, hinst, 0,
	)
	cofreEdit = edit

	pShowWindow.Call(hwnd, swShow)
	pSetWindowPos.Call(hwnd, hwndTop, uintptr(int32(ev.X)), uintptr(int32(ev.Y)), uintptr(int32(ev.W)), uintptr(int32(ev.H)), 0)
	pUpdateWindow.Call(hwnd)
	pSetForegroundWindow.Call(hwnd)
	pSetFocus.Call(edit)
	pSetTimer.Call(hwnd, 1, 200, 0)

	// ganchos
	cofreHookK, _, _ = pSetWindowsHookExW.Call(whKeyboardLL, cbKeyboard, hinst, 0)
	cofreHookM, _, _ = pSetWindowsHookExW.Call(whMouseLL, cbMouse, hinst, 0)
	log.Printf("cofre: ligado")

	var m msg
	for {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}

	// limpeza
	if cofreHookK != 0 {
		pUnhookWindowsHookEx.Call(cofreHookK)
		cofreHookK = 0
	}
	if cofreHookM != 0 {
		pUnhookWindowsHookEx.Call(cofreHookM)
		cofreHookM = 0
	}
	pKillTimer.Call(hwnd, 1)
	pDestroyWindow.Call(hwnd)
	cofreHwnd, cofreEdit = 0, 0
	cofreMu.Lock()
	cofreOn = false
	cofreTID = 0
	cofreMu.Unlock()
	log.Printf("cofre: desligado")
}
