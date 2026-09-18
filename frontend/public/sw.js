// Minimal service worker for PWA installability only
// No caching logic - relies entirely on HTTP cache headers
self.addEventListener('install', () => self.skipWaiting());
self.addEventListener('activate', () => self.clients.claim());
