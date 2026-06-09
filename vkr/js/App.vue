<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { compressToEncodedURIComponent } from 'lz-string'
import {
  Upload,
  Play,
  Loader2,
  CircleAlert,
  Download,
  ExternalLink,
  FileDown,
  Code2,
  Cpu,
  Activity,
  Braces,
  ScrollText,
  Network,
  Boxes,
  Tags,
  Spline,
  Sun,
  Moon,
} from 'lucide-vue-next'
import CodeEditor from './CodeEditor.vue'
import GostSvgEditor from './GostSvgEditor.vue'
import Waveform from './Waveform.vue'

const sampleSource = `module counter (
    input  logic       clk,
    input  logic       rst_n,
    input  logic       en,
    output logic [3:0] count
);

    always_ff @(posedge clk) begin
        if (!rst_n)
            count <= 4'd0;
        else if (en)
            count <= count + 1'b1;
    end

endmodule
`

const source = ref(sampleSource)
const top = ref('counter')
const fileName = ref('counter.sv')
const busy = ref(false)
const error = ref('')
const yosysLog = ref('')
const result = ref(null)
const level = ref('rtl')
const view = ref('gost')
const gostStyle = ref('wires')    // 'wires' | 'labels'
const falstadStyle = ref('wires') // 'wires' | 'labels'
const falstadText = ref('')

// Theme (Catppuccin Mocha = dark / Latte = light)
const THEME_KEY = 'ver2fal.theme'
const theme = ref('dark')
function applyTheme(value) {
  document.documentElement.setAttribute('data-theme', value)
}
function toggleTheme() {
  theme.value = theme.value === 'dark' ? 'light' : 'dark'
  try { localStorage.setItem(THEME_KEY, theme.value) } catch (_) {}
  applyTheme(theme.value)
}
onMounted(() => {
  try {
    const saved = localStorage.getItem(THEME_KEY)
    if (saved === 'light' || saved === 'dark') theme.value = saved
  } catch (_) {}
  applyTheme(theme.value)
})
const exporting = ref(false)
const gostEditor = ref(null)

const splitPercent = ref(42)
const resizing = ref(false)

const workspaceStyle = computed(() => ({
  gridTemplateColumns: `minmax(320px, ${splitPercent.value}%) 8px minmax(480px, ${100 - splitPercent.value}%)`,
}))

const activeCircuit = computed(() => {
  if (!result.value) return null
  return level.value === 'rtl' ? result.value.rtl : result.value.gate
})

const falstadUrl = computed(() => {
  if (!falstadText.value) return ''
  return `https://www.falstad.com/circuit/circuitjs.html?ctz=${compressToEncodedURIComponent(falstadText.value)}`
})

const falstadCanEmbed = computed(() => falstadUrl.value.length > 0 && falstadUrl.value.length < 7000)

const statusText = computed(() => {
  if (busy.value) return 'обработка'
  if (error.value) return 'ошибка'
  if (result.value) return `готово: ${result.value.top}`
  return 'ожидание'
})

function loadFile(event) {
  const file = event.target.files?.[0]
  if (!file) return
  fileName.value = file.name
  const reader = new FileReader()
  reader.onload = () => {
    source.value = String(reader.result || '')
    const match = source.value.match(/^\s*module\s+([A-Za-z_][A-Za-z0-9_$]*)\b/m)
    if (match) top.value = match[1]
  }
  reader.readAsText(file)
  event.target.value = ''
}

function startResize(event) {
  resizing.value = true
  event.currentTarget.setPointerCapture?.(event.pointerId)
}

function resizeWorkspace(event) {
  if (!resizing.value) return
  const rect = event.currentTarget.parentElement.getBoundingClientRect()
  const next = ((event.clientX - rect.left) / rect.width) * 100
  splitPercent.value = Math.min(68, Math.max(28, next))
}

function stopResize(event) {
  resizing.value = false
  event.currentTarget.releasePointerCapture?.(event.pointerId)
}

