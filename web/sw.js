// Service worker do Ecrã Remoto — torna o arranque instantâneo (guarda a interface em cache)
// e deixa a app instalável como PWA. NÃO guarda em cache nada do relé (WebSocket/estado).
const CACHE = "ecra-remoto-v1";
const SHELL = ["/app", "/app.js", "/manifest.webmanifest", "/icon-192.png", "/icon-512.png"];

self.addEventListener("install", (e) => {
  e.waitUntil(caches.open(CACHE).then((c) => c.addAll(SHELL)).then(() => self.skipWaiting()));
});

self.addEventListener("activate", (e) => {
  e.waitUntil(
    caches.keys().then((ks) => Promise.all(ks.filter((k) => k !== CACHE).map((k) => caches.delete(k)))).then(() => self.clients.claim())
  );
});

self.addEventListener("fetch", (e) => {
  const url = new URL(e.request.url);
  if (e.request.method !== "GET") return;
  if (url.origin !== location.origin) return;
  if (url.pathname.startsWith("/ws/") || url.pathname.startsWith("/estado/") || url.pathname.startsWith("/baixar")) return;

  const ehShell = SHELL.includes(url.pathname) || url.pathname === "/";
  if (ehShell) {
    e.respondWith(
      caches.open(CACHE).then(async (c) => {
        const cached = await c.match(e.request, { ignoreSearch: true });
        const rede = fetch(e.request).then((r) => { if (r && r.ok) c.put(url.pathname, r.clone()); return r; }).catch(() => cached);
        return cached || rede;
      })
    );
  }
});
