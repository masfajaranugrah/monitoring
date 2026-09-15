import { useMonitorStore } from '../stores/monitor'
import { useAuthStore } from '../stores/auth'

let source = null
let retryTimer = null

function handleEvent(eventName) {
  return (e) => {
    const store = useMonitorStore()
    try {
      const data = JSON.parse(e.data)
      store.lastEvent = { event: eventName, data }
    } catch (err) {
      console.error('bad sse payload', err)
    }
  }
}

function connect() {
  const auth = useAuthStore()
  if (!auth.token) return

  if (source) source.close()

  source = new EventSource(`/api/events`)
  source.onopen = () => {
    const store = useMonitorStore()
    store.connected = true
  }

  source.addEventListener('customer:update', handleEvent('customer:update'))
  source.addEventListener('stats:update', handleEvent('stats:update'))
  source.addEventListener('vpn:update', handleEvent('vpn:update'))
  source.addEventListener('alert:new', handleEvent('alert:new'))

  source.onerror = (err) => {
    const store = useMonitorStore()
    store.connected = false
    source.close()
    source = null
    if (err?.eventPhase === EventSource.CLOSED) {
      // Reconnect after a delay so the server has time to restart.
      clearTimeout(retryTimer)
      retryTimer = setTimeout(connect, 5000)
    }
  }
}

export function startRealtime() {
  connect()
}

export function stopRealtime() {
  if (source) source.close()
  source = null
  clearTimeout(retryTimer)
}