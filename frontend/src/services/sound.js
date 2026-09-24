import bellUrl from '../assets/bel.mp3'
import { requestNotifyPermission } from './notify'

const ALARM_REPEATS = 3
const ALARM_INTERVAL_MS = 700
const MAX_PLAY_ATTEMPTS = 4
const RETRY_DELAY_MS = 800

// Event types that count as real user activation for Notification permission.
const GESTURE_EVENTS = new Set(['pointerdown', 'mousedown', 'click', 'keydown', 'touchstart'])

let audio = null
let unlocked = false
let pendingAlarm = false
let alarmTimer = null
let retryTimer = null
let playAttempts = 0
let blockHintShown = false
let alarmName = ''
let spokeOnce = false

function ensureAudio() {
  if (audio) return audio
  audio = new Audio(bellUrl)
  audio.preload = 'auto'

  // Unlock on any kind of user engagement so audio becomes available as soon
  // as the person does anything at all (in browsers that require a gesture).
  const events = [
    'pointerdown',
    'mousedown',
    'mouseup',
    'click',
    'keydown',
    'touchstart',
    'wheel',
    'scroll'
  ]
  for (const e of events) document.addEventListener(e, handleUnlock, { capture: true })
  document.addEventListener('pointermove', handleUnlock, { once: true })
  document.addEventListener('mousemove', handleUnlock, { once: true })

  return audio
}

function handleUnlock(event) {
  if (unlocked) return
  unlocked = true

  const a = ensureAudio()
  a.volume = 0
  a.currentTime = 0
  a.play()
    .then(() => {
      a.pause()
      a.currentTime = 0
    })
    .catch(() => {})

  // Notification permission can only be requested from a genuine user gesture
  // (click/key/touch) — not from mousemove/wheel/scroll movement.
  if (event && GESTURE_EVENTS.has(event.type)) {
    requestNotifyPermission()
  }

  if (pendingAlarm) {
    pendingAlarm = false
    queueAlarm()
  }
}

function showBlockHint() {
  if (blockHintShown) return
  blockHintShown = true
  const el = document.createElement('div')
  el.style.cssText =
    'position:fixed;top:14px;left:50%;transform:translateX(-50%);z-index:99999;' +
    'background:rgba(24,34,59,.96);color:#fff;border:1px solid var(--border,#3b83f6);' +
    'border-radius:10px;padding:10px 16px;font-size:13px;box-shadow:0 6px 24px rgba(0,0,0,.4);' +
    'max-width:92vw;text-align:center;line-height:1.5;pointer-events:none;'
  el.textContent =
    '🔔 Bell alarm diblokir browser. Klik sekali halaman ini untuk mengaktifkan suara, ' +
    'atau aktifkan Autoplay untuk situs ini di pengaturan browser agar bunyi otomatis tanpa klik.'
  document.body.appendChild(el)
  setTimeout(() => el.remove(), 9000)
}

function playBellOnce() {
  const a = ensureAudio()
  const attempt = () => {
    try {
      a.currentTime = 0
      a.volume = 1
      const p = a.play()
      if (p && typeof p.catch === 'function') {
        p.catch(() => {
          if (!unlocked) {
            pendingAlarm = true
            scheduleRetry()
          } else {
            showBlockHint()
          }
        })
      }
    } catch (e) {
      if (!unlocked) {
        pendingAlarm = true
        scheduleRetry()
      } else {
        showBlockHint()
      }
    }
  }
  if (a.readyState >= 2) {
    attempt()
  } else {
    a.addEventListener('canplay', () => attempt(), { once: true })
  }
}

function scheduleRetry() {
  playAttempts++
  // Safari *can* still block HTMLAudio without a gesture, but speechSynthesis
  // is generally not gated the same way — speak as an audible fallback.
  if (playAttempts === 1 && !spokeOnce) {
    spokeOnce = true
    speakFallback(alarmName)
  }
  if (playAttempts > MAX_PLAY_ATTEMPTS) {
    showBlockHint()
    return
  }
  clearTimeout(retryTimer)
  retryTimer = setTimeout(() => {
    if (pendingAlarm) queueAlarm()
  }, RETRY_DELAY_MS)
}

// Desktop Safari & some strict browsers block normal audio without a gesture,
// but speechSynthesis typically plays anyway — so we speak the alarm instead.
function speakFallback(name) {
  try {
    if (!('speechSynthesis' in window)) return
    window.speechSynthesis.cancel()
    const u = new SpeechSynthesisUtterance(
      name ? `Alarm! ${name} tidak aktif` : 'Alarm! Router down'
    )
    u.lang = 'id-ID'
    u.volume = 1
    u.rate = 1
    window.speechSynthesis.speak(u)
  } catch (e) {
    /* noop */
  }
}

function queueAlarm() {
  clearTimeout(alarmTimer)
  playAttempts = 0
  let count = 0
  const ring = () => {
    if (count >= ALARM_REPEATS) return
    count++
    playBellOnce()
    alarmTimer = setTimeout(ring, ALARM_INTERVAL_MS)
  }
  ring()
}

// Pure status-driven: an alarm fires whenever the backend reports a customer
// went OFFLINE. No prior interaction is required for the attempt — browsers
// that allow autoplay (or have it enabled for this site) will sound it.
export function playAlarm(name) {
  clearTimeout(alarmTimer)
  clearTimeout(retryTimer)
  alarmName = name || ''
  spokeOnce = false
  ensureAudio()
  if (!unlocked) {
    pendingAlarm = true
  }
  queueAlarm()
}

// Register unlock listeners as early as possible so the very first engagement
// anywhere unblocks future alarms.
ensureAudio()