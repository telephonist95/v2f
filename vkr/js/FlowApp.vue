<script setup>
import { ref, markRaw, onMounted } from 'vue'
import { VueFlow, useVueFlow, MarkerType } from '@vue-flow/core'
import { Controls } from '@vue-flow/controls'
import '@vue-flow/core/dist/style.css'
import '@vue-flow/core/dist/theme-default.css'
import '@vue-flow/controls/dist/style.css'
import { domToPng } from 'modern-screenshot'
import FlowProcess from './FlowProcess.vue'
import FlowDecision from './FlowDecision.vue'
import FlowTerminal from './FlowTerminal.vue'

const nodeTypes = {
  process: markRaw(FlowProcess),
  decision: markRaw(FlowDecision),
  terminal: markRaw(FlowTerminal),
}

// --- helpers ---
function P(id, x, y, label) { return { id, type:'process',  position:{x,y}, data:{label} } }
function D(id, x, y, label) { return { id, type:'decision', position:{x,y}, data:{label} } }
function T(id, x, y, label) { return { id, type:'terminal', position:{x,y}, data:{label} } }

const arrow = { type: MarkerType.ArrowClosed, color:'#000', width:16, height:16 }
const eBase = { type:'smoothstep', markerEnd:arrow, style:{stroke:'#000',strokeWidth:2} }
const lStyle = { fontSize:'13px', fontStyle:'italic', fill:'#000', fontFamily:"'GOST Type B','GOST2304 Type B',Arial" }

function E(id, s, t, sh, th, label) {
  const e = { id, source:s, target:t, sourceHandle:`${s}-s-${sh}`, targetHandle:`${t}-t-${th}`, ...eBase }
  if (label) { e.label=label; e.labelStyle=lStyle; e.labelBgStyle={fill:'#fff',fillOpacity:1}; e.labelBgPadding=[4,3]; e.labelBgBorderRadius=0 }
  return e
}

// Алгоритм трассировки
const CX = 400, LX = 100, RX = 700

const routingNodes = [
  T('start',   CX, 0,     'Начало'),
  P('init',    CX, 90,    'Инициализировать аллокатор:\nadjTracks[], busTracks[],\nbusTrackY := baseY - busGap'),
  P('sort',    CX, 210,   'Отсортировать цепи по ID'),
  D('loop',    CX, 310,   'Есть\nнеобработанная\nцепь?'),
  P('find',    CX, 440,   'Определить источник\n(driver) и приёмники (sinks)'),
  D('miss',    CX, 550,   'Источник или\nприёмники\nотсутствуют?'),
  P('skip',    CX-280, 555,'Пропустить цепь'),
  D('adj',     CX, 690,   'Все приёмники\nв столбце\ndriver_col + 1?'),

  // --- adjacent (left) ---
  P('a_track', LX, 830,   'allocChanTrack(ch):\nтрек в левой зоне канала'),
  P('a_wire1', LX, 940,   'Провод: источник →\nгоризонталь до трека'),
  P('a_ys',    LX, 1050,  'Собрать Y источника\nи приёмников,\nотсортировать'),
  D('a_yloop', LX, 1170,  'Есть пара\nсоседних Y?'),
  P('a_vseg',  LX-230, 1175, 'Провод: вертикальный\nсегмент Y[i]→Y[i+1]'),
  D('a_sink',  LX, 1300,  'Есть\nприёмник?'),
  P('a_hseg',  LX-230, 1305, 'Провод: горизонталь\nот трека к приёмнику'),

  // --- bus (right) ---
  P('b_busY',  RX, 830,   'allocBusY():\nY-координата на шине'),
  P('b_srcTr', RX, 940,   'allocBusDropTrack(srcCh):\nтрек в правой зоне\nканала источника'),
  P('b_wire1', RX, 1060,  'Провод: источник →\nтрек → вверх до шины'),
  P('b_group', RX, 1170,  'Сгруппировать приёмники\nпо каналам (dropGroups)'),
  D('b_gloop', RX, 1280,  'Есть группа\nприёмников?'),
  P('b_gtr',   RX+30, 1410, 'allocBusDropTrack(ch):\nтрек правой зоны\nканала группы'),
  P('b_gys',   RX+30, 1530, 'Собрать Y: busY +\nприёмники группы'),
  D('b_gyloop',RX+30, 1640, 'Есть пара\nсоседних Y?'),
  P('b_gvseg', RX+300,1645, 'Провод: вертикальный\nсегмент Y[i]→Y[i+1]'),
  D('b_gsink', RX+30, 1770, 'Есть приёмник\nв группе?'),
  P('b_ghseg', RX+300,1775, 'Провод: горизонталь\nот трека к приёмнику'),

  P('b_xs',    RX, 1910,  'Собрать X точек\nподключения к шине'),
  D('b_xloop', RX, 2020,  'Есть пара\nсоседних X?'),
  P('b_hbus',  RX+270,2025,'Провод: горизонтальный\nсегмент шины X[i]→X[i+1]'),

  T('end',     CX, 2170,  'Конец'),
]

