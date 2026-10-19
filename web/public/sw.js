// bitdispatcher's service worker: keeps the interface itself (never the data) so a console reloaded during
// an outage still opens, and shows the last board it kept. Network first: a working server always wins.
const CACHE = 'bitdispatcher-shell'
self.addEventListener('install', (e) => {
  e.waitUntil(caches.open(CACHE).then((c) => c.add('./')).then(() => self.skipWaiting()).catch(() => self.skipWaiting()))
})
self.addEventListener('activate', (e) => e.waitUntil(self.clients.claim()))
self.addEventListener('fetch', (e) => {
  const u = new URL(e.request.url)
  if (e.request.method !== 'GET' || u.origin !== self.location.origin || u.pathname.startsWith('/api/') || u.pathname.startsWith('/manual')) return
  const key = e.request.mode === 'navigate' ? './' : e.request
  e.respondWith(
    fetch(e.request)
      .then((r) => {
        if (r.ok && (e.request.mode === 'navigate' || u.pathname.startsWith('/assets/') || u.pathname === '/favicon.svg')) {
          const copy = r.clone()
          caches.open(CACHE).then((c) => c.put(key, copy))
        }
        return r
      })
      .catch(() => caches.match(key).then((r) => r || Response.error())),
  )
})
