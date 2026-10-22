"use strict";
const SERVIDOR = location.origin.replace(/^http/, "ws");
const $ = (s) => document.querySelector(s);
const LS = "ecra_remoto_pcs";

/* ---------------- lista de PCs ---------------- */
function carregarPCs() { try { return JSON.parse(localStorage.getItem(LS)) || []; } catch { return []; } }
function gravarPCs(v) { try { localStorage.setItem(LS, JSON.stringify(v)); } catch {} }
let pcs = carregarPCs();
let editando = -1;

function pintarLista() {
  const c = $("#cartoes"); c.innerHTML = "";
  if (!pcs.length) {
    c.innerHTML = '<div class="vazio">Ainda não adicionou nenhum PC.<br>Toque em “Adicionar um PC” e introduza o ID e a senha que o programa mostra no computador.</div>';
    return;
  }
  pcs.forEach((pc, i) => {
    const d = document.createElement("div"); d.className = "pc";
    d.innerHTML = `<div class="ponto" id="p${i}"></div>
      <div style="min-width:0"><div class="nome">${esc(pc.nome)}</div><div class="sub" id="s${i}">${fmtID(pc.id)}</div></div>
      <button class="apagar" data-ap="${i}">🗑</button>
      <button class="lig" data-lig="${i}">Ligar</button>`;
    c.appendChild(d);
    estadoPC(pc.id, i);
  });
}
function esc(s){return (s||"").replace(/[<>&]/g,x=>({"<":"&lt;",">":"&gt;","&":"&amp;"}[x]));}
function fmtID(id){id=(id||"").replace(/\D/g,"");return id.length===9?`${id.slice(0,3)} ${id.slice(3,6)} ${id.slice(6)}`:id;}

async function estadoPC(id, i) {
  try {
    const r = await fetch(`/estado/${id.replace(/\D/g,"")}`); const j = await r.json();
    const p = $("#p"+i), s = $("#s"+i);
    if (!p) return;
    p.classList.toggle("on", !!j.online);
    if (s) s.textContent = (j.online ? "Ligado" : "Desligado") + (j.info?.nome ? " · " + j.info.nome : "") + " · " + fmtID(id);
  } catch {}
}

$("#btAdd").onclick = () => abrirAdd(-1);
function abrirAdd(i) {
  editando = i;
  $("#tAdd").textContent = i < 0 ? "Adicionar um PC" : "Editar PC";
  $("#inNome").value = i < 0 ? "" : pcs[i].nome;
  $("#inID").value = i < 0 ? "" : fmtID(pcs[i].id);
  $("#inSenha").value = i < 0 ? "" : pcs[i].senha;
  $("#veloAdd").classList.remove("oculto");
}
$("#btCancelar").onclick = () => $("#veloAdd").classList.add("oculto");
$("#btGuardar").onclick = () => {
  const nome = $("#inNome").value.trim() || "Meu PC";
  const id = $("#inID").value.replace(/\D/g, "");
  const senha = $("#inSenha").value.trim();
  if (id.length !== 9) { alert("O ID tem de ter 9 dígitos."); return; }
  if (!senha) { alert("Falta a senha."); return; }
  const obj = { nome, id, senha };
  if (editando < 0) pcs.push(obj); else pcs[editando] = obj;
  gravarPCs(pcs); $("#veloAdd").classList.add("oculto"); pintarLista();
};
$("#inID").oninput = (e) => { let v = e.target.value.replace(/\D/g,"").slice(0,9); e.target.value = v.replace(/(\d{3})(\d{0,3})(\d{0,3})/, (m,a,b,c)=>[a,b,c].filter(Boolean).join(" ")); };

$("#cartoes").addEventListener("click", (e) => {
  const ap = e.target.closest("[data-ap]"), lig = e.target.closest("[data-lig]");
  if (ap) { const i = +ap.dataset.ap; if (confirm("Remover “"+pcs[i].nome+"” da lista?")) { pcs.splice(i,1); gravarPCs(pcs); pintarLista(); } }
  if (lig) { ligar(pcs[+lig.dataset.lig]); }
});
setInterval(() => { if (!sessaoAtiva) pcs.forEach((pc,i)=>estadoPC(pc.id,i)); }, 5000);
pintarLista();

/* ---------------- sessão ---------------- */
let ws = null, sessaoAtiva = false, pcAtual = null;
let escala = 1, deslX = 0, deslY = 0; // img -> ecrã
let imgW = 0, imgH = 0, monW = 0, monH = 0, nMon = 1, monIdx = 0;
let qualidade = "equilibrado", mostrarCursor = true, toqueDireto = true;
const img = $("#ecra"), palco = $("#palco"), cursor = $("#cursor");
const lona = $("#lona"), ctx = lona.getContext("2d", { alpha: false });
let urlAtual = null;

