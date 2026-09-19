export const STATUS_COLORS = {
  ONLINE: '#22c55e',
  OFFLINE: '#ef4444',
  WARNING: '#eab308'
}

export const CUSTOMER_ICONS = [
  { value: 'customer', label: 'Pelanggan' },
  { value: 'wifi', label: 'WiFi' },
  { value: 'router', label: 'Router' },
  { value: 'jb', label: 'JB' },
  { value: 'server', label: 'Server' },
  { value: 'tower', label: 'Menara' },
  { value: 'building', label: 'Gedung' },
  { value: 'home', label: 'Rumah' },
  { value: 'dot', label: 'Titik' }
]

const CUSTOMER_ICON_INNER = {
  dot: '<circle cx="12" cy="12" r="6.2" fill="var(--ic-color)"/>',
  customer:
    '<circle cx="12" cy="8" r="4" fill="var(--ic-color)"/><path d="M4 21c0-4.2 3.6-6.5 8-6.5s8 2.3 8 6.5" fill="var(--ic-color)"/>',
  wifi:
    '<path d="M4.5 11.5a11 11 0 0 1 15 0"/><path d="M7.5 15.5a6.4 6.4 0 0 1 9 0"/><path d="M10.6 19.4a3 3 0 0 1 2.8 0"/>',
  jb:
    '<path d="M5.5 3.5h13v14l-3 3h-7l-3-3z" fill="var(--ic-color)"/><circle cx="9" cy="8.6" r="1.1" fill="#fff"/><circle cx="12" cy="8.6" r="1.1" fill="#fff"/><circle cx="15" cy="8.6" r="1.1" fill="#fff"/><path d="M9 14.2h6" stroke="#fff" stroke-width="1.4" stroke-linecap="round"/>',
  router:
    '<rect x="3" y="11" width="18" height="7.5" rx="2" fill="var(--ic-color)"/><rect x="5.5" y="7" width="13" height="4" rx="1" fill="var(--ic-color)" opacity=".55"/><path d="M7.5 15.2h.01M11 15.2h.01M16.6 7.4a6 6 0 0 1 0 3.4M19 6.5a9 9 0 0 1 0 5" stroke="var(--ic-color)" stroke-width="1.6" fill="none" stroke-linecap="round"/>',
  server:
    '<rect x="3" y="3.8" width="18" height="7" rx="1.6" fill="var(--ic-color)"/><rect x="3" y="13.2" width="18" height="7" rx="1.6" fill="var(--ic-color)"/><circle cx="7.4" cy="7.3" r="1.1" fill="#fff"/><circle cx="7.4" cy="16.7" r="1.1" fill="#fff"/><path d="M11 7.3h5.4M11 16.7h5.4" stroke="#fff" stroke-width="1.5" stroke-linecap="round"/>',
  tower:
    '<path d="M12 2.5 7.6 10h1.4l-1.9 9h9.8l-1.9-9h1.4z" fill="var(--ic-color)"/><rect x="10.9" y="2.5" width="2.2" height="7.5" fill="var(--ic-color)"/><path d="M6.4 7.5h11.2" stroke="var(--ic-color)" stroke-width="1.4" stroke-linecap="round"/>',
  building:
    '<path d="M5 21V5a2 2 0 0 1 2-2h6a2 2 0 0 1 2 2v16z" fill="var(--ic-color)"/><path d="M15 9h3a1 1 0 0 1 1 1v11h-4z" fill="var(--ic-color)"/><path d="M8.3 6h1.6M8.3 9.4h1.6M8.3 12.8h1.6M8.3 16.2h1.6" stroke="#fff" stroke-width="1.3" stroke-linecap="round"/>',
  home:
    '<path d="M4 10.6 12 4l8 6.6V20a1 1 0 0 1-1 1h-5v-6h-4v6H5a1 1 0 0 1-1-1z" fill="var(--ic-color)"/>'
}

export function customerIconSvg(name) {
  const key = CUSTOMER_ICON_INNER[name] ? name : 'customer'
  const inner = CUSTOMER_ICON_INNER[key]

  if (key === 'wifi') {
    return `<svg viewBox="0 0 24 24" fill="none" stroke="var(--ic-color)" stroke-width="2.2" stroke-linecap="round">${inner}</svg>`
  }
  return `<svg viewBox="0 0 24 24">${inner}</svg>`
}

export function statusColor(status) {
  return STATUS_COLORS[status] || '#94a3b8'
}