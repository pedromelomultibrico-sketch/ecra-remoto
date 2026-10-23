//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type msgEntrada struct {
	T    string  `json:"t"`
	Mon  int     `json:"mon"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	Bot  int     `json:"bot"`
	DX   int     `json:"dx"`
	DY   int     `json:"dy"`
	VK   uint16  `json:"vk"`
	Ch   string  `json:"ch"`
	Mods []string `json:"mods"`
	Txt  string  `json:"txt"`
	Q    string  `json:"q"`
	Acao string  `json:"acao"`
	Nome string  `json:"nome"`
	// transferência de ficheiros
	Caminho string `json:"caminho"`
	Dados   string `json:"dados"`
	Seq     int    `json:"seq"`
	Fim     bool   `json:"fim"`
	// pedido de foto do Jarvis
	Rid string `json:"rid"`
	// modo cofre
	On bool `json:"on"`
}

var qualidades = map[string][2]int{ // {qualidade jpeg, fps alvo}
	"nitido":      {72, 18},
	"equilibrado": {55, 14},
	"poupanca":    {40, 8},
}

func correr(c *Config) {
	paraFich := filepath.Join(pastaApp(), "parar")
	for {
		if _, err := os.Stat(paraFich); err == nil {
			log.Printf("ficheiro 'parar' presente — a sair")
			return
		}
		if err := sessao(c); err != nil {
			log.Printf("ligação terminou: %v", err)
		}
		time.Sleep(5 * time.Second)
	}
}

func sessao(c *Config) error {
	ws, err := ligarWS(servidor+"/ws/agente/"+c.ID, proxyWindows)
	if err != nil {
		return err
	}
	defer ws.Fechar()

	if err := ws.Texto(mustJSON(map[string]any{"t": "ola", "segredo": c.Segredo, "hsenha": hashSenha(c.ID, c.Senha)})); err != nil {
		return err
	}
	_, b, err := ws.Ler()
	if err != nil {
		return err
	}
	var r struct{ T, M string }
	json.Unmarshal(b, &r)
	if r.T == "erro" {
		messageBox(tituloBox, "O servidor recusou a ligação:\n"+r.M, MB_OK|MB_ICONWARNING)
		return fmt.Errorf("recusado: %s", r.M)
	}
	if r.T != "bemvindo" {
		return fmt.Errorf("resposta inesperada: %s", r.T)
	}
	log.Printf("ligado ao servidor")

	nome, _ := os.Hostname()
	user := os.Getenv("USERNAME")
	ws.Texto(mustJSON(map[string]any{"t": "info", "nome": nome, "utilizador": user, "versao": versao}))

	cap := &Captor{}
	defer cap.liberar()
	manterAcordado(c.Acordado)
	defer manterAcordado(false)

	visor := false
	qualidade := c.Qualidade
	if qualidade == "" {
		qualidade = "equilibrado"
	}
	monIdx := 0
	chamadas := make(chan msgEntrada, 256)
	fimLeitura := make(chan error, 1)

	go func() {
		for {
			op, b, err := ws.Ler()
			if err != nil {
				fimLeitura <- err
				return
			}
			if op != opTexto {
				continue
			}
			var m msgEntrada
			if json.Unmarshal(b, &m) != nil {
				continue
			}
			if m.T == "ver" || m.T == "pausa" {
				chamadas <- m // tratados no laço principal (mexem no estado)
				continue
			}
			if m.T == "foto" {
				go enviarFoto(ws, m.Rid, m.Mon, monIdx)
				continue
			}
			tratarComando(ws, cap, &m, &monIdx)
		}
	}()

	var ultimaClip uint32 = sequenciaClipboard()
	tickClip := time.NewTicker(700 * time.Millisecond)
	defer tickClip.Stop()

	envia := func() {
		ms := listarMonitores()
		if len(ms) == 0 {
			ms = []Monitor{ecraVirtual()}
		}
		if monIdx >= len(ms) {
			monIdx = 0
		}
		q := qualidades[qualidade]
		if q[0] == 0 {
			q = qualidades["equilibrado"]
		}
		m := ms[monIdx]
		larg, alt := dimensao(m, q[0])
		if err := cap.capturar(m, larg, alt); err != nil {
			ws.Texto(mustJSON(map[string]any{"t": "aviso", "m": err.Error()}))
			return
		}
		faixas, err := cap.diferencas(q[0], false)
		if err != nil || len(faixas) == 0 {
			return
		}
		rx, ry := posicaoRato()
		ws.Texto(mustJSON(map[string]any{"t": "quadro", "w": larg, "h": alt, "mw": m.W, "mh": m.H,
			"n": len(faixas), "rx": rx - m.X, "ry": ry - m.Y, "mon": monIdx, "mons": len(ms)}))
		for _, f := range faixas {
			cab := fmt.Sprintf("%d,%d,%d,%d,", f.X, f.Y, f.W, f.H)
			ws.Binario(append([]byte(cab), f.JPEG...))
		}
	}

	intervalo := func() time.Duration {
		q := qualidades[qualidade]
		if q[1] == 0 {
			q[1] = 12
		}
		return time.Second / time.Duration(q[1])
	}
	tick := time.NewTicker(intervalo())
	defer tick.Stop()

	paraFich := filepath.Join(pastaApp(), "parar")
	for {
		select {
		case err := <-fimLeitura:
			return err
		case m := <-chamadas:
			switch m.T {
			case "ver":
				visor = true
				cap.anterior = nil // força quadro completo
				if m.Q != "" {
					qualidade = m.Q
					c.Qualidade = m.Q
					gravarConfig(c)
					tick.Reset(intervalo())
				}
				if m.Mon >= 0 {
					monIdx = m.Mon
				}
			case "pausa":
				visor = false
			}
		case <-tick.C:
			if _, err := os.Stat(paraFich); err == nil {
				ws.Texto(mustJSON(map[string]any{"t": "adeus"}))
				return fmt.Errorf("pedido de paragem")
			}
			if visor {
				envia()
			}
		case <-tickClip.C:
			if visor {
				if s := sequenciaClipboard(); s != ultimaClip {
					ultimaClip = s
					if txt, err := lerClipboard(); err == nil && txt != "" && len(txt) < 100000 {
						ws.Texto(mustJSON(map[string]any{"t": "clip", "txt": txt}))
					}
				}
			}
		}
	}
}

func dimensao(m Monitor, _ int) (int, int) {
	// limita o lado maior a 1600 px para poupar dados; mantém a proporção
	const maxLado = 1600
	w, h := m.W, m.H
	if w <= 0 || h <= 0 {
		return 1280, 720
	}
	if w > maxLado || h > maxLado {
		if w >= h {
			h = h * maxLado / w
			w = maxLado
		} else {
			w = w * maxLado / h
			h = maxLado
		}
	}
	return w, h
}

var recebendo = map[string]*os.File{}

func tratarComando(ws *WS, cap *Captor, m *msgEntrada, monIdx *int) {
	ms := listarMonitores()
	if len(ms) == 0 {
		ms = []Monitor{ecraVirtual()}
	}
	mi := *monIdx
	if mi >= len(ms) {
		mi = 0
	}
	mon := ms[mi]
	px := func() (int, int) { return mon.X + int(m.X*float64(mon.W)), mon.Y + int(m.Y*float64(mon.H)) }

	switch m.T {
	case "mover":
		x, y := px()
		moverRato(x, y)
	case "clique":
		x, y := px()
		moverRato(x, y)
		botaoRato(m.Bot, true)
		botaoRato(m.Bot, false)
	case "duplo":
		x, y := px()
		moverRato(x, y)
		for i := 0; i < 2; i++ {
			botaoRato(0, true)
			botaoRato(0, false)
		}
	case "baixo":
		x, y := px()
		moverRato(x, y)
		botaoRato(m.Bot, true)
	case "cima":
		botaoRato(m.Bot, false)
	case "roda":
		rodaRato(m.DY, m.DX)
	case "tecla":
		if len(m.Mods) > 0 || m.Ch != "" {
			combinacao(m.Mods, m.VK, m.Ch)
		} else if m.VK != 0 {
			carregarVK(m.VK)
		}
	case "texto":
		escreverTexto(m.Txt)
	case "clip_para_pc":
		if err := escreverClipboard(m.Txt); err != nil {
			ws.Texto(mustJSON(map[string]any{"t": "aviso", "m": "Não consegui copiar: " + err.Error()}))
		}
	case "colar":
		escreverClipboard(m.Txt)
		combinacao([]string{"ctrl"}, 0, "v")
	case "sistema":
		if err := acaoSistema(m.Acao); err != nil {
			ws.Texto(mustJSON(map[string]any{"t": "aviso", "m": err.Error()}))
		}
	case "cofre":
		cofreLigar(m.On)
		ws.Texto(mustJSON(map[string]any{"t": "cofre", "on": cofreAtivo()}))
	case "estado_pc":
		pct, ac := bateria()
		ws.Texto(mustJSON(map[string]any{"t": "estado_pc", "bateria": pct, "corrente": ac, "unidades": unidades(), "versao": versao}))
	case "ficheiro_do_pc":
		enviarFicheiroParaTablet(ws, m.Caminho)
	case "ficheiro_para_pc_inicio":
		os.MkdirAll(filepath.Join(pastaUtilizador(), "Downloads"), 0o755)
		dest := filepath.Join(pastaUtilizador(), "Downloads", sanear(m.Nome))
		f, err := os.Create(dest)
		if err != nil {
			ws.Texto(mustJSON(map[string]any{"t": "aviso", "m": "Não consegui guardar: " + err.Error()}))
			return
		}
		recebendo[m.Nome] = f
	case "ficheiro_para_pc_parte":
		if f := recebendo[m.Nome]; f != nil {
			f.Write(decodeB64(m.Dados))
			if m.Fim {
				f.Close()
				delete(recebendo, m.Nome)
				ws.Texto(mustJSON(map[string]any{"t": "aviso", "m": "Ficheiro guardado em Transferências: " + m.Nome}))
			}
		}
	}
}

func sanear(n string) string {
	n = filepath.Base(n)
	for _, c := range []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|"} {
		n = strings.ReplaceAll(n, c, "_")
	}
	if n == "" {
		n = "ficheiro"
	}
	return n
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
