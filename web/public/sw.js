// The server decides who is notified, so every push that arrives is shown.
self.addEventListener('push', (event) => {
  const m = event.data.json()
  event.waitUntil(
    self.registration.showNotification(m.title, {
      body: m.body,
      tag: m.tag,
      // A notification with no tag must not ask to renotify, or the browser refuses to show it.
      renotify: Boolean(m.tag),
      icon: '/icon-192.png',
      data: { url: m.url },
    }),
  )
})

self.addEventListener('notificationclick', (event) => {
  event.notification.close()
  const url = new URL(event.notification.data.url, self.location.origin).href
  event.waitUntil(
    self.clients.matchAll({ type: 'window' }).then(async (windows) => {
      if (windows.length === 0) return self.clients.openWindow(url)
      await windows[0].focus()
      return windows[0].navigate(url)
    }),
  )
})

// Claims the pages already open, the one that turned notifications on among them, so the click above can navigate it.
self.addEventListener('activate', (event) => event.waitUntil(self.clients.claim()))
