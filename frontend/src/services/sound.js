import bellUrl from '../assets/bel.mp3'

let audio = null

function ensureAudio() {
  if (audio) return audio
  audio = new Audio(bellUrl)

  const unlock = () => {
    audio.volume = 0
    audio.currentTime = 0
    audio.play()
      .then(() => {
        audio.pause()
        audio.currentTime = 0
      })
      .catch(() => {})
    window.removeEventListener('pointerdown', unlock)
    window.removeEventListener('touchstart', unlock)
  }
  window.addEventListener('pointerdown', unlock)
  window.addEventListener('touchstart', unlock)

  return audio
}

export function playOfflineBell() {
  const a = ensureAudio()
  try {
    a.currentTime = 0
    a.volume = 1
    a.play().catch(() => {})
  } catch (e) {
    /* noop */
  }
}