function ligar(pc) {
  pcAtual = pc; sessaoAtiva = true;
  try { const el = document.documentElement; (el.requestFullscreen || el.webkitRequestFullscreen || function(){}).call(el); } catch (e) {}
  $("#inicio").classList.add("oculto");
  $("#veloAdd").classList.add("oculto");
  $("#sessao").classList.remove("oculto");
  $("#espera").classList.remove("oculto");
  $("#esperaTxt").textContent = "A ligar ao PC…";
  abrirWS();
}

function abrirWS() {
  try { ws && ws.close(); } catch {}
  ws = new WebSocket(`${SERVIDOR}/ws/ver/${pcAtual.id.replace(/\D/g,"")}`);
  ws.binaryType = "arraybuffer";
  ws.onopen = () => ws.send(JSON.stringify({ t: "auth", senha: pcAtual.senha }));
  ws.onmessage = receber;
  ws.onclose = () => { if (sessaoAtiva) mostrarEspera("Ligação perdida. A tentar de novo…", true); };
  ws.onerror = () => {};
}
let reintentar = null;
function mostrarEspera(txt, reconectar) {
  $("#espera").classList.remove("oculto");
  $("#esperaTxt").textContent = txt;
  if (reconectar) { clearTimeout(reintentar); reintentar = setTimeout(() => { if (sessaoAtiva) abrirWS(); }, 2500); }
}

let faixasEsperadas = 0, prontoQuadro = null;
function receber(ev) {
  if (typeof ev.data === "string") {
    const m = JSON.parse(ev.data);
    switch (m.t) {
      case "erro": sessaoAtiva = false; alert(m.m || "Erro"); sair(); break;
      case "estado":
        if (!m.pc) mostrarEspera("O PC está desligado ou o programa não está a correr.\nAssim que ligar, aparece aqui.", false);
        else { $("#espera").classList.add("oculto"); pedirEcra(); }
        break;
      case "quadro": prepararQuadro(m); break;
      case "aviso": toast(m.m); break;
      case "estado_pc": pintarEstadoPC(m); break;
      case "clip": $("#clipPC").value = m.txt || ""; if ($("#folhaClip").classList.contains("aberta")) toast("Área de transferência do PC atualizada"); break;
      case "fich_inicio": iniciarReceberFich(m); break;
      case "fich_parte": parteReceberFich(m); break;
    }
    return;
  }
  // binário: cabeçalho "x,y,w,h," + JPEG
  const buf = new Uint8Array(ev.data);
  let v = 0, nums = [];
  for (let i = 0; i < buf.length; i++) { if (buf[i] === 44) { nums.push(v); v = 0; if (nums.length === 4) { desenharFaixa(nums, buf.subarray(i+1)); return; } } else { v = v*10 + (buf[i]-48); } }
}

function prepararQuadro(m) {
  if (imgW !== m.w || imgH !== m.h) { imgW = m.w; imgH = m.h; lona.width = m.w; lona.height = m.h; ctx.fillStyle = "#000"; ctx.fillRect(0,0,m.w,m.h); }
  monW = m.mw; monH = m.mh; nMon = m.mons; monIdx = m.mon;
  faixasEsperadas = m.n;
  ultimoRx = m.rx; ultimoRy = m.ry;
  $("#espera").classList.add("oculto");
  ajustar();
}
let ultimoRx = 0, ultimoRy = 0;
function desenharFaixa([x,y,w,h], jpeg) {
  const bl = new Blob([jpeg], { type: "image/jpeg" });
  createImageBitmap(bl).then((bmp) => {
    ctx.drawImage(bmp, x, y);
    bmp.close();
    faixasEsperadas--;
    if (faixasEsperadas <= 0) publicar();
  }).catch(()=>{});
}
let pintar = false;
function publicar() {
  if (pintar) return; pintar = true;
  requestAnimationFrame(() => {
    pintar = false;
    lona.toBlob((b) => {
      if (!b) return;
      if (urlAtual) URL.revokeObjectURL(urlAtual);
      urlAtual = URL.createObjectURL(b);
      img.src = urlAtual;
    }, "image/jpeg", 0.9);
    if (mostrarCursor && monW) {
      cursor.style.opacity = "1";
      cursor.style.left = (deslX + (ultimoRx * imgW/monW) * escala) + "px";
      cursor.style.top = (deslY + (ultimoRy * imgH/monH) * escala) + "px";
    } else cursor.style.opacity = "0";
  });
}

