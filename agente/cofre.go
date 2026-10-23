//go:build windows

package main

// Modo cofre (versão leve, sem administrador e desenhada para o Windows Defender
// NÃO a marcar como malware):
//   - um painel PRETO por cima de todo o ecrã, sempre no topo;
//   - marcado com SetWindowDisplayAffinity(WDA_EXCLUDEFROMCAPTURE): quem está no
//     escritório vê preto, mas a captura que vai para o tablet/Jarvis continua a
//     ver o ambiente de trabalho real POR TRÁS do painel;
//   - não fecha com Alt+F4 (ignora o WM_CLOSE) e volta sempre ao topo;
//   - liga/desliga só por comando (tablet/voz) e desliga-se sozinho ao fim de
//     20 minutos, para nunca ficares trancado.
//
// NÃO usa ganchos de teclado/rato, nem .vbs, nem mexe em políticas, nem
// LogonUser — era isso que o Defender apanhava. Por isso o curioso fica CEGO
// (vê só preto) e os cliques dele caem no painel; travar o teclado dele a 100%
// exigiria os ganchos, que só com o programa assinado.

import (
	"log"
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
	pSetWindowPos             = user32.NewProc("SetWindowPos")
	pDestroyWindow            = user32.NewProc("DestroyWindow")
	pGetMessageW              = user32.NewProc("GetMessageW")
	pTranslateMessage         = user32.NewProc("TranslateMessage")
	pDispatchMessageW         = user32.NewProc("DispatchMessageW")
	pPostQuitMessage          = user32.NewProc("PostQuitMessage")
	pPostThreadMessageW       = user32.NewProc("PostThreadMessageW")
	pSetTimer                 = user32.NewProc("SetTimer")
	pKillTimer                = user32.NewProc("KillTimer")
	pLoadCursorW              = user32.NewProc("LoadCursorW")
	pSetWindowDisplayAffinity = user32.NewProc("SetWindowDisplayAffinity")

	pCreateSolidBrush = gdi32.NewProc("CreateSolidBrush")

	pGetModuleHandleW   = kernel32.NewProc("GetModuleHandleW")
	pGetCurrentThreadId = kernel32.NewProc("GetCurrentThreadId")
)

const (
	cofreWsExTopmost = 0x00000008
	cofreWsExToolwin = 0x00000080
	cofreWsPopup     = 0x80000000
	cofreSwShow      = 5

	cofreHwndTopmost = ^uintptr(0) // (HWND)-1
	cofreSwpNoActive = 0x0010
	cofreSwpShow     = 0x0040

	cofreWmDestroy = 0x0002
	cofreWmClose   = 0x0010
	cofreWmTimer   = 0x0113
	cofreWmQuit    = 0x0012

	cofreIdcArrow          = 32512
	cofreWdaExcludeFromCap = 0x00000011
	cofreTimerTopo         = 1
	cofreTimerAuto         = 2
	cofreAutoMs            = 20 * 60 * 1000 // 20 min de segurança
)

type cofreWndClassEx struct {
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

type cofreMsg struct {
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
	cofreBrush uintptr
	cofreReg   bool
)

func cofreAtivo() bool {
	cofreMu.Lock()
	defer cofreMu.Unlock()
	return cofreOn
}

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
			pPostThreadMessageW.Call(uintptr(tid), cofreWmQuit, 0, 0)
		}
	}
}

// WndProc: mantém-se no topo, ignora o Alt+F4 e desliga ao fim do tempo.
var cofreWndProc = syscall.NewCallback(func(hwnd uintptr, m uint32, w, l uintptr) uintptr {
	switch m {
	case cofreWmTimer:
		if w == cofreTimerTopo {
			ev := ecraVirtual()
			pSetWindowPos.Call(hwnd, cofreHwndTopmost, uintptr(int32(ev.X)), uintptr(int32(ev.Y)),
				uintptr(int32(ev.W)), uintptr(int32(ev.H)), cofreSwpShow|cofreSwpNoActive)
		} else if w == cofreTimerAuto {
			pPostQuitMessage.Call(0)
		}
		return 0
	case cofreWmClose:
		// ignora o Alt+F4 — só sai por comando remoto ou pelo tempo
		return 0
	case cofreWmDestroy:
		pPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(m), w, l)
	return r
})

func cofreThread() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	tid, _, _ := pGetCurrentThreadId.Call()
	cofreMu.Lock()
	cofreTID = uint32(tid)
	cofreMu.Unlock()

	hinst, _, _ := pGetModuleHandleW.Call(0)
	classe := u16("EcraRemotoCofre")

	if !cofreReg {
		if cofreBrush == 0 {
			cofreBrush, _, _ = pCreateSolidBrush.Call(0x000000) // preto
		}
		cur, _, _ := pLoadCursorW.Call(0, cofreIdcArrow)
		wc := cofreWndClassEx{
			LpfnWndProc:   cofreWndProc,
			HInstance:     hinst,
			HCursor:       cur,
			HbrBackground: cofreBrush,
			LpszClassName: classe,
		}
		wc.CbSize = uint32(unsafe.Sizeof(wc))
		pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
		cofreReg = true
	}

	ev := ecraVirtual()
	hwnd, _, _ := pCreateWindowExW.Call(
		cofreWsExTopmost|cofreWsExToolwin,
		uintptr(unsafe.Pointer(classe)),
		uintptr(unsafe.Pointer(u16("Cofre"))),
		cofreWsPopup,
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

	// esconder da captura: o escritório vê preto, o tablet vê o que está por trás
	if ok, _, _ := pSetWindowDisplayAffinity.Call(hwnd, cofreWdaExcludeFromCap); ok == 0 {
		log.Printf("cofre: SetWindowDisplayAffinity falhou (Windows antigo?) — a captura pode sair preta")
	}

	pShowWindow.Call(hwnd, cofreSwShow)
	pSetWindowPos.Call(hwnd, cofreHwndTopmost, uintptr(int32(ev.X)), uintptr(int32(ev.Y)),
		uintptr(int32(ev.W)), uintptr(int32(ev.H)), cofreSwpShow|cofreSwpNoActive)
	pUpdateWindow.Call(hwnd)
	pSetTimer.Call(hwnd, cofreTimerTopo, 500, 0)
	pSetTimer.Call(hwnd, cofreTimerAuto, cofreAutoMs, 0)
	log.Printf("cofre: ligado")

	var msg cofreMsg
	for {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}

	pKillTimer.Call(hwnd, cofreTimerTopo)
	pKillTimer.Call(hwnd, cofreTimerAuto)
	pDestroyWindow.Call(hwnd)
	cofreHwnd = 0
	cofreMu.Lock()
	cofreOn = false
	cofreTID = 0
	cofreMu.Unlock()
	log.Printf("cofre: desligado")
}