async function convert() {
  busy.value = true
  error.value = ''
  yosysLog.value = ''
  try {
    const response = await fetch('/api/convert', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ source: source.value, top: top.value }),
    })
    const payload = await response.json()
    if (!response.ok) {
      error.value = payload.error || 'conversion failed'
      yosysLog.value = payload.log || ''
      return
    }
    result.value = payload
    yosysLog.value = payload.yosysLog || ''
    level.value = 'rtl'
    view.value = 'gost'
    updateActiveCircuit()
  } catch (err) {
    error.value = String(err)
  } finally {
    busy.value = false
  }
}

function downloadText(name, text) {
  const blob = new Blob([text], { type: 'text/plain;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  Object.assign(document.createElement('a'), { href: url, download: name }).click()
  URL.revokeObjectURL(url)
}

function updateActiveCircuit() {
  const circuit = activeCircuit.value
  if (!circuit) {
    falstadText.value = ''
    return
  }
  falstadText.value = falstadStyle.value === 'labels'
    ? circuit.falstadLabeled || circuit.falstad || ''
    : circuit.falstad || ''
}

async function exportGostSvg() {
  if (!gostEditor.value) return
  exporting.value = true
  try {
    await gostEditor.value.exportSvg(`gost-${level.value}.svg`)
  } finally {
    exporting.value = false
  }
}

function exportGostJson() {
  const diagram = gostEditor.value?.editedDiagram?.() || activeCircuit.value?.gost || {}
  downloadText(`gost-${level.value}.json`, JSON.stringify(diagram, null, 2))
}

watch(level, updateActiveCircuit)
watch(activeCircuit, updateActiveCircuit)
watch(falstadStyle, updateActiveCircuit)
</script>

<template>
  <main class="shell">
    <header class="topbar">
      <div class="brand">
        <div class="title">
          <span class="title-strong">ver</span><span class="title-soft">2fal</span>
        </div>
        <div
          class="status"
          :class="{ bad: error, ok: result && !error, busy }"
          :title="statusText"
        >
          <span class="status-dot"></span>
          <span class="status-label">{{ statusText }}</span>
        </div>
      </div>

      <div class="actions">
        <label class="btn ghost" :title="`Загрузить .v или .sv (текущий: ${fileName})`">
          <Upload :size="16" />
          <span>Загрузить</span>
          <input type="file" accept=".v,.sv,.vh,.svh,text/plain" @change="loadFile" />
        </label>

        <div class="field">
          <label class="field-label">top</label>
          <input v-model="top" class="field-input" placeholder="имя модуля" />
        </div>

        <button class="btn primary" :disabled="busy" @click="convert">
          <Loader2 v-if="busy" :size="16" class="spin" />
          <Play v-else :size="16" />
          <span>{{ busy ? 'Синтезирую…' : 'Синтезировать' }}</span>
        </button>

        <button
          class="btn icon-btn"
          :title="theme === 'dark' ? 'Светлая тема' : 'Тёмная тема'"
          @click="toggleTheme"
        >
          <Sun v-if="theme === 'dark'" :size="16" />
          <Moon v-else :size="16" />
        </button>
      </div>
    </header>

    <section class="workspace" :style="workspaceStyle">
      <aside class="left-pane">
        <div class="pane-head">
          <span>Исходный код</span>
          <span class="pane-subtitle">SystemVerilog</span>
        </div>
        <CodeEditor v-model="source" class="editor" />
      </aside>

      <div
        class="splitter"
        :class="{ active: resizing }"
        @pointerdown="startResize"
        @pointermove="resizeWorkspace"
        @pointerup="stopResize"
        @pointercancel="stopResize"
      ></div>

      <section class="right-pane">
        <div class="tabs">
          <div class="seg" role="group" aria-label="Уровень абстракции">
            <span class="seg-label">Уровень</span>
            <button
              class="seg-btn"
              :class="{ active: level === 'rtl' }"
              :disabled="!result"
              @click="level = 'rtl'"
            >
              <Boxes :size="14" />
              <span>RTL</span>
            </button>
            <button
              class="seg-btn"
              :class="{ active: level === 'gate' }"
              :disabled="!result"
              @click="level = 'gate'"
            >
              <Cpu :size="14" />
              <span>Вентили</span>
            </button>
          </div>

          <div class="seg" role="group" aria-label="Вид">
            <span class="seg-label">Вид</span>
            <button
              class="seg-btn"
              :class="{ active: view === 'gost' }"
              :disabled="!result"
              @click="view = 'gost'"
            >
              <Network :size="14" />
              <span>ГОСТ</span>
            </button>
            <button
              class="seg-btn"
              :class="{ active: view === 'falstad' }"
              :disabled="!result"
              @click="view = 'falstad'"
            >
              <Code2 :size="14" />
              <span>Falstad</span>
            </button>
            <button
              class="seg-btn"
              :class="{ active: view === 'waveform' }"
              :disabled="!result"
              @click="view = 'waveform'"
            >
              <Activity :size="14" />
              <span>Диаграммы</span>
            </button>
            <button
              class="seg-btn"
              :class="{ active: view === 'json' }"
              :disabled="!result"
              @click="view = 'json'"
            >
              <Braces :size="14" />
              <span>JSON</span>
            </button>
            <button
              class="seg-btn"
              :class="{ active: view === 'log' }"
              @click="view = 'log'"
            >
              <ScrollText :size="14" />
              <span>Журнал</span>
            </button>
          </div>
        </div>

        <div v-if="error" class="error-banner" role="alert">
          <CircleAlert :size="16" />
          <span class="error-text">{{ error }}</span>
        </div>

        <div v-if="!result && !error" class="empty-state">
          <div class="empty-icon"><Cpu :size="40" /></div>
          <div class="empty-title">Готов к синтезу</div>
          <div class="empty-hint">
            Загрузите <code>.v</code>/<code>.sv</code> или отредактируйте код слева, затем нажмите
            <kbd>Синтезировать</kbd>.
          </div>
          <div class="empty-hint">
            После синтеза появятся вкладки <b>ГОСТ-схема</b>, <b>Falstad</b>, <b>Диаграммы</b>,
            <b>JSON</b> и <b>Журнал yosys</b>.
          </div>
        </div>

        <div v-else-if="view === 'gost'" class="view-body gost-view">
          <div class="gost-toolbar">
            <div class="stats" v-if="activeCircuit?.gost">
              <span>узлы {{ activeCircuit.gost.stats.nodes }}</span>
              <span>связи {{ activeCircuit.gost.stats.edges }}</span>
            </div>
            <div class="style-toggle" role="group" aria-label="Стиль соединений">
              <button
                class="style-btn"
                :class="{ active: gostStyle === 'wires' }"
                @click="gostStyle = 'wires'"
                title="Провода — связи рисуются ортогональными линиями"
              >
                <Spline :size="14" />
                <span>Провода</span>
              </button>
              <button
                class="style-btn"
                :class="{ active: gostStyle === 'labels' }"
                @click="gostStyle = 'labels'"
                title="Адресные метки — у каждого порта подпись имени сети"
              >
                <Tags :size="14" />
                <span>Метки</span>
              </button>
            </div>
            <span class="spacer"></span>
            <div class="gost-actions">
              <button class="btn ghost" :disabled="!result || exporting" @click="exportGostSvg">
                <FileDown :size="15" />
                <span>SVG</span>
              </button>
              <button class="btn ghost" :disabled="!result" @click="exportGostJson">
                <FileDown :size="15" />
                <span>JSON</span>
              </button>
            </div>
          </div>

          <div class="flow-wrap">
            <GostSvgEditor
              ref="gostEditor"
              :diagram="activeCircuit?.gost"
              :level="level"
              :connection-style="gostStyle"
              :net-labels="activeCircuit?.netLabels || {}"
            />
          </div>
        </div>

        <div v-else-if="view === 'falstad'" class="view-body falstad-view">
          <div class="falstad-actions">
            <div class="style-toggle" role="group" aria-label="Стиль соединений">
              <button
                class="style-btn"
                :class="{ active: falstadStyle === 'wires' }"
                @click="falstadStyle = 'wires'"
                title="Провода — ортогональные соединения между пинами"
              >
                <Spline :size="14" />
                <span>Провода</span>
              </button>
              <button
                class="style-btn"
                :class="{ active: falstadStyle === 'labels' }"
                @click="falstadStyle = 'labels'"
                title="Метки сетей (LabeledNodeElm) — пины соединяются по имени"
              >
                <Tags :size="14" />
                <span>Метки</span>
              </button>
            </div>
            <span class="spacer"></span>
            <a v-if="falstadUrl" class="btn primary link" :href="falstadUrl" target="_blank" rel="noreferrer">
              <ExternalLink :size="15" />
              <span>Открыть в Falstad</span>
            </a>
            <button class="btn ghost" :disabled="!falstadText" @click="downloadText(`${level}.${falstadStyle}.falstad.txt`, falstadText)">
              <FileDown :size="15" />
              <span>TXT</span>
            </button>
          </div>
          <iframe v-if="falstadCanEmbed" class="falstad-frame" :src="falstadUrl"></iframe>
          <CodeEditor v-else v-model="falstadText" read-only class="readonly-editor" language="text" />
        </div>

        <div v-else-if="view === 'waveform'" class="view-body waveform-view">
          <div class="waveform-toolbar">
            <span class="wf-state" v-if="result?.simulation">
              <template v-if="result.simulation.ok">
                симуляция: {{ result.simulation.autoTB ? 'автоматический testbench' : 'из исходного кода' }}
                <span v-if="result.simulation.topTB"> ({{ result.simulation.topTB }})</span>
              </template>
              <template v-else-if="result.simulation.error">
                <span class="wf-state-err">ошибка симуляции: {{ result.simulation.error }}</span>
              </template>
            </span>
            <span class="spacer"></span>
            <button
              class="btn ghost"
              :disabled="!result?.simulation?.vcdText"
              @click="downloadText(`${result?.top || 'design'}.vcd`, result.simulation.vcdText)"
            >
              <Download :size="15" />
              <span>VCD</span>
            </button>
          </div>
          <Waveform
            :data="result?.simulation?.parsed || null"
            :log="result?.simulation?.log || ''"
            class="waveform-pane"
          />
        </div>

        <div v-else-if="view === 'json'" class="view-body">
          <CodeEditor :model-value="result?.yosysJson || ''" read-only class="readonly-editor" language="json" />
        </div>

        <div v-else class="view-body">
          <CodeEditor :model-value="yosysLog" read-only class="readonly-editor" language="text" />
        </div>
      </section>
    </section>
  </main>
</template>

<style>
/* Catppuccin Mocha — dark theme (default). */
:root,
:root[data-theme="dark"] {
  --ctp-rosewater: #f5e0dc;
  --ctp-lavender: #b4befe;
  --ctp-mauve: #cba6f7;
  --ctp-blue: #89b4fa;
  --ctp-sky: #89dceb;
  --ctp-green: #a6e3a1;
  --ctp-yellow: #f9e2af;
  --ctp-red: #f38ba8;
  --ctp-text: #cdd6f4;
  --ctp-subtext: #a6adc8;
  --ctp-overlay: #6c7086;
  --ctp-surface2: #585b70;
  --ctp-surface1: #45475a;
  --ctp-surface0: #313244;
  --ctp-base: #1e1e2e;
  --ctp-mantle: #181825;
  --ctp-crust: #11111b;
}
/* Catppuccin Latte — light theme. Same role-based variable names, so all
   components automatically adapt without per-component overrides. */
:root[data-theme="light"] {
  --ctp-rosewater: #dc8a78;
  --ctp-lavender: #7287fd;
  --ctp-mauve: #8839ef;
  --ctp-blue: #1e66f5;
  --ctp-sky: #04a5e5;
  --ctp-green: #40a02b;
  --ctp-yellow: #df8e1d;
  --ctp-red: #d20f39;
  --ctp-text: #4c4f69;
  --ctp-subtext: #6c6f85;
  --ctp-overlay: #9ca0b0;
  --ctp-surface2: #acb0be;
  --ctp-surface1: #bcc0cc;
  --ctp-surface0: #ccd0da;
  --ctp-base: #eff1f5;
  --ctp-mantle: #e6e9ef;
  --ctp-crust: #dce0e8;
}
@font-face {
  font-family: "GOST Type B";
  src: url("/fonts/GOST2304_TypeB.ttf") format("truetype");
  font-weight: normal;
  font-style: normal;
}
@font-face {
  font-family: "GOST Type B Italic";
  src: url("/fonts/GOST2304_TypeB_italic.ttf") format("truetype");
  font-weight: normal;
  font-style: italic;
}
* { box-sizing: border-box; }
html, body, #app {
  width: 100%;
  height: 100%;
  margin: 0;
}
body {
  font-family: Inter, Arial, sans-serif;
  color: var(--ctp-text);
  background: var(--ctp-crust);
}
button, input, select {
  font: inherit;
}
.shell {
  display: flex;
  flex-direction: column;
  height: 100vh;
  min-width: 980px;
}

/* Topbar */
.topbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 18px;
  height: 56px;
  padding: 0 18px;
  border-bottom: 1px solid var(--ctp-surface0);
  background: linear-gradient(180deg, var(--ctp-mantle), var(--ctp-crust));
}
.brand {
  display: flex;
  align-items: center;
  gap: 14px;
}
.title {
  font-size: 20px;
  font-weight: 800;
  letter-spacing: -0.01em;
  display: inline-flex;
  align-items: baseline;
}
.title-strong { color: var(--ctp-lavender); }
.title-soft   { color: var(--ctp-text); }

