// Minimal service worker: PWA installability and fault notifications.
// No caching logic - relies entirely on HTTP cache headers.
self.addEventListener('install', () => self.skipWaiting());
self.addEventListener('activate', () => self.clients.claim());

// A click on a fault notification focuses an open app window on the Status page, or
// opens one.
self.addEventListener('notificationclick', event => {
  event.notification.close();
  const url = (event.notification.data && event.notification.data.url) || '/';
  event.waitUntil(
    self.clients.matchAll({ type: 'window', includeUncontrolled: true }).then(windows => {
      for (const w of windows) {
        if ('focus' in w) {
          return w.focus().then(f => (f && 'navigate' in f ? f.navigate(url) : f));
        }
      }
      return self.clients.openWindow(url);
    })
  );
});
