<script setup>
import { onBeforeUnmount, onMounted, ref, shallowRef } from 'vue'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'
import { useAuthStore } from '../stores/auth'

const auth = useAuthStore()
const container = ref(null)
const status = ref('Menghubungkan...')

const term = shallowRef(null)
const fit = shallowRef(null)
let ws = null

function buildUrl() {
  const proto = location.protocol === 'https:' ? 'wss' : 'ws'
  return `${proto}://${location.host}/api/terminal/ws?token=${encodeURIComponent(auth.token)}`
}

function connect() {
  if (ws) ws.close()

  ws = new WebSocket(buildUrl())
  ws.binaryType = 'arraybuffer'

  ws.onopen = () => {
    status.value = 'Connected'
    fit.value && fit.value.fit()
    term.value && term.value.focus()
  }
  ws.onmessage = (ev) => {
    term.value && term.value.write(new Uint8Array(ev.data))
  }
  ws.onclose = () => {
    status.value = 'Disconnected'
    term.value && term.value.write('\r\n\x1b[31m[terminal terputus — klik Sambungkan]\x1b[0m\r\n')
  }
  ws.onerror = () => {
    status.value = 'Error'
  }
}

function disconnect() {
  if (ws) {
    ws.close()
    ws = null
    status.value = 'Disconnected'
  }
}

function clearScreen() {
  term.value && term.value.clear()
}

onMounted(() => {
  term.value = new Terminal({
    cursorBlink: true,
    fontSize: 13,
    fontFamily: 'Menlo, Monaco, Consolas, monospace',
    theme: {
      background: '#0d1117',
      foreground: '#e6edf3',
      cursor: '#79c0ff'
    }
  })
  fit.value = new FitAddon()
  term.value.loadAddon(fit.value)
  term.value.open(container.value)
  fit.value.fit()

  term.value.onData((data) => {
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(new TextEncoder().encode(data))
    }
  })
  term.value.onResize(({ cols, rows }) => {
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify({ type: 'resize', cols, rows }))
    }
  })

  connect()
})

onBeforeUnmount(() => {
  disconnect()
  if (term.value) term.value.dispose()
  term.value = null
})
</script>

<template>
  <div>
    <div class="panel detail__card terminal-panel">
      <div class="terminal-toolbar">
        <strong>Terminal — Server</strong>
        <span class="terminal-status" :class="status === 'Connected' ? 'is-on' : ''">
          {{ status }}
        </span>
        <span class="terminal-actions">
          <button class="btn btn--ghost btn--sm" type="button" @click="clearScreen">Clear</button>
          <button
            class="btn btn--sm"
            :class="status === 'Connected' ? 'btn--danger' : 'btn--primary'"
            type="button"
            @click="status === 'Connected' ? disconnect() : connect()"
          >
            {{ status === 'Connected' ? 'Putuskan' : 'Sambungkan' }}
          </button>
        </span>
      </div>
      <div ref="container" class="terminal-box" />
    </div>
  </div>
</template>

<style scoped>
.terminal-panel {
  padding: 0;
  display: flex;
  flex-direction: column;
}

.terminal-toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 16px;
  border-bottom: 1px solid var(--border, #e5e7eb);
}

.terminal-status {
  font-size: 0.8rem;
  color: #b45309;
}

.terminal-status.is-on {
  color: #16a34a;
}

.terminal-actions {
  margin-left: auto;
  display: flex;
  gap: 8px;
}

.terminal-box {
  height: 68vh;
  min-height: 360px;
  padding: 8px;
  background: #0d1117;
}
</style>