.status {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 10px;
  border: 1px solid var(--ctp-surface1);
  font-size: 12px;
  color: var(--ctp-subtext);
  background: var(--ctp-base);
  border-radius: 999px;
  letter-spacing: 0.02em;
}
.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--ctp-overlay);
}
.status.ok       { border-color: rgba(166,227,161,0.55); color: var(--ctp-green); }
.status.ok .status-dot   { background: var(--ctp-green); box-shadow: 0 0 0 3px rgba(166,227,161,0.18); }
.status.bad      { border-color: rgba(243,139,168,0.55); color: var(--ctp-red); }
.status.bad .status-dot  { background: var(--ctp-red);   box-shadow: 0 0 0 3px rgba(243,139,168,0.18); }
.status.busy     { border-color: rgba(137,180,250,0.55); color: var(--ctp-blue); }
.status.busy .status-dot { background: var(--ctp-blue); animation: pulse 1.2s ease-in-out infinite; }

@keyframes pulse {
  0%, 100% { transform: scale(1);    opacity: 1; }
  50%      { transform: scale(1.35); opacity: 0.6; }
}

.actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

/* Unified button system */
.btn,
.file-btn,
button,
.link {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  height: 34px;
  padding: 0 12px;
  border: 1px solid var(--ctp-surface1);
  background: var(--ctp-surface0);
  color: var(--ctp-text);
  text-decoration: none;
  cursor: pointer;
  font-weight: 500;
  border-radius: 6px;
  transition: background 0.12s ease, border-color 0.12s ease, color 0.12s ease, transform 0.04s ease;
}
.btn:hover:not(:disabled),
.file-btn:hover,
button:hover:not(:disabled),
.link:hover {
  border-color: var(--ctp-lavender);
  background: var(--ctp-surface1);
  color: var(--ctp-rosewater);
}
.btn:active:not(:disabled),
button:active:not(:disabled) {
  transform: translateY(1px);
}
.btn:focus-visible,
button:focus-visible {
  outline: 2px solid var(--ctp-lavender);
  outline-offset: 1px;
}

