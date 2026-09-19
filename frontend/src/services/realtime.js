import { useMonitorStore } from '../stores/monitor'
import { useAuthStore } from '../stores/auth'
import { playAlarm } from './sound'
import { notify, offlineAlertPayload } from './notify'

let socket = null
let retryTimer = null
let stopped = true

function handleMessage(event) {
  const store = useMonitorStore()
  try {
    const msg = JSON.parse(event.data)
    const ev = { event: msg.event, data: msg.data }
    store.lastEvent = ev

    if (msg.event === 'alert:new') {
      const data = msg.data || {}
      if (data.alert_type === 'OFFLINE' || data.status === 'OFFLINE') {
        playAlarm()
        const { title, body, tag } = offlineAlertPayload(data)
        notify(title, body, { tag })
      }
    }
  } catch (err) {
    console.error('bad ws payload', err)
  }
}

function connect() {
  const auth = useAuthStore()
  if (!auth.token || stopped) return

  if (socket) socket.close()

  const proto = window.location.protocol === 'https:' ? 'wss' : 'ws'
  const url = `${proto}://${window.location.host}/api/events?token=${encodeURIComponent(auth.token)}`
  socket = new WebSocket(url)

  socket.onopen = () => {
    const store = useMonitorStore()
    store.connected = true
  }

  socket.onmessage = handleMessage

  socket.onerror = () => {
    const store = useMonitorStore()
    store.connected = false
    if (socket) socket.close()
  }

  socket.onclose = () => {
    const store = useMonitorStore()
    store.connected = false
    socket = null
    if (stopped) return
    // Always retry so realtime survives transient errors / proxy resets.
    clearTimeout(retryTimer)
    retryTimer = setTimeout(connect, 3000)
  }
}

export function startRealtime() {
  stopped = false
  clearTimeout(retryTimer)
  connect()
}

export function stopRealtime() {
  stopped = true
  clearTimeout(retryTimer)
  if (socket) {
    socket.onclose = null
    socket.close()
  }
  socket = null
}