//go:build windows

package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var versao = "0" // definido na compilação (-X main.versao=N)

const (
	servidor  = "wss://ecra-remoto.pedromelomultibrico.workers.dev"
	repo      = "pedromelomultibrico-sketch/ecra-remoto"
	nomeExe   = "ecra-remoto.exe"
	chaveRun  = `Software\Microsoft\Windows\CurrentVersion\Run`
	valorRun  = "EcraRemoto"
	tituloBox = "Ecrã Remoto"
)

type Config struct {
	ID       string `json:"id"`
	Senha    string `json:"senha"`
	Segredo  string `json:"segredo"`
	Acordado bool   `json:"acordado"`
	Qualidade string `json:"qualidade"`
}

func pastaApp() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base = filepath.Join(pastaUtilizador(), "AppData", "Local")
	}
	return filepath.Join(base, "EcraRemoto")
}

func caminhoConfig() string { return filepath.Join(pastaApp(), "config.json") }

func aleatorioDigitos(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		d, _ := rand.Int(rand.Reader, big.NewInt(10))
		b.WriteByte(byte('0' + d.Int64()))
	}
	return b.String()
}

func novaSenha() string {
	const letras = "abcdefghjkmnpqrstuvwxyz23456789"
	var b strings.Builder
	for i := 0; i < 8; i++ {
		d, _ := rand.Int(rand.Reader, big.NewInt(int64(len(letras))))
		b.WriteByte(letras[d.Int64()])
	}
	return b.String()
}

