import bellUrl from '../assets/bel.mp3'
import { requestNotifyPermission } from './notify'

const ALARM_REPEATS = 3
const ALARM_INTERVAL_MS = 700

let audio = null
let unlocked = false
let pendingAlarm = false
let alarmTimer = null

function ensureAudio() {
  if (audio) return audio
  audio = new Audio(bellUrl)
  audio.preload = 'auto'

  document.addEventListener('pointerdown', handleUnlock)
  document.addEventListener('mousedown', handleUnlock)
  document.addEventListener('keydown', handleUnlock)
  document.addEventListener('touchstart', handleUnlock)

  return audio
}

function handleUnlock() {
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

  // Permission prompt is only allowed from a real user gesture.
  requestNotifyPermission()

  if (pendingAlarm) {
    pendingAlarm = false
    queueAlarm()
  }
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
          if (!unlocked) pendingAlarm = true
        })
      }
    } catch (e) {
      if (!unlocked) pendingAlarm = true
    }
  }
  if (a.readyState >= 2) {
    attempt()
  } else {
    a.addEventListener('canplay', attempt, { once: true })
  }
}

function queueAlarm() {
  clearTimeout(alarmTimer)
  let count = 0
  const ring = () => {
    if (count >= ALARM_REPEATS) return
    count++
    playBellOnce()
    alarmTimer = setTimeout(ring, ALARM_INTERVAL_MS)
  }
  ring()
}

export function playAlarm() {
  clearTimeout(alarmTimer)
  if (!unlocked) {
    pendingAlarm = true
    return
  }
  queueAlarm()
}