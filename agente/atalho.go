//go:build windows

package main

// Atalhos LOCAIS para ligar o cofre no próprio PC, sem administrador e sem o que
// o Defender apanha (nada de ganchos de teclado nem .vbs):
//   1) Ctrl+Alt+C — atalho global normal (RegisterHotKey), liga o cofre;
//   2) o argumento --cofre: um atalho (.lnk) no ambiente de trabalho que corre
//      "ecra-remoto.exe --cofre" deixa um pedido ao agente que já corre.

import (
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"
	"unsafe"
)

var pRegisterHotKey = user32.NewProc("RegisterHotKey")

const (
	atalhoModAlt     = 0x0001
	atalhoModControl = 0x0002
	atalhoModNoRep   = 0x4000
	atalhoVkC        = 0x43
	atalhoWmHotkey   = 0x0312
)

func pedidoCofreFich() string { return filepath.Join(pastaApp(), "cofre-pedido") }

func vigiarAtalhosCofre() {
	// pedidos vindos do atalho do ambiente de trabalho (ecra-remoto.exe --cofre)
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
	if r, _, _ := pRegisterHotKey.Call(0, 1, atalhoModControl|atalhoModAlt|atalhoModNoRep, atalhoVkC); r == 0 {
		log.Printf("atalho: Ctrl+Alt+C ocupado por outro programa")
	}
	var m cofreMsg
	for {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			return
		}
		if m.Message == atalhoWmHotkey {
			log.Printf("atalho: Ctrl+Alt+C -> cofre")
			go cofreLigar(true)
			continue
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}
