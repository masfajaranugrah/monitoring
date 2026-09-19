import { useMonitorStore } from '../stores/monitor'
import { useAuthStore } from '../stores/auth'
import { playAlarm } from './sound'
import { notify, offlineAlertPayload } from './notify'
import api from '../api'

let socket = null
let retryTimer = null
let pollTimer = null
let stopped = true

const lastRing = new Map()
const seenAlertIds = new Set()
let pollSeeded = false

// Ring the alarm + desktop notification for an offline alert, deduped per
// customer for 60s so repeated pings / poll + WS races never double-ring.
function fireAlarm(data = {}) {
  const cid = data.customer_id
  if (cid != null) {
    const now = Date.now()
    if (lastRing.has(cid) && now - lastRing.get(cid) < 60000) return
    lastRing.set(cid, now)
  }
  playAlarm(data.name)
  const { title, body, tag } = offlineAlertPayload(data)
  notify(title, body, { tag })
}

async function pollAlerts() {
  try {
    const { data } = await api.get('/alerts', { params: { limit: 30 } })
    const list = data.data || []
    if (!pollSeeded) {
      list.forEach((a) => seenAlertIds.add(String(a.id)))
      pollSeeded = true
      return
    }
    for (const a of list) {
      const key = String(a.id)
      if (seenAlertIds.has(key)) continue
      seenAlertIds.add(key)
      if (a.alert_type === 'OFFLINE') {
        const name = a.title ? String(a.title).replace(/\s+OFFLINE$/, '') : ''
        fireAlarm({ customer_id: a.customer_id, name, ip: '', time: a.created_at })
      }
    }
  } catch (e) {
    /* network errors are fine; next tick retries */
  }
}

function handleMessage(event) {
  const store = useMonitorStore()
  try {
    const msg = JSON.parse(event.data)
    const ev = { event: msg.event, data: msg.data }
    store.lastEvent = ev

    if (msg.event === 'alert:new') {
      const data = msg.data || {}
      if (data.alert_type === 'OFFLINE' || data.status === 'OFFLINE') {
        fireAlarm(data)
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
  clearInterval(pollTimer)
  pollTimer = setInterval(pollAlerts, 15000)
  pollAlerts()
}

export function stopRealtime() {
  stopped = true
  clearTimeout(retryTimer)
  clearInterval(pollTimer)
  pollTimer = null
  if (socket) {
    socket.onclose = null
    socket.close()
  }
  socket = null
}