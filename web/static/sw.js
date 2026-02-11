// Minimal service worker for PWA installability.
// Caches the app shell for offline fallback.
const CACHE_NAME = 'cc-mobile-v4';
const SHELL_URLS = ['/', '/css/style.css', '/js/app.js', '/manifest.json', '/icon.svg'];

self.addEventListener('install', (e) => {
  e.waitUntil(caches.open(CACHE_NAME).then((c) => c.addAll(SHELL_URLS)));
  self.skipWaiting();
});

self.addEventListener('activate', (e) => {
  e.waitUntil(
    caches.keys().then((names) =>
      Promise.all(names.filter((n) => n !== CACHE_NAME).map((n) => caches.delete(n)))
    )
  );
  self.clients.claim();
});

self.addEventListener('fetch', (e) => {
  // Network-first: try network, fall back to cache for app shell
  e.respondWith(
    fetch(e.request).catch(() =>
      caches.match(e.request).then((r) => r || new Response('Offline', { status: 503 }))
    )
  );
});
