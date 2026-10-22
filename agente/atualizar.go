//go:build windows

package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

const releasesLatest = "https://api.github.com/repos/" + repo + "/releases/latest"

// vigiarAtualizacoes verifica de hora a hora se há uma versão mais recente no GitHub
// e, se houver, descarrega-a, substitui o programa e reinicia — tudo sozinho.
func vigiarAtualizacoes(instalado string) {
	time.Sleep(90 * time.Second)
	for {
		if url, nova := procurarNova(); url != "" {
			log.Printf("versão nova encontrada: %s", nova)
			if err := aplicarAtualizacao(instalado, url); err != nil {
				log.Printf("falha a atualizar: %v", err)
			} else {
				return // o novo processo assume; este sai
			}
		}
		time.Sleep(time.Hour)
	}
}

func procurarNova() (string, string) {
	cli := &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{Proxy: proxyWindows}}
	req, _ := http.NewRequest("GET", releasesLatest, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "EcraRemoto")
	resp, err := cli.Do(req)
	if err != nil {
		log.Printf("atualização: %v", err)
		return "", ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", ""
	}
	var j struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if json.NewDecoder(resp.Body).Decode(&j) != nil {
		return "", ""
	}
	tag := j.TagName
	if len(tag) > 0 && tag[0] == 'v' {
		tag = tag[1:]
	}
	atual, _ := strconv.Atoi(versao)
	nova, _ := strconv.Atoi(tag)
	if nova <= atual {
		return "", ""
	}
	for _, a := range j.Assets {
		if a.Name == nomeExe {
			return a.URL, tag
		}
	}
	return "", ""
}

func aplicarAtualizacao(instalado, url string) error {
	cli := &http.Client{Timeout: 5 * time.Minute, Transport: &http.Transport{Proxy: proxyWindows}}
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "EcraRemoto")
	resp, err := cli.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	novo := filepath.Join(pastaApp(), "novo.exe")
	f, err := os.Create(novo)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		return err
	}
	f.Close()
	if err := iniciar(novo, "--atualizar-de", instalado); err != nil {
		return err
	}
	os.WriteFile(filepath.Join(pastaApp(), "parar"), []byte("1"), 0o644)
	time.Sleep(2 * time.Second)
	return nil
}