function ajustar() {
  if (!imgW) return;
  const pw = palco.clientWidth, ph = palco.clientHeight;
  escala = Math.min(pw/imgW, ph/imgH);
  const w = imgW*escala, h = imgH*escala;
  deslX = (pw-w)/2; deslY = (ph-h)/2;
  img.style.width = imgW+"px"; img.style.height = imgH+"px";
  img.style.transform = `translate(${deslX}px,${deslY}px) scale(${escala})`;
}
addEventListener("resize", ajustar);

function enviar(o){ if (ws && ws.readyState===1) ws.send(JSON.stringify(o)); }
function pedirEcra(){ enviar({ t: "ver", q: qualidade, mon: monIdx }); }

/* ------- toque / rato ------- */
function paraMonitor(cx, cy) {
  const x = (cx - deslX) / (imgW*escala);
  const y = (cy - deslY) / (imgH*escala);
  return { x: Math.max(0, Math.min(1, x)), y: Math.max(0, Math.min(1, y)) };
}

let t0 = 0, movX = 0, movY = 0, arrastou = false, baixou = false;
let prX = 0, prY = 0, pinchD = 0, ultX = 0, ultY = 0;
palco.addEventListener("pointerdown", (e) => {
  palco.setPointerCapture(e.pointerId);
  if (modoTeclado) return;
  t0 = Date.now(); arrastou = false; baixou = false;
  prX = ultX = e.clientX; prY = ultY = e.clientY;
}, { passive: false });

palco.addEventListener("pointermove", (e) => {
  if (!t0) return;
  const dx = e.clientX - ultX, dy = e.clientY - ultY;
  ultX = e.clientX; ultY = e.clientY;
  if (Math.abs(e.clientX-prX) + Math.abs(e.clientY-prY) > 8) arrastou = true;
  if (toqueDireto) {
    const p = paraMonitor(e.clientX, e.clientY);
    if (arrastou && !baixou && Date.now()-t0 > 180) { baixou = true; enviar({ t:"baixo", bot:0, ...p }); }
    enviar({ t: "mover", ...p });
  } else {
    const fator = 1.6 / escala;
    movRel(dx*fator, dy*fator);
  }
}, { passive: false });

let rx = 0.5, ry = 0.5;
function movRel(dx, dy) {
  rx = Math.max(0, Math.min(1, rx + dx/monW));
  ry = Math.max(0, Math.min(1, ry + dy/monH));
  enviar({ t: "mover", x: rx, y: ry });
}

palco.addEventListener("pointerup", (e) => {
  if (!t0) return;
  const dt = Date.now() - t0; t0 = 0;
  const p = toqueDireto ? paraMonitor(e.clientX, e.clientY) : { x: rx, y: ry };
  if (baixou) { enviar({ t:"cima", bot:0 }); return; }
  if (!arrastou && dt < 350) { enviar({ t: "clique", bot: 0, ...p }); }
}, { passive: false });

let longo = null;
palco.addEventListener("pointerdown", (e) => {
  clearTimeout(longo);
  longo = setTimeout(() => {
    if (!arrastou) { const p = toqueDireto ? paraMonitor(e.clientX,e.clientY) : {x:rx,y:ry}; enviar({ t:"clique", bot:2, ...p }); navigator.vibrate&&navigator.vibrate(20); t0=0; }
  }, 600);
});
palco.addEventListener("pointerup", () => clearTimeout(longo));
palco.addEventListener("pointermove", () => { if (arrastou) clearTimeout(longo); });

let toques = new Map();
palco.addEventListener("pointerdown", e => toques.set(e.pointerId, e));
palco.addEventListener("pointermove", (e) => {
  if (!toques.has(e.pointerId)) return;
  toques.set(e.pointerId, e);
  if (toques.size === 2) {
    clearTimeout(longo); t0 = 0;
    const [a,b] = [...toques.values()];
    const cy = (a.clientY+b.clientY)/2;
    if (lastCy !== null) enviar({ t:"roda", dy: Math.round((cy-lastCy)*2), dx:0 });
    lastCy = cy;
  }
});
let lastCy = null;
palco.addEventListener("pointerup", e => { toques.delete(e.pointerId); if (toques.size<2) lastCy=null; });

