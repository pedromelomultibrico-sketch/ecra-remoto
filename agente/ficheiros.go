//go:build windows

package main

import (
	"encoding/base64"
	"log"
	"os"
	"path/filepath"
)

func decodeB64(s string) []byte {
	b, _ := base64.StdEncoding.DecodeString(s)
	return b
}

const parteFich = 48 * 1024 // bytes por parte (vai a base64, ~64 KB por mensagem)

// enviarFicheiroParaTablet lê um ficheiro do PC e manda-o em partes para o visor.
func enviarFicheiroParaTablet(ws *WS, caminho string) {
	st, err := os.Stat(caminho)
	if err != nil || st.IsDir() {
		ws.Texto(mustJSON(map[string]any{"t": "aviso", "m": "Ficheiro não encontrado."}))
		return
	}
	if st.Size() > 200<<20 {
		ws.Texto(mustJSON(map[string]any{"t": "aviso", "m": "Ficheiro demasiado grande (máx. 200 MB)."}))
		return
	}
	f, err := os.Open(caminho)
	if err != nil {
		ws.Texto(mustJSON(map[string]any{"t": "aviso", "m": err.Error()}))
		return
	}
	defer f.Close()
	nome := filepath.Base(caminho)
	ws.Texto(mustJSON(map[string]any{"t": "fich_inicio", "nome": nome, "tam": st.Size()}))
	buf := make([]byte, parteFich)
	seq := 0
	for {
		n, e := f.Read(buf)
		if n > 0 {
			ws.Texto(mustJSON(map[string]any{"t": "fich_parte", "nome": nome, "seq": seq,
				"dados": base64.StdEncoding.EncodeToString(buf[:n]), "fim": e != nil}))
			seq++
		}
		if e != nil {
			break
		}
	}
	log.Printf("ficheiro %s enviado (%d partes)", nome, seq)
}