const routingEdges = [
  E('e1','start','init','bot','top'),
  E('e2','init','sort','bot','top'),
  E('e3','sort','loop','bot','top'),
  E('e4','loop','find','bot','top','да'),
  E('e5','find','miss','bot','top'),
  E('e6','miss','skip','left','right','да'),
  E('e7','miss','adj','bot','top','нет'),
  E('e8','skip','loop','top','left'),

  // adjacent
  E('e10','adj','a_track','left','top','да'),
  E('e11','a_track','a_wire1','bot','top'),
  E('e12','a_wire1','a_ys','bot','top'),
  E('e13','a_ys','a_yloop','bot','top'),
  E('e14','a_yloop','a_vseg','left','right','да'),
  E('e15','a_vseg','a_yloop','top','left'),
  E('e16','a_yloop','a_sink','bot','top','нет'),
  E('e17','a_sink','a_hseg','left','right','да'),
  E('e18','a_hseg','a_sink','top','left'),
  E('e19','a_sink','end','bot','top','нет'),

  // bus
  E('e20','adj','b_busY','right','top','нет'),
  E('e21','b_busY','b_srcTr','bot','top'),
  E('e22','b_srcTr','b_wire1','bot','top'),
  E('e23','b_wire1','b_group','bot','top'),
  E('e24','b_group','b_gloop','bot','top'),
  E('e25','b_gloop','b_gtr','bot','top','да'),
  E('e26','b_gtr','b_gys','bot','top'),
  E('e27','b_gys','b_gyloop','bot','top'),
  E('e28','b_gyloop','b_gvseg','right','left','да'),
  E('e29','b_gvseg','b_gyloop','top','right'),
  E('e30','b_gyloop','b_gsink','bot','top','нет'),
  E('e31','b_gsink','b_ghseg','right','left','да'),
  E('e32','b_ghseg','b_gsink','top','right'),
  E('e33','b_gsink','b_gloop','left','right','нет'),
  E('e34','b_gloop','b_xs','bot','top','нет'),
  E('e35','b_xs','b_xloop','bot','top'),
  E('e36','b_xloop','b_hbus','right','left','да'),
  E('e37','b_hbus','b_xloop','top','right'),
  E('e38','b_xloop','end','bot','top','нет'),

  // main loop back
  E('e40','loop','end','right','right','нет'),
]

const KEY = 'flow-routing-v1'
const nodes = ref(loadPos(routingNodes))
const edges = ref(routingEdges)
const { fitView, onNodeDragStop } = useVueFlow({ id:'flow-r' })
const exporting = ref(false)

function loadPos(list) {
  try {
    const s = JSON.parse(localStorage.getItem(KEY)||'{}')
    return list.map(n => s[n.id] ? {...n,position:s[n.id]} : n)
  } catch { return list }
}

onNodeDragStop(({ nodes:m }) => {
  const s = JSON.parse(localStorage.getItem(KEY)||'{}')
  for (const n of m) s[n.id] = {x:n.position.x, y:n.position.y}
  localStorage.setItem(KEY, JSON.stringify(s))
})

onMounted(() => setTimeout(() => fitView({padding:.05}), 300))

async function exportPng() {
  const el = document.querySelector('.vue-flow')
  if (!el) return
  exporting.value = true
  try {
    const url = await domToPng(el, {
      scale:4, backgroundColor:'#fff',
      filter: n => { const c=n?.classList; if(!c)return true; return !c.contains('vue-flow__controls')&&!c.contains('vue-flow__background') },
    })
    Object.assign(document.createElement('a'),{download:'algo-routing.png',href:url}).click()
  } catch(e){alert(String(e))} finally{exporting.value=false}
}
</script>

<template>
  <div class="app">
    <div class="toolbar">
      <span class="tl">Алгоритм трассировки соединений</span>
      <button class="btn" :disabled="exporting" @click="exportPng">{{exporting?'...':'PNG 4x'}}</button>
    </div>
    <div class="canvas">
      <VueFlow :nodes="nodes" :edges="edges" :node-types="nodeTypes"
        :default-edge-options="{type:'smoothstep'}" :min-zoom=".05" :max-zoom="4"
        :snap-to-grid="true" :snap-grid="[5,5]" fit-view-on-init class="flow">
        <Controls position="bottom-left"/>
      </VueFlow>
    </div>
  </div>
</template>

<style>
@font-face{font-family:'GOST Type B';src:url('/fonts/GOST2304_TypeB.ttf') format('truetype');font-weight:normal;font-style:normal}
@font-face{font-family:'GOST Type B';src:url('/fonts/GOST2304_TypeB_italic.ttf') format('truetype');font-weight:normal;font-style:italic}
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:'GOST Type B',Arial;background:#fff;color:#000}
.vue-flow__controls{border-radius:4px!important;overflow:hidden}
.app{display:flex;flex-direction:column;height:100vh}
.toolbar{display:flex;align-items:center;justify-content:space-between;padding:8px 20px;border-bottom:1px solid #ccc;background:#fafafa}
.tl{font-size:15px;font-weight:700}
.btn{padding:6px 16px;background:#333;color:#fff;border:none;font:600 13px 'GOST Type B',Arial;cursor:pointer}
.btn:hover:not(:disabled){background:#555}.btn:disabled{opacity:.5}
.canvas{flex:1;position:relative;min-height:0}
.flow{width:100%;height:100%}
</style>
