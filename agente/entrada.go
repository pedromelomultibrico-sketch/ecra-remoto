//go:build windows

package main

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

func enviarRato(flags uint32, dados int32) {
	in := mouseInput{Typ: INPUT_MOUSE, Flags: flags, MouseData: uint32(dados)}
	pSendInput.Call(1, uintptr(unsafe.Pointer(&in)), unsafe.Sizeof(in))
}

func moverRato(x, y int) {
	pSetCursorPos.Call(uintptr(x), uintptr(y))
}

func botaoRato(botao int, baixo bool) {
	var f uint32
	switch botao {
	case 2:
		f = MOUSEEVENTF_RIGHTUP
		if baixo {
			f = MOUSEEVENTF_RIGHTDOWN
		}
	case 1:
		f = MOUSEEVENTF_MIDDLEUP
		if baixo {
			f = MOUSEEVENTF_MIDDLEDOWN
		}
	default:
		f = MOUSEEVENTF_LEFTUP
		if baixo {
			f = MOUSEEVENTF_LEFTDOWN
		}
	}
	enviarRato(f, 0)
}

func rodaRato(dy, dx int) {
	if dy != 0 {
		enviarRato(MOUSEEVENTF_WHEEL, int32(dy))
	}
	if dx != 0 {
		enviarRato(MOUSEEVENTF_HWHEEL, int32(dx))
	}
}

var teclasEstendidas = map[uint16]bool{
	0x21: true, 0x22: true, 0x23: true, 0x24: true, // PgUp PgDn End Home
	0x25: true, 0x26: true, 0x27: true, 0x28: true, // setas
	0x2D: true, 0x2E: true, // Insert Delete
	0x5B: true, 0x5C: true, 0x5D: true, // Win, Menu
	0xA3: true, 0xA5: true, // Ctrl e Alt da direita
	0x6F: true, 0x90: true, 0x2C: true, // / do teclado numérico, NumLock, PrintScreen
}

func teclaVK(vk uint16, baixo bool) {
	scan, _, _ := pMapVirtualKeyW.Call(uintptr(vk), 0)
	in := keybdInput{Typ: INPUT_KEYBOARD, Vk: vk, Scan: uint16(scan)}
	if teclasEstendidas[vk] {
		in.Flags |= KEYEVENTF_EXTENDEDKEY
	}
	if !baixo {
		in.Flags |= KEYEVENTF_KEYUP
	}
	pSendInput.Call(1, uintptr(unsafe.Pointer(&in)), unsafe.Sizeof(in))
}

func carregarVK(vk uint16) {
	teclaVK(vk, true)
	teclaVK(vk, false)
}

// escreverTexto escreve qualquer texto (acentos incluídos) como se fosse teclado Unicode.
func escreverTexto(s string) {
	for _, r := range s {
		switch r {
		case '\n', '\r':
			carregarVK(0x0D)
			continue
		case '\t':
			carregarVK(0x09)
			continue
		case '\b':
			carregarVK(0x08)
			continue
		}
		for _, u := range syscall.StringToUTF16(string(r)) {
			if u == 0 {
				continue
			}
			a := keybdInput{Typ: INPUT_KEYBOARD, Scan: u, Flags: KEYEVENTF_UNICODE}
			b := keybdInput{Typ: INPUT_KEYBOARD, Scan: u, Flags: KEYEVENTF_UNICODE | KEYEVENTF_KEYUP}
			pSendInput.Call(1, uintptr(unsafe.Pointer(&a)), unsafe.Sizeof(a))
			pSendInput.Call(1, uintptr(unsafe.Pointer(&b)), unsafe.Sizeof(b))
		}
	}
}

var vkModificador = map[string]uint16{"ctrl": 0x11, "alt": 0x12, "shift": 0x10, "win": 0x5B}

// combinacao carrega modificadores + uma tecla (por código VK ou por carácter).
func combinacao(mods []string, vk uint16, ch string) {
	if vk == 0 && ch != "" {
		r := []rune(ch)[0]
		v, _, _ := pVkKeyScanW.Call(uintptr(r))
		if int16(v) == -1 {
			escreverTexto(ch)
			return
		}
		vk = uint16(v & 0xFF)
		sh := (v >> 8) & 0xFF
		if sh&1 != 0 {
			mods = append(mods, "shift")
		}
	}
	var feitos []uint16
	for _, m := range mods {
		if k, ok := vkModificador[m]; ok {
			teclaVK(k, true)
			feitos = append(feitos, k)
		}
	}
	if vk != 0 {
		carregarVK(vk)
	}
	for i := len(feitos) - 1; i >= 0; i-- {
		teclaVK(feitos[i], false)
	}
}

