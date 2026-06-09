<script setup>
import { computed, ref, watch } from 'vue'
import { ZoomIn, ZoomOut, Maximize2, FolderTree, Activity } from 'lucide-vue-next'

const props = defineProps({
  data: { type: Object, default: () => null }, // VCDFile-shaped
  log: { type: String, default: '' },
})

const pxPerUnit = ref(2.0)
const showHierarchy = ref(false)
const rowHeight = 32
const tracesEl = ref(null)
const cursorTime = ref(null) // VCD time at cursor, or null

const hasData = computed(
  () => !!props.data && Array.isArray(props.data.signals) && props.data.signals.length > 0,
)
const endTime = computed(() => Number(props.data?.endTime || 0))

const signals = computed(() => {
  if (!hasData.value) return []
  return props.data.signals.map((s) => ({
    ...s,
    displayName: scopeJoin(s.scope) + (s.name || ''),
  }))
})

function scopeJoin(scope) {
  if (!showHierarchy.value || !scope || !scope.length) return ''
  return scope.join('.') + '.'
}

// Per-signal change index.
const changesById = computed(() => {
  const idx = new Map()
  if (!props.data) return idx
  for (const ch of props.data.changes || []) {
    if (!idx.has(ch.id)) idx.set(ch.id, [])
    idx.get(ch.id).push(ch)
  }
  for (const arr of idx.values()) arr.sort((a, b) => a.time - b.time)
  return idx
})

const svgWidth = computed(() => Math.max(800, Math.ceil(endTime.value * pxPerUnit.value) + 80))
const svgHeight = computed(() => signals.value.length * rowHeight + 24)

const gridTicks = computed(() => {
  const t = endTime.value || 1
  const step = niceStep(t / 10)
  const ticks = []
  for (let x = 0; x <= t; x += step) ticks.push(x)
  return ticks
})

function niceStep(raw) {
  if (raw <= 0) return 1
  const mag = Math.pow(10, Math.floor(Math.log10(raw)))
  const norm = raw / mag
  let nice
  if (norm < 1.5) nice = 1
  else if (norm < 3.5) nice = 2
  else if (norm < 7.5) nice = 5
  else nice = 10
  return nice * mag
}

function buildScalarPath(id) {
  const chs = changesById.value.get(id) || []
  if (!chs.length) return ''
  const yHigh = 6
  const yLow = rowHeight - 6
  const yMid = (yHigh + yLow) / 2
  const lvl = (v) => {
    if (v === '0') return yLow
    if (v === '1') return yHigh
    return yMid
  }
  let d = ''
  let prevY = lvl(chs[0].value)
  d += `M0 ${prevY}`
  for (let i = 0; i < chs.length; i++) {
    const x = chs[i].time * pxPerUnit.value
    const y = lvl(chs[i].value)
    d += ` L${x} ${prevY}`
    if (y !== prevY) d += ` L${x} ${y}`
    prevY = y
  }
  d += ` L${endTime.value * pxPerUnit.value} ${prevY}`
  return d
}

function buildVectorSegments(id, width) {
  const chs = changesById.value.get(id) || []
  if (!chs.length) return []
  const segs = []
  let prev = chs[0]
  for (let i = 1; i < chs.length; i++) {
    segs.push({
      x1: prev.time * pxPerUnit.value,
      x2: chs[i].time * pxPerUnit.value,
      value: prev.value,
      label: vecLabel(prev.value, width),
      hasX: hasUnknown(prev.value),
    })
    prev = chs[i]
  }
  segs.push({
    x1: prev.time * pxPerUnit.value,
    x2: endTime.value * pxPerUnit.value,
    value: prev.value,
    label: vecLabel(prev.value, width),
    hasX: hasUnknown(prev.value),
  })
  return segs
}

function hasUnknown(v) {
  return typeof v === 'string' && /[xXzZ]/.test(v)
}

function vecLabel(value, width) {
  if (value === undefined || value === null) return ''
  if (typeof value !== 'string') return String(value)
  if (value.startsWith('r')) return value.slice(1)
  let bits = value
  if (bits.length < width) {
    const pad = bits[0] === '1' ? '0' : bits[0]
    bits = pad.repeat(width - bits.length) + bits
  }
  if (hasUnknown(bits)) return bits
  const hexDigits = Math.ceil(width / 4)
  const padded = bits.padStart(hexDigits * 4, '0')
  let hex = ''
  for (let i = 0; i < padded.length; i += 4) {
    hex += parseInt(padded.slice(i, i + 4), 2).toString(16).toUpperCase()
  }
  return '0x' + hex
}