.file-btn input,
.btn input[type="file"] { display: none; }

.primary,
.btn.primary {
  border-color: var(--ctp-blue);
  background: var(--ctp-blue);
  color: var(--ctp-crust);
  font-weight: 700;
}
.primary:hover:not(:disabled),
.btn.primary:hover:not(:disabled) {
  border-color: var(--ctp-lavender);
  background: var(--ctp-lavender);
  color: var(--ctp-crust);
}
button:disabled,
.btn:disabled {
  opacity: 0.45;
  cursor: not-allowed;
  filter: saturate(0.6);
}
.primary:disabled,
.btn.primary:disabled {
  opacity: 0.65;
  cursor: wait;
}
.ghost,
.btn.ghost {
  background: var(--ctp-base);
}
.icon-btn,
.btn.icon-btn {
  padding: 0 8px;
  background: var(--ctp-base);
  color: var(--ctp-subtext);
}
.icon-btn:hover:not(:disabled),
.btn.icon-btn:hover:not(:disabled) {
  color: var(--ctp-yellow);
  border-color: var(--ctp-yellow);
}

/* Spinner */
.spin {
  animation: spin 0.9s linear infinite;
}
@keyframes spin {
  to { transform: rotate(360deg); }
}

/* "Top module" labelled field */
.field {
  display: inline-flex;
  align-items: stretch;
  height: 34px;
  border: 1px solid var(--ctp-surface1);
  border-radius: 6px;
  overflow: hidden;
  background: var(--ctp-base);
}
.field:focus-within {
  border-color: var(--ctp-lavender);
}
.field-label {
  display: inline-flex;
  align-items: center;
  padding: 0 9px;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.08em;
  color: var(--ctp-subtext);
  background: var(--ctp-surface0);
  border-right: 1px solid var(--ctp-surface1);
  text-transform: uppercase;
}
.field-input {
  width: 130px;
  border: 0;
  padding: 0 10px;
  background: transparent;
  color: var(--ctp-text);
  font: inherit;
  outline: none;
}
.field-input::placeholder { color: var(--ctp-overlay); }
.workspace {
  display: grid;
  min-height: 0;
  flex: 1;
  background: var(--ctp-crust);
}
.left-pane,
.right-pane {
  min-height: 0;
  display: flex;
  flex-direction: column;
}
.left-pane {
  background: var(--ctp-base);
}
.pane-head,
.tabs,
.gost-toolbar,
.falstad-actions {
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 46px;
  padding: 8px 12px;
  border-bottom: 1px solid var(--ctp-surface0);
  background: var(--ctp-mantle);
}
.pane-head {
  justify-content: space-between;
  font-weight: 700;
  letter-spacing: 0.01em;
}
.pane-subtitle {
  color: var(--ctp-subtext);
  font-size: 12px;
  font-weight: 500;
}
.editor {
  flex: 1;
  min-height: 0;
}

