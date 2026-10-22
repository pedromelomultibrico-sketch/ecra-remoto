//go:build windows

package main

import (
	"bytes"
	"errors"
	"image"
	"image/jpeg"
	"syscall"
	"unsafe"
)

type Monitor struct {
	X, Y, W, H int
	Primario   bool
}

var monitoresTmp []Monitor
var cbMonitores = syscall.NewCallback(func(hmon, hdc, lprc, lparam uintptr) uintptr {
	var mi monitorInfo
	mi.CbSize = uint32(unsafe.Sizeof(mi))
	pGetMonitorInfoW.Call(hmon, uintptr(unsafe.Pointer(&mi)))
	r := mi.RcMonitor
	monitoresTmp = append(monitoresTmp, Monitor{X: int(r.Left), Y: int(r.Top), W: int(r.Right - r.Left), H: int(r.Bottom - r.Top), Primario: mi.DwFlags&1 == 1})
	return 1
})

func listarMonitores() []Monitor {
	monitoresTmp = nil
	pEnumDisplayMonitors.Call(0, 0, cbMonitores, 0)
	ms := monitoresTmp
	// o principal primeiro
	for i := range ms {
		if ms[i].Primario && i != 0 {
			ms[0], ms[i] = ms[i], ms[0]
		}
	}
	return ms
}

func ecraVirtual() Monitor {
	x, _, _ := pGetSystemMetrics.Call(SM_XVSCR)
	y, _, _ := pGetSystemMetrics.Call(SM_YVSCR)
	w, _, _ := pGetSystemMetrics.Call(SM_CXVSCR)
	h, _, _ := pGetSystemMetrics.Call(SM_CYVSCR)
	return Monitor{X: int(int32(x)), Y: int(int32(y)), W: int(int32(w)), H: int(int32(h))}
}

// Captor guarda o DC e o bitmap entre capturas, para não os recriar a cada imagem.
type Captor struct {
	w, h    int
	memDC   uintptr
	bmp     uintptr
	old     uintptr
	bits    []byte
	anterior []byte
}

func (c *Captor) liberar() {
	if c.memDC != 0 {
		pSelectObject.Call(c.memDC, c.old)
		pDeleteObject.Call(c.bmp)
		pDeleteDC.Call(c.memDC)
	}
	c.memDC, c.bmp, c.bits, c.anterior = 0, 0, nil, nil
}

func (c *Captor) preparar(w, h int) error {
	if c.memDC != 0 && c.w == w && c.h == h {
		return nil
	}
	c.liberar()
	ecra, _, _ := pGetDC.Call(0)
	defer pReleaseDC.Call(0, ecra)
	mem, _, _ := pCreateCompatibleDC.Call(ecra)
	if mem == 0 {
		return errors.New("CreateCompatibleDC falhou")
	}
	var bi bitmapInfo
	bi.Header = bitmapInfoHeader{BiSize: 40, BiWidth: int32(w), BiHeight: -int32(h), BiPlanes: 1, BiBitCount: 32, BiCompression: BI_RGB}
	var ptr uintptr
	bmp, _, _ := pCreateDIBSection.Call(mem, uintptr(unsafe.Pointer(&bi)), DIB_RGB, uintptr(unsafe.Pointer(&ptr)), 0, 0)
	if bmp == 0 || ptr == 0 {
		pDeleteDC.Call(mem)
		return errors.New("CreateDIBSection falhou")
	}
	old, _, _ := pSelectObject.Call(mem, bmp)
	pSetStretchBltMode.Call(mem, HALFTONE)
	pSetBrushOrgEx.Call(mem, 0, 0, 0)
	c.w, c.h, c.memDC, c.bmp, c.old = w, h, mem, bmp, old
	c.bits = unsafe.Slice((*byte)(unsafe.Pointer(ptr)), w*h*4)
	c.anterior = nil
	return nil
}

// capturar copia a área m do ecrã para o bitmap, reduzida para w x h.
func (c *Captor) capturar(m Monitor, w, h int) error {
	if err := c.preparar(w, h); err != nil {
		return err
	}
	ecra, _, _ := pGetDC.Call(0)
	if ecra == 0 {
		return errors.New("sem acesso ao ecrã")
	}
	defer pReleaseDC.Call(0, ecra)
	var r uintptr
	if w == m.W && h == m.H {
		r, _, _ = pBitBlt.Call(c.memDC, 0, 0, uintptr(w), uintptr(h), ecra, uintptr(m.X), uintptr(m.Y), SRCCOPY)
	} else {
		r, _, _ = pStretchBlt.Call(c.memDC, 0, 0, uintptr(w), uintptr(h), ecra, uintptr(m.X), uintptr(m.Y), uintptr(m.W), uintptr(m.H), SRCCOPY)
	}
	pGdiFlush.Call()
	if r == 0 {
		return errors.New("ecrã indisponível (PC bloqueado ou janela protegida)")
	}
	return nil
}

type Faixa struct {
	X, Y, W, H int
	JPEG       []byte
}

const bloco = 64

// diferencas compara com a imagem anterior e devolve as faixas alteradas, já em JPEG.
// Divide o ecrã em faixas horizontais de 64 px; em cada uma manda só o troço entre
// o primeiro e o último bloco que mudaram.
func (c *Captor) diferencas(qualidade int, tudo bool) ([]Faixa, error) {
	w, h := c.w, c.h
	cur := c.bits
	prev := c.anterior
	if prev == nil || len(prev) != len(cur) {
		tudo = true
	}
	var faixas []Faixa
	colunas := (w + bloco - 1) / bloco
	for y0 := 0; y0 < h; y0 += bloco {
		y1 := y0 + bloco
		if y1 > h {
			y1 = h
		}
		minC, maxC := -1, -1
		if tudo {
			minC, maxC = 0, colunas-1
		} else {
			for cx := 0; cx < colunas; cx++ {
				x0 := cx * bloco
				x1 := x0 + bloco
				if x1 > w {
					x1 = w
				}
				mudou := false
				for y := y0; y < y1; y++ {
					o := (y*w + x0) * 4
					f := (y*w + x1) * 4
					if !bytes.Equal(cur[o:f], prev[o:f]) {
						mudou = true
						break
					}
				}
				if mudou {
					if minC < 0 {
						minC = cx
					}
					maxC = cx
				}
			}
		}
		if minC < 0 {
			continue
		}
		x0 := minC * bloco
		x1 := (maxC + 1) * bloco
		if x1 > w {
			x1 = w
		}
		img := image.NewRGBA(image.Rect(0, 0, x1-x0, y1-y0))
		for y := y0; y < y1; y++ {
			src := cur[(y*w+x0)*4 : (y*w+x1)*4]
			dst := img.Pix[(y-y0)*img.Stride : (y-y0)*img.Stride+(x1-x0)*4]
			for i := 0; i < len(src); i += 4 {
				dst[i] = src[i+2]
				dst[i+1] = src[i+1]
				dst[i+2] = src[i]
				dst[i+3] = 255
			}
		}
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: qualidade}); err != nil {
			return nil, err
		}
		faixas = append(faixas, Faixa{X: x0, Y: y0, W: x1 - x0, H: y1 - y0, JPEG: buf.Bytes()})
	}
	if c.anterior == nil || len(c.anterior) != len(cur) {
		c.anterior = make([]byte, len(cur))
	}
	copy(c.anterior, cur)
	return faixas, nil
}

func posicaoRato() (int, int) {
	var p point
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	return int(p.X), int(p.Y)
}
