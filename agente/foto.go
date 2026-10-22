//go:build windows

package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/jpeg"
)

// respostaFoto — o campo T tem de ser o primeiro: o servidor reconhece a
// resposta por começar exatamente por {"t":"foto"
type respostaFoto struct {
	T    string `json:"t"`
	Rid  string `json:"rid"`
	W    int    `json:"w,omitempty"`
	H    int    `json:"h,omitempty"`
	Mon  int    `json:"mon"`
	Mons int    `json:"mons"`
	JPEG string `json:"jpeg,omitempty"`
	Erro string `json:"erro,omitempty"`
}

// enviarFoto tira uma captura completa (com captor próprio, para não
// interferir com a transmissão para o tablet) e devolve-a em JPEG base64.
func enviarFoto(ws *WS, rid string, pedido int, monAtual int) {
	r := respostaFoto{T: "foto", Rid: rid}
	defer func() {
		if e := recover(); e != nil {
			r.Erro = "falha na captura"
			b, _ := json.Marshal(r)
			ws.Texto(string(b))
		}
	}()
	ms := listarMonitores()
	if len(ms) == 0 {
		ms = []Monitor{ecraVirtual()}
	}
	mi := pedido
	if mi < 0 || mi >= len(ms) {
		mi = monAtual
	}
	if mi < 0 || mi >= len(ms) {
		mi = 0
	}
	m := ms[mi]
	r.Mon, r.Mons = mi, len(ms)
	larg, alt := dimensao(m, 0)
	c := &Captor{}
	defer c.liberar()
	if err := c.capturar(m, larg, alt); err != nil {
		r.Erro = err.Error()
	} else {
		img := image.NewRGBA(image.Rect(0, 0, larg, alt))
		src := c.bits
		for i := 0; i+3 < len(src) && i+3 < len(img.Pix); i += 4 {
			img.Pix[i] = src[i+2]
			img.Pix[i+1] = src[i+1]
			img.Pix[i+2] = src[i]
			img.Pix[i+3] = 255
		}
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 70}); err != nil {
			r.Erro = err.Error()
		} else {
			r.W, r.H = larg, alt
			r.JPEG = base64.StdEncoding.EncodeToString(buf.Bytes())
		}
	}
	b, _ := json.Marshal(r)
	ws.Texto(string(b))
}
