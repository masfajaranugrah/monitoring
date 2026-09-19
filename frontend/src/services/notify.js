let requested = false

export function requestNotifyPermission() {
  if (typeof window === 'undefined' || !('Notification' in window)) return
  if (requested) return
  requested = true
  const p = Notification.requestPermission()
  if (p && typeof p.then === 'function') {
    p.then(() => {}).catch(() => {})
  }
}

function formatTime(value) {
  if (!value) return new Date().toLocaleTimeString('id-ID')
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return new Date().toLocaleTimeString('id-ID')
  return d.toLocaleTimeString('id-ID')
}

export function notify(title, body, opts = {}) {
  if (typeof window === 'undefined' || !('Notification' in window)) return
  if (Notification.permission !== 'granted') return
  try {
    new Notification(title, {
      body,
      icon: opts.icon,
      tag: opts.tag || 'fm-alert',
      silent: true
    })
  } catch (e) {
    /* noop */
  }
}

export function offlineAlertPayload(data = {}) {
  const name = data.name || data.customer_name || '-'
  const ip = data.ip || '-'
  const time = formatTime(data.time)
  return {
    title: '🔴 ROUTER DOWN',
    body: `Pelanggan : ${name}\nIP : ${ip}\nWaktu : ${time} 🔊 ALARM`,
    tag: `fm-alert-${data.customer_id || 'x'}`
  }
}