/* Segmented controls (level / view) */
.tabs {
  flex-wrap: wrap;
  gap: 14px;
}
.seg {
  display: inline-flex;
  align-items: center;
  gap: 1px;
  padding: 3px;
  background: var(--ctp-base);
  border: 1px solid var(--ctp-surface1);
  border-radius: 7px;
}
.seg-label {
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: var(--ctp-overlay);
  padding: 0 8px 0 6px;
}
.seg-btn {
  height: 28px;
  padding: 0 10px;
  border: 0;
  background: transparent;
  border-radius: 5px;
  color: var(--ctp-subtext);
  font-weight: 500;
  font-size: 13px;
  gap: 6px;
}
.seg-btn:hover:not(:disabled) {
  background: var(--ctp-surface0);
  color: var(--ctp-text);
}
.seg-btn.active,
.seg-btn.active:hover {
  background: var(--ctp-surface1);
  color: var(--ctp-lavender);
  font-weight: 700;
}
.seg-btn:disabled {
  opacity: 0.35;
}
.splitter {
  position: relative;
  cursor: col-resize;
  background: transparent;
  touch-action: none;
}
.splitter::after {
  content: "";
  position: absolute;
  inset: 0;
  background: transparent;
  transition: background 0.12s ease;
}
.splitter:hover::after,
.splitter.active::after {
  background: rgba(137, 180, 250, 0.16);
}
.right-pane {
  background: var(--ctp-base);
}