// Return the most recent value of a signal at time t, or '' if undefined yet.
function valueAt(id, t) {
  const arr = changesById.value.get(id)
  if (!arr || !arr.length) return ''
  if (t == null) return arr[arr.length - 1].value
  // Binary search for last change with time <= t.
  let lo = 0
  let hi = arr.length - 1
  let best = -1
  while (lo <= hi) {
    const mid = (lo + hi) >> 1
    if (arr[mid].time <= t) {
      best = mid
      lo = mid + 1
    } else {
      hi = mid - 1
    }
  }
  return best >= 0 ? arr[best].value : ''
}

function displayValueAt(sig, t) {
  const raw = valueAt(sig.id, t)
  if (raw === '') return '—'
  if (sig.width === 1) return raw
  return vecLabel(raw, sig.width)
}

function zoomIn() {
  pxPerUnit.value = Math.min(64, pxPerUnit.value * 1.5)
}
function zoomOut() {
  pxPerUnit.value = Math.max(0.05, pxPerUnit.value / 1.5)
}
function fit() {
  const t = endTime.value || 1
  pxPerUnit.value = Math.max(0.1, Math.min(64, 1000 / t))
}

function onTracesMove(ev) {
  const el = tracesEl.value
  if (!el) return
  const rect = el.getBoundingClientRect()
  const x = ev.clientX - rect.left + el.scrollLeft
  const t = x / pxPerUnit.value
  if (t < 0 || t > endTime.value) {
    cursorTime.value = null
    return
  }
  cursorTime.value = Math.round(t * 1000) / 1000
}

function onTracesLeave() {
  cursorTime.value = null
}

const cursorX = computed(() => {
  if (cursorTime.value == null) return -1
  return cursorTime.value * pxPerUnit.value
})

watch(
  () => props.data,
  () => {
    if (hasData.value) fit()
    cursorTime.value = null
  },
  { immediate: true },
)
</script>

