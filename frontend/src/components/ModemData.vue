<script setup>
import ModemTable from './ModemTable.vue'

const props = defineProps({
  data: { default: null }
})

function isObj(v) {
  return v !== null && typeof v === 'object' && !Array.isArray(v)
}
function isRowArray(v) {
  return Array.isArray(v) && v.length > 0 && isObj(v[0])
}
function hasRows(v) {
  return isObj(v) && Array.isArray(v.rows)
}
function fmt(v) {
  if (v === null || v === undefined) return '-'
  if (typeof v === 'object') return JSON.stringify(v, null, 2)
  return String(v)
}
</script>

<template>
  <div class="md">
    <!-- Bentuk TableResult: { table, rows: [...] } -->
    <template v-if="hasRows(data)">
      <div v-if="data.table" class="md__title">{{ data.table }}</div>
      <ModemTable :rows="data.rows" />
    </template>

    <!-- Array baris langsung -->
    <template v-else-if="isRowArray(data)">
      <ModemTable :rows="data" />
    </template>

    <!-- Objek dengan campuran tabel & skalar -->
    <template v-else-if="isObj(data)">
      <template v-for="(v, k) in data" :key="k">
        <template v-if="isRowArray(v)">
          <div class="md__title">{{ k }}</div>
          <ModemTable :rows="v" />
        </template>
        <template v-else-if="hasRows(v)">
          <div class="md__title">{{ v.table || k }}</div>
          <ModemTable :rows="v.rows" />
        </template>
        <template v-else-if="isObj(v)">
          <div class="md__title">{{ k }}</div>
          <ModemData :data="v" />
        </template>
        <div v-else class="md__kv">
          <span class="md__k">{{ k }}</span>
          <span class="md__v">{{ fmt(v) }}</span>
        </div>
      </template>
    </template>

    <pre v-else-if="data !== null && data !== undefined" class="md__raw">{{ fmt(data) }}</pre>
    <div v-else class="md__empty">Tidak ada data</div>
  </div>
</template>

<style scoped>
.md {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.md__title {
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: var(--text-dim);
  margin-top: 4px;
}
.md__kv {
  display: grid;
  grid-template-columns: auto 1fr;
  gap: 2px 16px;
  font-size: 12.5px;
  padding: 2px 0;
}
.md__k {
  color: var(--text-dim);
}
.md__v {
  color: var(--text);
  word-break: break-word;
}
.md__raw {
  margin: 0;
  padding: 12px;
  background: var(--bg-panel-2);
  border: 1px solid var(--border);
  border-radius: 8px;
  font-size: 12px;
  white-space: pre-wrap;
  word-break: break-word;
  max-height: 50vh;
  overflow: auto;
}
.md__empty {
  color: var(--text-faint);
  font-size: 12.5px;
}
</style>
