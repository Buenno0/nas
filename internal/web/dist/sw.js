// Service worker do Ozymandias: só recebe avisos (Web Push). Não guarda nada
// em cache — o app continua sempre falando com o servidor.
self.addEventListener('install', () => self.skipWaiting())
self.addEventListener('activate', (e) => e.waitUntil(self.clients.claim()))

self.addEventListener('push', (e) => {
  let aviso = { titulo: 'Ozymandias', corpo: '' }
  try {
    aviso = e.data ? e.data.json() : aviso
  } catch {
    aviso.corpo = e.data ? e.data.text() : ''
  }
  e.waitUntil(
    self.registration.showNotification(aviso.titulo || 'Ozymandias', {
      body: aviso.corpo || '',
      icon: '/icons/appicon-192.png',
      badge: '/icons/appicon-192.png',
      tag: aviso.tag || undefined,
      data: { url: aviso.url || '/' },
    }),
  )
})

// Tocar na notificação abre (ou traz para a frente) o app na página certa.
self.addEventListener('notificationclick', (e) => {
  e.notification.close()
  const url = new URL(e.notification.data?.url || '/', self.location.origin).href
  e.waitUntil(
    self.clients.matchAll({ type: 'window', includeUncontrolled: true }).then((janelas) => {
      for (const j of janelas) {
        if (j.url.startsWith(self.location.origin)) {
          j.navigate(url)
          return j.focus()
        }
      }
      return self.clients.openWindow(url)
    }),
  )
})
