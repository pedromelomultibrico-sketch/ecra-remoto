// Service worker minimo: fetch handler vazio (necessario para a app ser instalavel),
// sem cache para nao servir versoes antigas durante o arranque.
self.addEventListener("install", (e) => self.skipWaiting());
self.addEventListener("activate", (e) => e.waitUntil(self.clients.claim()));
self.addEventListener("fetch", (e) => {});
