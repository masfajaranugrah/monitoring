import { ref } from 'vue'

export const menuOpen = ref(false)

export function toggleMenu() {
  menuOpen.value = !menuOpen.value
}

export function closeMenu() {
  menuOpen.value = false
}