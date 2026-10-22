# Ecrã Remoto

Acesso remoto ao PC a partir do tablet — como o AnyDesk, mas sem precisar de administrador.

## Como funciona
- **Programa do PC** (`agente/`, Go → `ecra-remoto.exe`): corre na pasta do utilizador (sem administrador), arranca com o Windows, mostra um **ID** de 9 dígitos e uma **senha**. Liga-se para fora a um relé na Cloudflare.
- **Relé** (`worker/worker.js`, Cloudflare Worker + Durable Object): liga o PC ao tablet por WebSocket. Não guarda imagens; só encaminha.
- **App do tablet** (`app/`, Android + WebView) e a interface (`web/`): vê o ecrã e controla rato, teclado, ficheiros e energia.

## Funções
Ver o ecrã (vários monitores), rato e teclado, teclas especiais e atalhos, área de transferência (PC↔tablet), enviar e receber ficheiros, bloquear/suspender/reiniciar/desligar, estado (bateria, discos). Qualidade ajustável.

## Segurança
Cada PC tem um segredo próprio (fixa o ID ao primeiro PC) e uma senha para o tablet. 8 senhas erradas = 15 min de bloqueio. Tudo por TLS.

## Instalar
- PC: descarregar o `.exe` da última versão e abrir. Instala-se sozinho e passa a arrancar com o Windows.
- Tablet: instalar o APK da última versão. Atualiza-se sozinho.