<template>
  <div class="wf-host">
    <div v-if="!hasData" class="wf-empty">
      <div class="wf-empty-card">
        <Activity :size="42" />
        <div class="wf-empty-title">Симуляция не выполнена</div>
        <div class="wf-empty-hint">
          После синтеза система автоматически запускает iverilog/vvp. Если в исходном коде
          есть testbench с <code>$dumpvars</code> — он будет использован, иначе сгенерируется
          автоматический testbench по портам модуля.
        </div>
        <pre v-if="log" class="wf-log">{{ log }}</pre>
      </div>
    </div>

    <template v-else>
      <div class="wf-toolbar">
        <div class="wf-meta">
          <span class="meta-pill"><b>{{ signals.length }}</b> сигналов</span>
          <span class="meta-pill"><b>{{ (data.changes || []).length }}</b> изменений</span>
          <span class="meta-pill">
            <b>{{ endTime }}</b> {{ data.timescale || '' }}
          </span>
          <span v-if="cursorTime != null" class="meta-pill meta-cursor">
            T = <b>{{ cursorTime }}</b>
          </span>
        </div>
        <span class="wf-spacer"></span>
        <button class="wf-btn" @click="zoomOut" title="Уменьшить (Ctrl + −)">
          <ZoomOut :size="14" />
        </button>
        <button class="wf-btn" @click="zoomIn" title="Увеличить (Ctrl + +)">
          <ZoomIn :size="14" />
        </button>
        <button class="wf-btn" @click="fit" title="По размеру">
          <Maximize2 :size="14" />
        </button>
        <label class="wf-toggle" :class="{ on: showHierarchy }">
          <input type="checkbox" v-model="showHierarchy" />
          <FolderTree :size="14" />
          <span>иерархия</span>
        </label>
      </div>

      <div class="wf-body">
        <div class="wf-names">
          <div class="wf-names-head">
            <span class="hn-name">Сигнал</span>
            <span class="hn-val">Значение</span>
          </div>
          <div
            v-for="(sig, i) in signals"
            :key="sig.id + '_' + i"
            class="wf-name"
            :class="{ stripe: i % 2 === 1 }"
            :style="{ height: rowHeight + 'px' }"
            :title="sig.displayName + ' (' + sig.kind + ', ' + sig.width + ' бит)'"
          >
            <span class="wf-name-text">{{ sig.displayName }}</span>
            <span v-if="sig.width > 1" class="wf-name-width">[{{ sig.width - 1 }}:0]</span>
            <span class="wf-name-val">{{ displayValueAt(sig, cursorTime) }}</span>
          </div>
        </div>

        <div
          class="wf-traces"
          ref="tracesEl"
          @mousemove="onTracesMove"
          @mouseleave="onTracesLeave"
        >
          <svg :width="svgWidth" :height="svgHeight" xmlns="http://www.w3.org/2000/svg">
            <!-- alternating row backgrounds -->
            <g class="wf-stripes">
              <rect
                v-for="(sig, i) in signals"
                :key="'stripe_' + sig.id + '_' + i"
                v-if="i % 2 === 1"
                x="0"
                :y="i * rowHeight + 16"
                :width="svgWidth"
                :height="rowHeight"
              />
            </g>

            <!-- Vertical grid -->
            <g class="wf-grid">
              <line
                v-for="t in gridTicks"
                :key="'grid_' + t"
                :x1="t * pxPerUnit"
                y1="0"
                :x2="t * pxPerUnit"
                :y2="svgHeight"
              />
              <text
                v-for="t in gridTicks"
                :key="'tlabel_' + t"
                :x="t * pxPerUnit + 3"
                :y="12"
                class="wf-tick-label"
              >{{ t }}</text>
            </g>

            <!-- Cursor -->
            <line
              v-if="cursorX >= 0"
              class="wf-cursor"
              :x1="cursorX"
              y1="0"
              :x2="cursorX"
              :y2="svgHeight"
            />

            <!-- Traces -->
            <g
              v-for="(sig, i) in signals"
              :key="sig.id + '_trace_' + i"
              :transform="`translate(0, ${i * rowHeight + 16})`"
            >
              <line :x1="0" :y1="rowHeight - 1" :x2="svgWidth" :y2="rowHeight - 1" class="wf-baseline" />

              <template v-if="sig.width === 1">
                <path :d="buildScalarPath(sig.id)" class="wf-scalar" />
              </template>
              <template v-else>
                <g class="wf-vector">
                  <polygon
                    v-for="(seg, idx) in buildVectorSegments(sig.id, sig.width)"
                    :key="'vbox_' + idx"
                    :points="`
                      ${seg.x1},${rowHeight / 2}
                      ${seg.x1 + 4},6
                      ${seg.x2 - 4},6
                      ${seg.x2},${rowHeight / 2}
                      ${seg.x2 - 4},${rowHeight - 6}
                      ${seg.x1 + 4},${rowHeight - 6}
                    `"
                    :class="{ 'wf-vbox-x': seg.hasX }"
                  />
                  <text
                    v-for="(seg, idx) in buildVectorSegments(sig.id, sig.width)"
                    :key="'vlabel_' + idx"
                    :x="(seg.x1 + seg.x2) / 2"
                    :y="rowHeight / 2 + 4"
                    class="wf-vlabel"
                  >{{ (seg.x2 - seg.x1) > 28 ? seg.label : '' }}</text>
                </g>
              </template>
            </g>
          </svg>
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.wf-host {
  display: flex;
  flex-direction: column;
  height: 100%;
  width: 100%;
  background: var(--ctp-base);
  color: var(--ctp-text);
  font-family: 'JetBrains Mono', 'Cascadia Code', Menlo, monospace;
  font-size: 12px;
}

/* Empty card */
.wf-empty {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
}
.wf-empty-card {
  max-width: 540px;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  padding: 32px;
  background: var(--ctp-mantle);
  border: 1px solid var(--ctp-surface0);
  border-radius: 10px;
  color: var(--ctp-subtext);
  text-align: center;
}
.wf-empty-card > :first-child {
  color: var(--ctp-overlay);
}
.wf-empty-title {
  color: var(--ctp-text);
  font-size: 16px;
  font-weight: 700;
}
.wf-empty-hint {
  font-family: Inter, sans-serif;
  font-size: 13px;
  line-height: 1.5;
}
.wf-empty-hint code {
  font-family: 'JetBrains Mono', monospace;
  background: var(--ctp-surface0);
  padding: 1px 5px;
  border-radius: 3px;
}
.wf-log {
  width: 100%;
  text-align: left;
  white-space: pre-wrap;
  background: var(--ctp-crust);
  padding: 10px 12px;
  border: 1px solid var(--ctp-surface0);
  border-radius: 6px;
  max-height: 40vh;
  overflow: auto;
  font-size: 11px;
  color: var(--ctp-overlay);
}

/* Toolbar */
.wf-toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
  height: 40px;
  padding: 0 12px;
  background: var(--ctp-mantle);
  border-bottom: 1px solid var(--ctp-surface0);
}
.wf-meta {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.meta-pill {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 3px 9px;
  background: var(--ctp-surface0);
  border: 1px solid var(--ctp-surface1);
  border-radius: 999px;
  font-size: 11px;
  color: var(--ctp-subtext);
}
.meta-pill b {
  color: var(--ctp-text);
  font-weight: 700;
  margin-right: 2px;
}
.meta-cursor {
  border-color: var(--ctp-yellow);
  color: var(--ctp-yellow);
}
.meta-cursor b { color: var(--ctp-yellow); }

.wf-spacer { flex: 1; }

.wf-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  height: 28px;
  min-width: 28px;
  padding: 0 8px;
  background: var(--ctp-surface0);
  color: var(--ctp-text);
  border: 1px solid var(--ctp-surface1);
  border-radius: 5px;
  cursor: pointer;
}
.wf-btn:hover {
  border-color: var(--ctp-lavender);
  color: var(--ctp-lavender);
}

.wf-toggle {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 0 10px;
  height: 28px;
  background: var(--ctp-surface0);
  border: 1px solid var(--ctp-surface1);
  border-radius: 5px;
  color: var(--ctp-subtext);
  cursor: pointer;
  user-select: none;
}
.wf-toggle:hover { border-color: var(--ctp-lavender); }
.wf-toggle.on {
  border-color: var(--ctp-mauve);
  color: var(--ctp-mauve);
}
.wf-toggle input {
  position: absolute;
  opacity: 0;
  pointer-events: none;
}

/* Body */
.wf-body {
  flex: 1;
  display: flex;
  min-height: 0;
}
.wf-names {
  flex: 0 0 280px;
  display: flex;
  flex-direction: column;
  background: var(--ctp-mantle);
  border-right: 1px solid var(--ctp-surface0);
  overflow-y: auto;
  overflow-x: hidden;
}
.wf-names-head {
  display: flex;
  align-items: center;
  height: 20px;
  padding: 0 10px;
  background: var(--ctp-base);
  border-bottom: 1px solid var(--ctp-surface0);
  font-size: 10px;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  color: var(--ctp-overlay);
  font-weight: 700;
  gap: 6px;
}
.hn-name { flex: 1; }
.hn-val  { width: 84px; text-align: right; }

.wf-name {
  padding: 0 10px;
  display: flex;
  align-items: center;
  gap: 6px;
  white-space: nowrap;
  border-bottom: 1px solid var(--ctp-surface0);
  overflow: hidden;
}
.wf-name.stripe { background: var(--ctp-base); }
.wf-name-text {
  flex: 1;
  text-overflow: ellipsis;
  overflow: hidden;
  color: var(--ctp-text);
}
.wf-name-width {
  color: var(--ctp-lavender);
  font-size: 10px;
}
.wf-name-val {
  width: 84px;
  text-align: right;
  color: var(--ctp-yellow);
  font-weight: 700;
  font-size: 11px;
  text-overflow: ellipsis;
  overflow: hidden;
}

/* Traces */
.wf-traces {
  flex: 1;
  overflow: auto;
  background: var(--ctp-base);
  cursor: crosshair;
}
.wf-stripes rect {
  fill: var(--ctp-mantle);
  fill-opacity: 0.35;
}
.wf-grid line {
  stroke: var(--ctp-surface0);
  stroke-width: 1;
}
.wf-tick-label {
  font-size: 9px;
  fill: var(--ctp-subtext);
}
.wf-baseline {
  stroke: var(--ctp-surface0);
  stroke-width: 1;
  shape-rendering: crispEdges;
}
.wf-scalar {
  fill: none;
  stroke: var(--ctp-green);
  stroke-width: 1.5;
  shape-rendering: crispEdges;
}
.wf-vector polygon {
  fill: var(--ctp-surface0);
  stroke: var(--ctp-blue);
  stroke-width: 1;
}
.wf-vector polygon.wf-vbox-x {
  fill: var(--ctp-red);
  stroke: var(--ctp-red);
  fill-opacity: 0.25;
}
.wf-vlabel {
  fill: var(--ctp-text);
  text-anchor: middle;
  font-size: 10px;
  font-family: 'JetBrains Mono', monospace;
  pointer-events: none;
}
.wf-cursor {
  stroke: var(--ctp-yellow);
  stroke-width: 1;
  stroke-dasharray: 3 3;
  pointer-events: none;
  opacity: 0.85;
}
</style>