/* ------- teclado ------- */
let modoTeclado = false;
const entrada = $("#entradaTeclado");
$("#fbTeclado").onclick = () => {
  modoTeclado = !modoTeclado;
  $("#fbTeclado").classList.toggle("lig", modoTeclado);
  if (modoTeclado) { entrada.value=""; entrada.focus(); } else entrada.blur();
};
entrada.addEventListener("input", (e) => {
  const v = e.target.value;
  if (v) { enviar({ t:"texto", txt:v }); e.target.value=""; }
});
entrada.addEventListener("keydown", (e) => {
  const esp = { Backspace:8, Enter:13, Tab:9, Escape:27, Delete:46, ArrowUp:38, ArrowDown:40, ArrowLeft:37, ArrowRight:39, Home:36, End:35, PageUp:33, PageDown:34 };
  if (esp[e.key]) { enviar({ t:"tecla", vk: esp[e.key] }); e.preventDefault(); }
});
$("#fbRato").onclick = () => { modoTeclado=false; entrada.blur(); $("#fbTeclado").classList.remove("lig"); toast(toqueDireto?"Toque direto":"Touchpad"); };

/* ------- barra e folhas ------- */
function folha(id){ document.querySelectorAll(".folha").forEach(f=>f.classList.remove("aberta")); const f=$("#"+id); if(f) f.classList.add("aberta"); }
document.querySelectorAll(".fechar").forEach(b=>b.onclick=()=>$("#"+b.dataset.f).classList.remove("aberta"));
$("#fbMenu").onclick = () => { folha("folhaMenu"); enviar({t:"estado_pc"}); };
$("#fbEspeciais").onclick = () => folha("folhaEsp");
$("#fbFich").onclick = () => folha("folhaFich");
$("#fbClip").onclick = () => folha("folhaClip");
$("#fbRodar").onclick = () => { if(nMon<=1){toast("Só há um monitor");return;} monIdx=(monIdx+1)%nMon; pedirEcra(); toast("Monitor "+(monIdx+1)+" de "+nMon); };
$("#fbSair").onclick = sair;
let privadoLigado = false;
function porPrivado(on){ privadoLigado=on; $("#fbPrivado").classList.toggle("lig",on); const i=$("#intPrivado"); if(i) i.classList.toggle("on",on); enviar({t:"modo_privado",on}); toast(on?"Ecrã do PC privado — apagado para quem está no escritório":"Ecrã do PC outra vez visível"); }
$("#fbPrivado").onclick = () => porPrivado(!privadoLigado);

function sair(){
  sessaoAtiva=false; clearTimeout(reintentar);
  try{ enviar({t:"pausa"}); ws&&ws.close(); }catch{}
  $("#sessao").classList.add("oculto"); $("#inicio").classList.remove("oculto");
  document.querySelectorAll(".folha").forEach(f=>f.classList.remove("aberta"));
  pcs.forEach((pc,i)=>estadoPC(pc.id,i));
}

/* teclas especiais */
const MODS = [["Ctrl","ctrl"],["Alt","alt"],["Shift","shift"],["Win","win"]];
const ESPEC = [["Esc",27],["Tab",9],["↵ Enter",13],["⌫",8],["Del",46],["↑",38],["↓",40],["←",37],["→",39],["Home",36],["End",35],["PgUp",33],["PgDn",34],["F2",113],["F5",116],["F11",122]];
let modsAtivos = new Set();
function pintarTeclas(){
  const m=$("#mods"); m.innerHTML="";
  MODS.forEach(([n,k])=>{ const b=document.createElement("button"); b.className="te"+(modsAtivos.has(k)?" on":""); b.textContent=n; b.onclick=()=>{ modsAtivos.has(k)?modsAtivos.delete(k):modsAtivos.add(k); pintarTeclas(); }; m.appendChild(b); });
  const t=$("#teclasEsp"); t.innerHTML="";
  ESPEC.forEach(([n,vk])=>{ const b=document.createElement("button"); b.className="te"; b.textContent=n; b.onclick=()=>{ enviar({t:"tecla",vk,mods:[...modsAtivos]}); modsAtivos.clear(); pintarTeclas(); }; t.appendChild(b); });
}
pintarTeclas();

/* sistema */
document.querySelectorAll("[data-sis]").forEach(b=>b.onclick=()=>{
  const a=b.dataset.sis;
  if((a==="reiniciar"||a==="desligar") && !confirm("Tem a certeza que quer "+a+" o PC?")) return;
  enviar({t:"sistema",acao:a});
  if(a==="reiniciar"||a==="desligar") toast("A "+a+"… (5 s para cancelar no Mais)");
  else toast("Feito");
});
function pintarEstadoPC(m){
  const un=(m.unidades||[]).map(u=>`<span class="chip">${u}</span>`).join("");
  const bat = m.bateria>=0 ? `Bateria: ${m.bateria}% ${m.corrente?"(ligado à corrente)":"(sem corrente)"}` : (m.corrente?"Ligado à corrente":"");
  $("#estadoPC").innerHTML = `${bat?bat+"<br>":""}Versão do programa: ${m.versao||"?"}<br><div class="lista-u">${un}</div>`;
}

