package main

import (
	"bufio"
	"crypto/rand"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// WS é um cliente WebSocket mínimo (só biblioteca padrão), que respeita o proxy do Windows.
type WS struct {
	rw  io.ReadWriteCloser
	br  *bufio.Reader
	mu  sync.Mutex
	fim bool
}

const (
	opCont   = 0
	opTexto  = 1
	opBin    = 2
	opFechar = 8
	opPing   = 9
	opPong   = 10
)

func ligarWS(endereco string, proxy func(*http.Request) (*url.URL, error)) (*WS, error) {
	chaveRaw := make([]byte, 16)
	rand.Read(chaveRaw)
	chave := base64.StdEncoding.EncodeToString(chaveRaw)

	u, err := url.Parse(endereco)
	if err != nil {
		return nil, err
	}
	switch u.Scheme {
	case "wss":
		u.Scheme = "https"
	case "ws":
		u.Scheme = "http"
	}
	tr := &http.Transport{
		Proxy:               proxy,
		DialContext:         (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout: 15 * time.Second,
		ForceAttemptHTTP2:   false,
		TLSNextProto:        map[string]func(string, *tls.Conn) http.RoundTripper{},
	}
	cli := &http.Client{Transport: tr, Timeout: 0}
	req, _ := http.NewRequest("GET", u.String(), nil)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", chave)
	req.Header.Set("User-Agent", "EcraRemoto/"+versao)
	resp, err := cli.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		resp.Body.Close()
		return nil, fmt.Errorf("o servidor respondeu %s", resp.Status)
	}
	h := sha1.Sum([]byte(chave + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	if resp.Header.Get("Sec-WebSocket-Accept") != base64.StdEncoding.EncodeToString(h[:]) {
		resp.Body.Close()
		return nil, errors.New("resposta WebSocket inválida")
	}
	rw, ok := resp.Body.(io.ReadWriteCloser)
	if !ok {
		resp.Body.Close()
		return nil, errors.New("ligação sem modo bidirecional")
	}
	return &WS{rw: rw, br: bufio.NewReaderSize(rw, 64*1024)}, nil
}

func (w *WS) escreverFrame(op byte, dados []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.fim {
		return io.ErrClosedPipe
	}
	var cab [14]byte
	cab[0] = 0x80 | op
	n := len(dados)
	i := 2
	switch {
	case n < 126:
		cab[1] = 0x80 | byte(n)
	case n < 65536:
		cab[1] = 0x80 | 126
		binary.BigEndian.PutUint16(cab[2:], uint16(n))
		i = 4
	default:
		cab[1] = 0x80 | 127
		binary.BigEndian.PutUint64(cab[2:], uint64(n))
		i = 10
	}
	// máscara a zeros: obrigatória no cliente, mas por TLS não precisa de ser aleatória
	i += 4
	if _, err := w.rw.Write(cab[:i]); err != nil {
		return err
	}
	if n > 0 {
		if _, err := w.rw.Write(dados); err != nil {
			return err
		}
	}
	return nil
}

func (w *WS) Texto(s string) error   { return w.escreverFrame(opTexto, []byte(s)) }
func (w *WS) Binario(b []byte) error { return w.escreverFrame(opBin, b) }

// Ler devolve a próxima mensagem completa (texto ou binária); trata do ping/pong sozinho.
func (w *WS) Ler() (op byte, dados []byte, err error) {
	var msg []byte
	var opMsg byte
	for {
		var c [2]byte
		if _, err = io.ReadFull(w.br, c[:]); err != nil {
			return
		}
		fin := c[0]&0x80 != 0
		o := c[0] & 0x0F
		mascara := c[1]&0x80 != 0
		n := uint64(c[1] & 0x7F)
		if n == 126 {
			var e [2]byte
			if _, err = io.ReadFull(w.br, e[:]); err != nil {
				return
			}
			n = uint64(binary.BigEndian.Uint16(e[:]))
		} else if n == 127 {
			var e [8]byte
			if _, err = io.ReadFull(w.br, e[:]); err != nil {
				return
			}
			n = binary.BigEndian.Uint64(e[:])
		}
		if n > 64<<20 {
			err = errors.New("mensagem demasiado grande")
			return
		}
		var mk [4]byte
		if mascara {
			if _, err = io.ReadFull(w.br, mk[:]); err != nil {
				return
			}
		}
		p := make([]byte, n)
		if _, err = io.ReadFull(w.br, p); err != nil {
			return
		}
		if mascara {
			for i := range p {
				p[i] ^= mk[i%4]
			}
		}
		switch o {
		case opPing:
			w.escreverFrame(opPong, p)
			continue
		case opPong:
			continue
		case opFechar:
			err = io.EOF
			w.Fechar()
			return
		case opTexto, opBin:
			opMsg = o
			msg = p
		case opCont:
			msg = append(msg, p...)
		}
		if fin {
			return opMsg, msg, nil
		}
	}
}

func (w *WS) Fechar() {
	w.mu.Lock()
	if !w.fim {
		w.fim = true
		w.rw.Close()
	}
	w.mu.Unlock()
}