/* Error banner */
.error-banner {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 14px;
  border-bottom: 1px solid rgba(243, 139, 168, 0.35);
  background: linear-gradient(180deg,
    rgba(243, 139, 168, 0.18),
    rgba(243, 139, 168, 0.08));
  color: var(--ctp-red);
  font-size: 13px;
}
.error-banner svg { flex-shrink: 0; }
.error-text {
  font-family: 'JetBrains Mono', monospace;
  font-size: 12px;
  white-space: pre-wrap;
}

/* Empty state */
.empty-state {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  padding: 40px 20px;
  text-align: center;
  color: var(--ctp-subtext);
}
.empty-icon {
  display: inline-flex;
  width: 72px;
  height: 72px;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  background: var(--ctp-surface0);
  color: var(--ctp-overlay);
  margin-bottom: 4px;
}
.empty-title {
  color: var(--ctp-text);
  font-size: 17px;
  font-weight: 700;
}
.empty-hint {
  max-width: 460px;
  line-height: 1.5;
  font-size: 13px;
}
.empty-hint code,
.empty-hint kbd,
.empty-hint b {
  font-family: 'JetBrains Mono', monospace;
  font-size: 12px;
  padding: 1px 6px;
  border: 1px solid var(--ctp-surface1);
  background: var(--ctp-surface0);
  color: var(--ctp-text);
  border-radius: 4px;
  font-weight: 600;
}
.empty-hint b { color: var(--ctp-lavender); }
.view-body {
  position: relative;
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}
.gost-view {
  background: var(--ctp-base);
}
.gost-toolbar {
  justify-content: space-between;
}
.stats {
  display: flex;
  gap: 10px;
  font-size: 13px;
  color: var(--ctp-subtext);
}
.gost-actions,
.falstad-actions {
  border: 0;
  padding: 0;
  background: transparent;
}
.flow-wrap {
  position: relative;
  flex: 1;
  min-height: 0;
}
.falstad-view {
  background: var(--ctp-base);
}
.falstad-actions {
  justify-content: flex-start;
}
.falstad-frame {
  flex: 1;
  width: 100%;
  min-height: 0;
  border: 0;
}
.readonly-editor {
  flex: 1;
  min-height: 0;
}
.waveform-view {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}
.waveform-toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
  height: 36px;
  padding: 0 12px;
  background: var(--ctp-mantle);
  border-bottom: 1px solid var(--ctp-surface0);
  color: var(--ctp-subtext);
  font-size: 12px;
}
.waveform-toolbar .spacer { flex: 1; }
.wf-state-err { color: var(--ctp-red); }
.waveform-pane {
  flex: 1;
  min-height: 0;
}

/* Style toggle (wires / labels) */
.style-toggle {
  display: inline-flex;
  align-items: center;
  gap: 1px;
  padding: 3px;
  background: var(--ctp-base);
  border: 1px solid var(--ctp-surface1);
  border-radius: 7px;
}
.style-btn {
  height: 26px;
  padding: 0 9px;
  border: 0;
  background: transparent;
  border-radius: 4px;
  color: var(--ctp-subtext);
  font-size: 12px;
  gap: 5px;
}
.style-btn:hover:not(.active) {
  background: var(--ctp-surface0);
  color: var(--ctp-text);
}
.style-btn.active,
.style-btn.active:hover {
  background: var(--ctp-surface1);
  color: var(--ctp-sky);
  font-weight: 700;
}
.falstad-actions .spacer,
.gost-toolbar .spacer { flex: 1; }
</style>