/* área de transferência */
$("#btEnviarClip").onclick = () => { enviar({t:"clip_para_pc", txt:$("#clipPC").value}); toast("Enviado para a área de transferência do PC"); };
$("#btColar").onclick = () => { enviar({t:"colar", txt:$("#clipPC").value}); toast("Colado no PC"); };

/* qualidade e definições */
$("#segQ").addEventListener("click",(e)=>{ const b=e.target.closest("[data-q]"); if(!b)return; qualidade=b.dataset.q; document.querySelectorAll("#segQ button").forEach(x=>x.classList.toggle("sel",x===b)); pedirEcra(); });
$("#intCursor").onclick = (e)=>{ mostrarCursor=!mostrarCursor; e.currentTarget.classList.toggle("on",mostrarCursor); };
$("#intDireto").onclick = (e)=>{ toqueDireto=!toqueDireto; e.currentTarget.classList.toggle("on",toqueDireto); toast(toqueDireto?"Toque direto":"Touchpad"); };
$("#intPrivado").onclick = (e)=>{ const on=!e.currentTarget.classList.contains("on"); e.currentTarget.classList.toggle("on",on); enviar({t:"modo_privado",on}); toast(on?"Ecrã do PC privado — apagado para quem está no escritório":"Ecrã do PC outra vez visível"); };
$("#fbRodar").addEventListener("contextmenu",e=>{e.preventDefault();folha("folhaDef");});
let defLongo=null;
$("#fbRodar").addEventListener("pointerdown",()=>{defLongo=setTimeout(()=>folha("folhaDef"),600);});
$("#fbRodar").addEventListener("pointerup",()=>clearTimeout(defLongo));

/* ------- ficheiros ------- */
$("#btEnviarPC").onclick = () => $("#ficheiroTablet").click();
$("#ficheiroTablet").onchange = async (e) => {
  const f = e.target.files[0]; if(!f) return;
  if (f.size > 200*1024*1024) { toast("Máx. 200 MB"); return; }
  enviar({ t:"ficheiro_para_pc_inicio", nome:f.name });
  const tam = f.size; let lido = 0;
  const reader = f.stream().getReader();
  $("#transf").textContent = "A enviar "+f.name+"… 0%";
  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    for (let o=0; o<value.length; o+=48*1024) {
      const ped = value.subarray(o, o+48*1024);
      enviar({ t:"ficheiro_para_pc_parte", nome:f.name, dados: b64(ped), fim:false });
      lido += ped.length;
      $("#transf").textContent = "A enviar "+f.name+"… "+Math.round(lido/tam*100)+"%";
      await espera();
    }
  }
  enviar({ t:"ficheiro_para_pc_parte", nome:f.name, dados:"", fim:true });
  $("#transf").textContent = "Enviado — "+f.name;
  e.target.value="";
};
function espera(){ return new Promise(r=>{ if(ws.bufferedAmount>2*1024*1024){ const t=setInterval(()=>{ if(ws.bufferedAmount<512*1024){clearInterval(t);r();} },30);} else r(); }); }
function b64(u8){ let s=""; for(let i=0;i<u8.length;i++) s+=String.fromCharCode(u8[i]); return btoa(s); }

let recv = null;
function iniciarReceberFich(m){ recv = { nome:m.nome, partes:[] }; toast("A receber "+m.nome+"…"); }
function parteReceberFich(m){
  if(!recv) return;
  const bin = atob(m.dados||""); const u = new Uint8Array(bin.length);
  for(let i=0;i<bin.length;i++) u[i]=bin.charCodeAt(i);
  recv.partes.push(u);
  if(m.fim){
    const bl = new Blob(recv.partes);
    if (window.AndroidFich) {
      const fr = new FileReader();
      fr.onload = () => { try { window.AndroidFich.guardar(recv.nome, fr.result); } catch(e){} toast("Recebido: "+recv.nome); recv=null; };
      fr.readAsDataURL(bl);
    } else {
      const a = document.createElement("a"); a.href=URL.createObjectURL(bl); a.download=recv.nome; a.click();
      toast("Recebido: "+recv.nome); recv=null;
    }
  }
}

/* toast */
let toastT=null;
function toast(t){ const a=$("#aviso"); a.textContent=t; a.classList.remove("oculto"); a.style.opacity="1"; clearTimeout(toastT); toastT=setTimeout(()=>{a.style.opacity="0"; setTimeout(()=>a.classList.add("oculto"),400);},3200); }