func lerConfig() (*Config, error) {
	b, err := os.ReadFile(caminhoConfig())
	if err != nil {
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func gravarConfig(c *Config) error {
	os.MkdirAll(pastaApp(), 0o755)
	b, _ := json.MarshalIndent(c, "", "  ")
	tmp := caminhoConfig() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, caminhoConfig())
}

func obterConfig() *Config {
	c, err := lerConfig()
	if err == nil && len(c.ID) == 9 && c.Senha != "" && c.Segredo != "" {
		return c
	}
	seg := make([]byte, 24)
	rand.Read(seg)
	c = &Config{ID: aleatorioDigitos(9), Senha: novaSenha(), Segredo: hex.EncodeToString(seg), Qualidade: "equilibrado"}
	gravarConfig(c)
	return c
}

func hashSenha(id, senha string) string {
	h := sha256.Sum256([]byte(id + ":" + senha))
	return hex.EncodeToString(h[:])
}

func idBonito(id string) string {
	if len(id) != 9 {
		return id
	}
	return id[0:3] + " " + id[3:6] + " " + id[6:9]
}

// proxyWindows usa o proxy configurado no Windows (se houver), como o browser.
func proxyWindows(req *http.Request) (*url.URL, error) {
	if u, err := http.ProxyFromEnvironment(req); u != nil || err != nil {
		return u, err
	}
	const k = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`
	_, ativo, ok := regQuery(k, "ProxyEnable")
	if !ok || ativo == 0 {
		return nil, nil
	}
	srv, _, ok := regQuery(k, "ProxyServer")
	if !ok || srv == "" {
		return nil, nil
	}
	// pode vir "http=a:80;https=b:8080" ou só "a:8080"
	escolhido := srv
	if strings.Contains(srv, "=") {
		escolhido = ""
		for _, p := range strings.Split(srv, ";") {
			kv := strings.SplitN(p, "=", 2)
			if len(kv) == 2 && (kv[0] == "https" || (escolhido == "" && kv[0] == "http")) {
				escolhido = kv[1]
			}
		}
	}
	if escolhido == "" {
		return nil, nil
	}
	if !strings.Contains(escolhido, "://") {
		escolhido = "http://" + escolhido
	}
	return url.Parse(escolhido)
}

func mutexUnico() bool {
	h, _, e := pCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(u16("Local\\EcraRemotoAgente"))))
	if h == 0 {
		return false
	}
	if errno, ok := e.(syscall.Errno); ok && errno == ERROR_ALREADY_EXISTS {
		pCloseHandle.Call(h)
		return false
	}
	return true
}

func ativarArranque(exe string) {
	regSetString(chaveRun, valorRun, `"`+exe+`" --arranque`)
}

func iniciar(exe string, args ...string) error {
	c := exec.Command(exe, args...)
	c.Dir = filepath.Dir(exe)
	return c.Start()
}

func copiarFicheiro(de, para string) error {
	in, err := os.Open(de)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := para + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	out.Close()
	os.Remove(para + ".old")
	if _, err := os.Stat(para); err == nil {
		if err := os.Rename(para, para+".old"); err != nil {
			return err
		}
	}
	return os.Rename(tmp, para)
}

func mostrarInfo(c *Config, primeira bool) {
	txt := fmt.Sprintf("O Ecrã Remoto está ATIVO neste PC.\n\n   ID:      %s\n   Senha:  %s\n\n"+
		"No tablet, abra a app Ecrã Remoto e adicione este PC com o ID e a senha.\n"+
		"O programa arranca sozinho sempre que entrar no Windows.\n\n", idBonito(c.ID), c.Senha)
	if primeira {
		messageBox(tituloBox, txt+"(Pode voltar a ver estes dados abrindo o programa outra vez.)", MB_OK|MB_ICONINFORMATION)
		return
	}
	r := messageBox(tituloBox, txt+"SIM  = gerar uma senha nova\nNÃO  = fechar esta janela\nCANCELAR = desligar o Ecrã Remoto neste PC", MB_YESNOCANCEL|MB_ICONINFORMATION)
	switch r {
	case IDYES:
		c.Senha = novaSenha()
		gravarConfig(c)
		messageBox(tituloBox, fmt.Sprintf("Senha nova: %s\n\nID: %s", c.Senha, idBonito(c.ID)), MB_OK|MB_ICONINFORMATION)
	case IDCANCEL:
		os.WriteFile(filepath.Join(pastaApp(), "parar"), []byte("1"), 0o644)
		regDelete(chaveRun, valorRun)
		messageBox(tituloBox, "O Ecrã Remoto foi desligado e já não arranca com o Windows.\nPara voltar a ligar, abra o programa outra vez.", MB_OK|MB_ICONWARNING)
	}
}

func prepararLog() {
	os.MkdirAll(pastaApp(), 0o755)
	p := filepath.Join(pastaApp(), "registo.txt")
	if st, err := os.Stat(p); err == nil && st.Size() > 2<<20 {
		os.Rename(p, p+".1")
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err == nil {
		log.SetOutput(f)
	}
	log.SetFlags(log.LstdFlags)
}

func main() {
	// coordenadas em píxeis reais, mesmo com o Windows em 125%/150%
	if pSetProcessDpiAwarenessContext.Find() == nil {
		pSetProcessDpiAwarenessContext.Call(^uintptr(3)) // PER_MONITOR_AWARE_V2 (-4)
	} else {
		pSetProcessDPIAware.Call()
	}
	prepararLog()

	args := strings.Join(os.Args[1:], " ")
	// "Cofre" no ambiente de trabalho: deixa o pedido ao agente que já corre e sai.
	if strings.Contains(args, "--cofre") {
		os.MkdirAll(pastaApp(), 0o755)
		os.WriteFile(pedidoCofreFich(), []byte("1"), 0o644)
		return
	}
	exe, _ := os.Executable()
	exe, _ = filepath.Abs(exe)
	instalado := filepath.Join(pastaApp(), nomeExe)
	estaInstalado := strings.EqualFold(exe, instalado)

	// 1) aberto a partir de outro sítio (Transferências ou atualizador): instala na pasta do utilizador
	if !estaInstalado {
		atualizacao := strings.Contains(args, "--atualizar-de")
		obterConfig()
		// espera que a versão a correr saia (para libertar o ficheiro .exe)
		os.WriteFile(filepath.Join(pastaApp(), "parar"), []byte("1"), 0o644)
		for i := 0; i < 20 && !mutexUnico(); i++ {
			time.Sleep(500 * time.Millisecond)
		}
		os.Remove(filepath.Join(pastaApp(), "parar"))
		if err := copiarFicheiro(exe, instalado); err != nil {
			messageBox(tituloBox, "Não foi possível instalar na pasta do utilizador:\n"+err.Error(), MB_OK|MB_ICONWARNING)
			return
		}
		ativarArranque(instalado)
		arg := "--primeira"
		if atualizacao {
			arg = "--atualizado"
		}
		if err := iniciar(instalado, arg); err != nil {
			messageBox(tituloBox, "Não foi possível arrancar:\n"+err.Error(), MB_OK|MB_ICONWARNING)
		}
		return
	}

	// 2) é o programa instalado
	unico := mutexUnico()
	if !unico && (strings.Contains(args, "--atualizado") || strings.Contains(args, "--primeira")) {
		// espera que a versão anterior saia
		for i := 0; i < 40 && !unico; i++ {
			time.Sleep(500 * time.Millisecond)
			unico = mutexUnico()
		}
	}
	c := obterConfig()
	if !unico {
		// já está a correr: mostra o ID/senha e as opções
		mostrarInfo(c, false)
		return
	}
	os.Remove(filepath.Join(pastaApp(), "parar"))
	os.Remove(instalado + ".old")
	ativarArranque(instalado)
	log.Printf("Ecrã Remoto %s a arrancar (ID %s)", versao, c.ID)

	if strings.Contains(args, "--primeira") {
		go mostrarInfo(c, true)
	}
	go vigiarAtualizacoes(instalado)
	go vigiarAtalhosCofre(instalado)
	correr(c)
}
