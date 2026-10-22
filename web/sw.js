// SW de auto-limpeza: apaga caches antigas e remove-se. Recupera de um SW anterior preso.
self.addEventListener("install", () => self.skipWaiting());
self.addEventListener("activate", (e) => {
  e.waitUntil((async () => {
    try {
      const ks = await caches.keys();
      await Promise.all(ks.map((k) => caches.delete(k)));
      await self.registration.unregister();
      const cs = await self.clients.matchAll({ type: "window" });
      cs.forEach((c) => { try { c.navigate(c.url); } catch (e) {} });
    } catch (e) {}
  })());
});
self.addEventListener("fetch", (e) => {});