// --- área de transferência ---

func abrirClipboard() bool {
	for i := 0; i < 10; i++ {
		r, _, _ := pOpenClipboard.Call(0)
		if r != 0 {
			return true
		}
		time.Sleep(30 * time.Millisecond)
	}
	return false
}

func lerClipboard() (string, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if !abrirClipboard() {
		return "", errors.New("área de transferência ocupada")
	}
	defer pCloseClipboard.Call()
	h, _, _ := pGetClipboardData.Call(CF_UNICODETEXT)
	if h == 0 {
		return "", nil
	}
	p, _, _ := pGlobalLock.Call(h)
	if p == 0 {
		return "", nil
	}
	defer pGlobalUnlock.Call(h)
	tam, _, _ := pGlobalSize.Call(h)
	n := int(tam / 2)
	s := unsafe.Slice((*uint16)(unsafe.Pointer(p)), n)
	for i, c := range s {
		if c == 0 {
			s = s[:i]
			break
		}
	}
	return syscall.UTF16ToString(s), nil
}

func escreverClipboard(texto string) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	u, err := syscall.UTF16FromString(texto)
	if err != nil {
		return err
	}
	if !abrirClipboard() {
		return errors.New("área de transferência ocupada")
	}
	defer pCloseClipboard.Call()
	pEmptyClipboard.Call()
	h, _, _ := pGlobalAlloc.Call(GMEM_MOVEABLE, uintptr(len(u)*2))
	if h == 0 {
		return errors.New("sem memória")
	}
	p, _, _ := pGlobalLock.Call(h)
	copy(unsafe.Slice((*uint16)(unsafe.Pointer(p)), len(u)), u)
	pGlobalUnlock.Call(h)
	r, _, _ := pSetClipboardData.Call(CF_UNICODETEXT, h)
	if r == 0 {
		return errors.New("não foi possível copiar")
	}
	return nil
}

func sequenciaClipboard() uint32 {
	r, _, _ := pGetClipboardSequenceNumber.Call()
	return uint32(r)
}

// --- sistema ---

func acaoSistema(a string) error {
	esconder := &syscall.SysProcAttr{HideWindow: true}
	run := func(args ...string) error {
		c := exec.Command("shutdown", args...)
		c.SysProcAttr = esconder
		return c.Start()
	}
	switch a {
	case "bloquear":
		pLockWorkStation.Call()
	case "reiniciar":
		return run("/r", "/t", "5")
	case "desligar":
		return run("/s", "/t", "5")
	case "sair":
		return run("/l")
	case "suspender":
		c := exec.Command("rundll32.exe", "powrprof.dll,SetSuspendState", "0,1,0")
		c.SysProcAttr = esconder
		return c.Start()
	case "cancelar":
		return run("/a")
	case "gestor":
		combinacao([]string{"ctrl", "shift"}, 0x1B, "")
	case "explorador":
		shellOpen("explorer.exe")
	}
	return nil
}

func manterAcordado(sim bool) {
	if sim {
		pSetThreadExecutionState.Call(ES_CONTINUOUS | ES_SYSTEM_REQUIRED | ES_DISPLAY_REQUIRED)
	} else {
		pSetThreadExecutionState.Call(ES_CONTINUOUS)
	}
}

func bateria() (int, bool) {
	var ps powerStatus
	r, _, _ := pGetSystemPowerStatus.Call(uintptr(unsafe.Pointer(&ps)))
	if r == 0 || ps.BatteryLifePercent == 255 {
		return -1, ps.ACLineStatus == 1
	}
	return int(ps.BatteryLifePercent), ps.ACLineStatus == 1
}

func unidades() []string {
	buf := make([]uint16, 256)
	n, _, _ := pGetLogicalDriveStringsW.Call(uintptr(len(buf)), uintptr(unsafe.Pointer(&buf[0])))
	var r []string
	ini := 0
	for i := 0; i < int(n); i++ {
		if buf[i] == 0 {
			if i > ini {
				r = append(r, syscall.UTF16ToString(buf[ini:i]))
			}
			ini = i + 1
		}
	}
	return r
}

func pastaUtilizador() string {
	h, _ := os.UserHomeDir()
	return h
}
