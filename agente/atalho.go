//go:build windows

package main

// Atalhos para LIGAR O COFRE no próprio PC (23/10, pedido do Pedro), sem administrador:
//   1) a tecla do cadeado do teclado HP (envia Windows+L). Para o Windows não
//      bloquear à maneira dele, desliga-se o bloqueio do Windows+L só para este
//      utilizador (HKCU ...\Policies\System\DisableLockWorkstation=1). Se o
//      Windows não deixar gravar isso, a tecla continua a fazer o bloqueio normal
//      e o cofre NÃO é ligado por ela (para não ficar cofre por cima do bloqueio);
//   2) Ctrl+Alt+C, sempre;
//   3) o ficheiro "Cofre" no ambiente de trabalho (corre ecra-remoto.exe --cofre).

import (
	"log"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

var (
	pRegisterHotKey = user32.NewProc("RegisterHotKey")
)

const (
	chavePolSistema = `Software\Microsoft\Windows\CurrentVersion\Policies\System`
	modAlt          = 0x0001
	modControl      = 0x0002
	modNoRepeat     = 0x4000
	wmHotkey        = 0x0312
	vkL             = 0x4C
	vkC             = 0x43
)

var cadeadoAtivo bool // true só se o bloqueio do Windows+L ficou mesmo desligado

func regSetDWORD(chave, nome string, valor uint32) error {
	var h syscall.Handle
	r, _, _ := pRegCreateKeyExW.Call(HKEY_CURRENT_USER, uintptr(unsafe.Pointer(u16(chave))), 0, 0, 0, KEY_WRITE, 0, uintptr(unsafe.Pointer(&h)), 0)
	if r != 0 {
		return syscall.Errno(r)
	}
	defer pRegCloseKey.Call(uintptr(h))
	v := valor
	r, _, _ = pRegSetValueExW.Call(uintptr(h), uintptr(unsafe.Pointer(u16(nome))), 0, REG_DWORD, uintptr(unsafe.Pointer(&v)), 4)
	if r != 0 {
		return syscall.Errno(r)
	}
	return nil
}

var cbAtalho = syscall.NewCallback(func(code int32, wparam, lparam uintptr) uintptr {
	if code == hcAction && cadeadoAtivo && (wparam == wmKeyDown || wparam == wmSysKeyDown) {
		k := (*struct {
			VkCode, ScanCode, Flags, Time uint32
			DwExtra                       uintptr
		})(unsafe.Pointer(lparam))
		if k.Flags&llkhfInjected == 0 && k.VkCode == vkL && (getKey(vkLWin) || getKey(vkRWin)) {
			if !cofreAtivo() {
				log.Printf("atalho: tecla do cadeado -> cofre")
				go cofreLigar(true)
			}
			return 1
		}
	}
	r, _, _ := pCallNextHookEx.Call(0, uintptr(code), wparam, lparam)
	return r
})

func pastaAmbienteTrabalho() string {
	if p, _, ok := regQuery(`Software\Microsoft\Windows\CurrentVersion\Explorer\Shell Folders`, "Desktop"); ok && p != "" {
		return p
	}
	return filepath.Join(pastaUtilizador(), "Desktop")
}

// criarFicheiroCofre põe no ambiente de trabalho um "Cofre.vbs" que liga o cofre
// sem abrir janela nenhuma.
func criarFicheiroCofre(exe string) {
	p := filepath.Join(pastaAmbienteTrabalho(), "Cofre.vbs")
	txt := "CreateObject(\"WScript.Shell\").Run \"\"\"" + exe + "\"\" --cofre\", 0, False\r\n"
	if b, err := os.ReadFile(p); err == nil && string(b) == txt {
		return
	}
	if err := os.WriteFile(p, []byte(txt), 0o644); err != nil {
		log.Printf("atalho: não consegui criar %s: %v", p, err)
	}
}

// pedidoCofreFich: o "ecra-remoto.exe --cofre" deixa este ficheiro e sai; o agente apanha-o.
func pedidoCofreFich() string { return filepath.Join(pastaApp(), "cofre-pedido") }

func vigiarAtalhosCofre(exe string) {
	criarFicheiroCofre(exe)

	// pedidos vindos do ficheiro do ambiente de trabalho
	go func() {
		for {
			if _, err := os.Stat(pedidoCofreFich()); err == nil {
				os.Remove(pedidoCofreFich())
				log.Printf("atalho: pedido de cofre pelo ficheiro")
				cofreLigar(true)
			}
			time.Sleep(400 * time.Millisecond)
		}
	}()

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := regSetDWORD(chavePolSistema, "DisableLockWorkstation", 1); err != nil {
		log.Printf("atalho: o Windows não deixou desligar o Windows+L (%v) — a tecla do cadeado fica com o bloqueio normal", err)
	} else if _, v, ok := regQuery(chavePolSistema, "DisableLockWorkstation"); ok && v == 1 {
		cadeadoAtivo = true
		log.Printf("atalho: tecla do cadeado passa a ligar o cofre")
	}

	hinst, _, _ := pGetModuleHandleW.Call(0)
	if h, _, _ := pSetWindowsHookExW.Call(whKeyboardLL, cbAtalho, hinst, 0); h == 0 {
		log.Printf("atalho: gancho do teclado falhou")
	}
	if r, _, _ := pRegisterHotKey.Call(0, 1, modControl|modAlt|modNoRepeat, vkC); r == 0 {
		log.Printf("atalho: Ctrl+Alt+C ocupado por outro programa")
	}

	var m msg
	for {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			return
		}
		if m.Message == wmHotkey {
			log.Printf("atalho: Ctrl+Alt+C -> cofre")
			go cofreLigar(true)
			continue
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}
