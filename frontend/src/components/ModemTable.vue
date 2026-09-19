<script setup>
import { computed } from 'vue'

const props = defineProps({
  rows: { type: Array, default: () => [] }
})

// Kolom = gabungan semua key dari seluruh baris (urutan kemunculan pertama).
const columns = computed(() => {
  const seen = new Set()
  const cols = []
  for (const row of props.rows) {
    if (!row || typeof row !== 'object') continue
    for (const k of Object.keys(row)) {
      if (!seen.has(k)) {
        seen.add(k)
        cols.push(k)
      }
    }
  }
  return cols
})

function cell(v) {
  if (v === null || v === undefined) return ''
  if (typeof v === 'object') return JSON.stringify(v)
  return String(v)
}
</script>

<template>
  <div class="md-table__wrap">
    <table class="md-table">
      <thead>
        <tr>
          <th v-for="c in columns" :key="c">{{ c }}</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="(row, i) in rows" :key="i">
          <td v-for="c in columns" :key="c">{{ cell(row[c]) }}</td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<style scoped>
.md-table__wrap {
  overflow-x: auto;
  border: 1px solid var(--border);
  border-radius: 8px;
}
.md-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 12px;
}
.md-table th,
.md-table td {
  padding: 7px 10px;
  text-align: left;
  border-bottom: 1px solid var(--border);
  white-space: nowrap;
}
.md-table th {
  color: var(--text-dim);
  font-weight: 600;
  background: var(--bg-panel-2);
  position: sticky;
  top: 0;
}
.md-table tr:last-child td {
  border-bottom: none;
}
</style>
