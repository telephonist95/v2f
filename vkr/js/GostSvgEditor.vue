<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import * as joint from '@joint/core'

const props = defineProps({
  diagram: { type: Object, default: null },
  level: { type: String, default: 'rtl' },
  // 'wires'  → classic orthogonal trunks (default GOST view)
  // 'labels' → address-method: drop wires, decorate each pin with the net name
  connectionStyle: { type: String, default: 'wires' },
  // Optional override map { netId: "name" } used by labels mode.
  netLabels: { type: Object, default: () => ({}) },
})

function netLabelFor(port) {
  if (!port?.netId) return ''
  const m = props.netLabels?.[port.netId]
  if (m) return m
  return `N${port.netId}`
}

const FONT = "'GOST Type B Italic', 'GOST Type B', Arial, sans-serif"
const BUS_PAD = 10
const WIRE_STUB = 16

const PORT_MARKUP = [{ tagName: 'circle', selector: 'circle' }]
const PORT_ATTRS = {
  circle: {
    r: 0,
    magnet: true,
    fill: 'transparent',
    stroke: 'none',
  },
}
const LINK_MARKUP = [
  { tagName: 'path', selector: 'wrapper', attributes: { fill: 'none', cursor: 'pointer', 'pointer-events': 'stroke' } },
  { tagName: 'path', selector: 'line', attributes: { fill: 'none', 'pointer-events': 'none' } },
]

const cellNamespace = {
  ...joint.shapes,
  gost: {
    Node: joint.dia.Element.define('gost.Node', {
      attrs: { root: { cursor: 'move' } },
    }),

    Terminal: joint.dia.Element.define('gost.Terminal', {
      attrs: { root: { cursor: 'move' } },
    }),

    BusRail: joint.dia.Element.define('gost.BusRail', {
      attrs: { root: { cursor: 'move' } },
    }, {
      markup: [
        { tagName: 'rect', selector: 'hit' },
        { tagName: 'line', selector: 'rail' },
        { tagName: 'line', selector: 'slash' },
        { tagName: 'text', selector: 'widthLabel' },
        { tagName: 'text', selector: 'busLabel' },
      ],
    }),

    Annotation: joint.dia.Element.define('gost.Annotation', {
      attrs: { root: { pointerEvents: 'none' } },
    }),

    WireTree: joint.dia.Element.define('gost.WireTree', {
      attrs: { root: { cursor: 'move' } },
    }),

    WireColumn: joint.dia.Element.define('gost.WireColumn', {
      attrs: { root: { cursor: 'move' } },
    }),
  },
}

const paperHost = ref(null)
const selectedId = ref(null)
const labelEdits = reactive({})
const camera = reactive({ x: 0, y: 0, width: 1000, height: 640 })
const panning = ref(false)
const lastPointer = reactive({ x: 0, y: 0 })

let graph = null
let paper = null
let resizeObserver = null
let activeLinkView = null
let activeElementToolsView = null
let building = false

const selectedNode = computed(() => {
  if (!selectedId.value) return null
  const node = props.diagram?.nodes?.find((item) => item.id === selectedId.value)
  if (!node) return null
  return {
    ...node,
    label: labelEdits[node.id] ?? node.label,
  }
})

watch(
  () => props.diagram,
  (newDiagram) => {
    if (!newDiagram) {
      graph?.clear()
      selectedId.value = null
      return
    }
    for (const key of Object.keys(labelEdits)) delete labelEdits[key]
    selectedId.value = null
    fitToBounds()
    buildGraph()
  },
  { immediate: true },
)

// Switching the connection style (wires ↔ labels) rebuilds the same diagram
// with or without the link cells. Layout/coordinates stay identical so zoom &
// pan continue to work.
watch(
  () => props.connectionStyle,
  () => {
    if (graph && props.diagram) buildGraph()
  },
)

onMounted(() => {
  graph = new joint.dia.Graph({}, { cellNamespace })
  paper = new joint.dia.Paper({
    el: paperHost.value,
    model: graph,
    cellViewNamespace: cellNamespace,
    width: 1000,
    height: 640,
    gridSize: 1,
    drawGrid: false,
    background: { color: '#ffffff' },
    sorting: joint.dia.Paper.sorting.APPROX,

    interactive(cellView) {
      const type = cellView.model.get('type')
      if (type === 'gost.Annotation') return false
      if (cellView.model instanceof joint.dia.Link) {
        // Direct vertex/arrowhead manipulation is disabled — all wire editing
        // goes through the Segments link tool (perpendicular drags only).
        // linkMove MUST be false too: otherwise grabbing the wire body lets
        // the user translate the whole link with the cursor, which shifts
        // every vertex by (tx,ty) while source/target stay pinned to their
        // ports → the first/last segments turn diagonal.
        return {
          vertexAdd: false,
          vertexMove: false,
          vertexRemove: false,
          arrowheadMove: false,
          labelMove: false,
          linkMove: false,
          useLinkTools: true,
        }
      }
      return { elementMove: true }
    },
  })

  // Dismiss link tools the moment the user starts touching any non-annotation
  // element (whether a click or the start of a drag) so the tool handles never
  // race with element-driven recompute logic.
  paper.on('element:pointerdown', (view) => {
    const type = view.model.get('type')
    if (type === 'gost.Annotation') return
    clearLinkTools()
    if (type !== 'gost.WireColumn') clearElementTools()
  })

  paper.on('element:pointerclick', (view) => {
    const type = view.model.get('type')
    if (type === 'gost.WireColumn') {
      clearLinkTools()
      selectNode(null)
      showWireColumnTool(view)
      return
    }
    if (type !== 'gost.Node' && type !== 'gost.Terminal') return
    clearLinkTools()
    clearElementTools()
    selectNode(view.model.id)
  })

  paper.on('blank:pointerclick', () => {
    clearLinkTools()
    clearElementTools()
    selectNode(null)
  })

  paper.on('link:pointerclick', (view) => {
    selectNode(null)
    clearElementTools()
    showLinkTools(view)
  })

  paper.on('element:pointerup', (view) => {
    const type = view.model.get('type')
    if (type === 'gost.Annotation') return

    const cell = view.model
    graph.getConnectedLinks(cell).forEach((link) => {
      if (link.get('isTapLink')) recomputeTapVertex(link)
      else if (link.get('isWireTreeStub')) recomputeWireTreeStubRoute(link)
      else if (link.get('isWireLink')) recomputeWireRoute(link)
    })

    setTimeout(() => updateJunctionDots(), 0)
  })

  paper.on('link:pointerup', () => {
    // captureLinkBaseline runs from VerticalDragLockedSegments.onHandleChangeEnd
    // only after a real segment drag; a bare click must NOT overwrite the
    // baseline, otherwise post-recompute vertices get frozen as the baseline
    // and the next element move breaks.
    setTimeout(() => updateJunctionDots(), 0)
  })

  graph.on('change:position', (cell, newPos, opt) => {
    if (building) return
    const type = cell.get('type')
    if (type !== 'gost.BusRail' && type !== 'gost.Node' && type !== 'gost.Terminal' && type !== 'gost.WireTree' && type !== 'gost.WireColumn') return

    // When a bus rail moves, also translate every WireColumn that's anchored
    // to it. Otherwise the oblique link (column-top → bus-tap port) only sees
    // its target move, while the source stays put, so it stretches at 45°.
    if (type === 'gost.BusRail' && !opt?.fromBusMove) {
      const prev = cell.previous('position') || { x: 0, y: 0 }
      const dx = (newPos?.x ?? cell.position().x) - prev.x
      const dy = (newPos?.y ?? cell.position().y) - prev.y
      if (dx || dy) {
        for (const c of graph.getCells()) {
          if (c.get('type') === 'gost.WireColumn' && c.get('_busRailId') === cell.id) {
            c.translate(dx, dy, { fromBusMove: true })
          }
        }
      }
    }

    graph.getConnectedLinks(cell).forEach((link) => {
      if (link.get('isTapLink')) recomputeTapVertex(link)
      else if (link.get('isWireTreeStub')) recomputeWireTreeStubRoute(link)
      else if (link.get('isWireLink')) recomputeWireRoute(link)
    })
  })

  // Наблюдаем за РОДИТЕЛЕМ paper-host. JointJS Paper выставляет inline
  // width/height на сам paper-host (через paper.setDimensions), и эти
  // inline-стили перебивают наш CSS `position:absolute; inset:18px`.
  // Размер же контейнера (.gost-svg-editor) меняется свободно — слежение
  // за ним даёт нам корректный resize при перетаскивании сплиттера.
  resizeObserver = new ResizeObserver(() => {
    updatePaperSize()
    applyCamera()
  })
  const observeTarget = paperHost.value.parentElement || paperHost.value
  resizeObserver.observe(observeTarget)

  updatePaperSize()
  fitToBounds()
  if (props.diagram) buildGraph()
})

onBeforeUnmount(() => {
  resizeObserver?.disconnect()
  paper?.remove()
  graph?.clear()
})

function clamp(value, min, max) {
  return Math.max(min, Math.min(max, value))
}

function pointKey(point) {
  return `${Math.round(point.x)},${Math.round(point.y)}`
}

function portId(nodeId, port) {
  return `${nodeId}__${port.name}__${port.netId}`
}

function segLen(seg) {
  return Math.abs(seg.x2 - seg.x1) + Math.abs(seg.y2 - seg.y1)
}

function portGroups() {
  return {
    in: {
      position: { name: 'absolute' },
      markup: PORT_MARKUP,
      attrs: PORT_ATTRS,
    },
    out: {
      position: { name: 'absolute' },
      markup: PORT_MARKUP,
      attrs: PORT_ATTRS,
    },
  }
}

function busPortGroups() {
  return {
    endpoint: {
      position: { name: 'absolute' },
      markup: PORT_MARKUP,
      attrs: PORT_ATTRS,
    },
    tap: {
      position: { name: 'absolute' },
      markup: PORT_MARKUP,
      attrs: PORT_ATTRS,
    },
  }
}

function nodeLabel(node) {
  return labelEdits[node.id] ?? node.label
}

function isTerminal(node) {
  return node.kind === 'input' || node.kind === 'output'
}

function firstPortY(node) {
  const ys = (node.ports || []).map((port) => port.y)
  if (!ys.length) return node.y + node.height
  return Math.min(...ys)
}

function headerHeight(node) {
  const byBlock = clamp(node.height * 0.24, 8, 15)
  const beforeFirstPort = Math.max(7, firstPortY(node) - node.y - 6)
  return Math.min(byBlock, beforeFirstPort)
}

function functionFontSize(node) {
  return clamp(headerHeight(node) * 0.9, 6.8, 9.8)
}

function shouldShowNodeLabel(node) {
  if (node.height < 52) return false
  if (node.kind !== 'logic') return Boolean(nodeLabel(node))
  const label = String(node.label || '').toUpperCase()
  return label !== 'AND'
    && label !== 'NAND'
    && label !== 'OR'
    && label !== 'NOR'
    && label !== 'XOR'
    && label !== 'XNOR'
    && label !== 'NOT'
    && label !== 'BUF'
}

function nodeLabelSize(node) {
  const text = String(nodeLabel(node) || '')
  if (!text) return 10
  return clamp(((node.width - 10) / text.length) * 1.35, 5.8, 8.4)
}

function nodeLabelY(node) {
  const head = headerHeight(node)
  const minY = node.y + head + nodeLabelSize(node) + 5
  const centerY = node.y + head + (node.height - head) * 0.56
  return Math.max(minY, centerY)
}

function portFontSize(node) {
  const count = node.ports?.length || 0
  if (count >= 8) return 6.2
  if (count >= 5) return 6.4
  return 6.7
}

function shortPortName(name) {
  if (!name) return ''
  if (name.length <= 8) return name
  return `${name.slice(0, 7)}.`
}

function shouldShowPortName(node, port) {
  if (isTerminal(node)) return false
  return Boolean(port.name)
}

function portTextX(node, port) {
  // Port name sits just inside the body. Only ports with an IN-BODY indicator
  // (the dynamic triangle on C1) need extra padding; the inverted bubble for
  // R / E sits OUTSIDE the body and doesn't overlap the text — so R/D/E keep
  // their natural flush-with-edge position.
  const inset = bodyInsetX(node)
  const east = (port.x - node.x) >= node.width / 2
  const triPad = port.dynamic ? PORT_TRI_LEN + 2 : 0
  return east ? node.width - inset - 3 - triPad : inset + 3 + triPad
}

// Short lead from body edge to actual pin position. For inverted outputs the
// lead starts at the bubble's outer edge instead. Inverted inputs (active-low
// reset, enable) have a similar bubble — the lead skips past it too.
function addPortLeads(node, markup, attrs) {
  const inset = bodyInsetX(node)
  const anyInputBubble = (node.ports || []).some(
    (p) => p.direction !== 'out' && p.inverted,
  )
  // Without inset and without any port bubble (input or output) there is
  // nothing to lead from — the pin already sits on the body edge.
  if (inset === 0 && !node.inverted && !anyInputBubble) return

  for (const [index, port] of (node.ports || []).entries()) {
    const relX = port.x - node.x
    const relY = port.y - node.y
    const east = relX >= node.width / 2
    const sign = east ? +1 : -1
    const bodyEdgeRel = sign > 0 ? node.width - inset : inset
    const hasOutputBubble = node.inverted && port.direction === 'out'
    const hasInputBubble = port.direction !== 'out' && port.inverted

    // For blocks flush with bounds (inset=0) the pin sits on the body edge,
    // so an input-bubble would force the lead through the circle. In that
    // case skip the lead entirely — the wire visually crosses the bubble,
    // which is the canonical ГОСТ indication.
    if (hasInputBubble && inset === 0) continue

    // Start of lead — past the bubble (either input or output) if present.
    let startX = bodyEdgeRel
    if (hasOutputBubble || hasInputBubble) {
      startX = bodyEdgeRel + sign * 2 * BUBBLE_R
    }
    if (Math.abs(startX - relX) < 0.5) continue

    const sel = `lead${index}`
    markup.push({ tagName: 'line', selector: sel })
    attrs[sel] = {
      x1: startX,
      y1: relY,
      x2: relX,
      y2: relY,
      stroke: '#000000',
      strokeWidth: LEAD_STROKE,
      strokeLinecap: hasOutputBubble || hasInputBubble ? 'round' : 'butt',
      vectorEffect: 'non-scaling-stroke',
    }
  }
}

// Static-input bubble centre — ВСЕГДА снаружи УГО (ГОСТ 2.743-91 табл. 3 п. 3).
// The bubble sits in the [body-edge, body-edge + 2r] strip on the wire side.
// When the block is flush with bounds (inset=0) the bubble extends past the
// bounds rect; the wire visually passes through it — that's the canonical
// "circle on the wire" indication of an inverted static input.
function portBubbleCx(node, port) {
  const inset = bodyInsetX(node)
  const east = (port.x - node.x) >= node.width / 2
  const sign = east ? +1 : -1
  const bodyEdgeRel = sign > 0 ? node.width - inset : inset
  return bodyEdgeRel + sign * BUBBLE_R
}

// Triangle size — small enough to clearly fit inside the УГО and not collide
// with port name text.
const PORT_TRI_LEN = 4
const PORT_TRI_HALF = 2.4

// ГОСТ 2.743-91 table 3 indicators:
//   port.dynamic  → triangle INSIDE the УГО (динамический указатель)
//   port.inverted → small circle OUTSIDE the УГО (инверсный статический)
//   both          → "инверсный динамический" (negedge clock)
function addPortIndicators(node, markup, attrs) {
  const inset = bodyInsetX(node)
  for (const [index, port] of (node.ports || []).entries()) {
    if (!port.dynamic && !port.inverted) continue
    if (port.direction === 'out') continue // outputs use node.inverted bubble
    const relX = port.x - node.x
    const relY = port.y - node.y
    const east = relX >= node.width / 2
    const sign = east ? +1 : -1
    const bodyEdgeRel = sign > 0 ? node.width - inset : inset

    if (port.inverted) {
      const sel = `pinBubble${index}`
      markup.push({ tagName: 'circle', selector: sel })
      attrs[sel] = {
        cx: portBubbleCx(node, port),
        cy: relY,
        r: BUBBLE_R,
        fill: '#ffffff',
        stroke: '#000000',
        strokeWidth: BUBBLE_STROKE,
        vectorEffect: 'non-scaling-stroke',
      }
    }

    if (port.dynamic) {
      // Compact triangle pointing INWARD (into the УГО), apex deepest, base
      // on the body edge. Kept small (PORT_TRI_LEN×2·PORT_TRI_HALF) so it
      // never collides with the port-name text shown beside the pin.
      const sel = `pinTri${index}`
      const baseX = bodyEdgeRel
      const tipX = bodyEdgeRel - sign * PORT_TRI_LEN
      markup.push({ tagName: 'polygon', selector: sel })
      attrs[sel] = {
        points: `${baseX},${relY - PORT_TRI_HALF} ${baseX},${relY + PORT_TRI_HALF} ${tipX},${relY}`,
        fill: '#ffffff',
        stroke: '#000000',
        strokeWidth: BUBBLE_STROKE,
        strokeLinejoin: 'miter',
        vectorEffect: 'non-scaling-stroke',
      }
    }
  }
}

function portAnchor(port) {
  return port.direction === 'in' ? 'start' : 'end'
}

function outputPorts(node) {
  return (node.ports || []).filter((port) => port.direction === 'out')
}

function bubbleX(port) {
  return port.direction === 'out' ? port.x + 2.6 : port.x - 2.6
}

function textAttrs(x, y, size, anchor = 'middle') {
  return {
    x,
    y,
    textAnchor: anchor,
    textVerticalAnchor: 'middle',
    dominantBaseline: 'middle',
    fontFamily: FONT,
    fontStyle: 'italic',
    fontSize: size,
    fill: '#000000',
    pointerEvents: 'none',
  }
}

function portItems(node) {
  return (node.ports || []).map((port) => ({
    id: portId(node.id, port),
    group: port.direction,
    args: {
      x: port.x - node.x,
      y: port.y - node.y,
    },
  }))
}

// In labels-mode each pin grows a tiny arrow stub and a net-name text. Pins
// on the east side of the block point east; pins on the west side point west.
// All these decorations live inside the Node element's markup so they move
// with the node when the user drags it.
const NET_LABEL_STUB = 18
const NET_LABEL_ARROW = 5
const NET_LABEL_PAD = 3

function decorateWithNetLabels(node, markup, attrs) {
  for (const [index, port] of (node.ports || []).entries()) {
    const label = netLabelFor(port)
    if (!label) continue
    const relPortX = port.x - node.x
    const relPortY = port.y - node.y
    const east = relPortX >= node.width / 2
    const sign = east ? +1 : -1
    const isOut = port.direction === 'out'

    // Net-stub starts past any port-side bubble so the line doesn't cut
    // through the inversion circle. For inverted outputs the bubble belongs
    // to node.inverted; for inverted inputs it belongs to port.inverted.
    const hasOutputBubble = node.inverted && isOut
    const hasInputBubble = !isOut && port.inverted
    let stubStartX = relPortX
    if (hasOutputBubble || hasInputBubble) {
      // Start the stub at the bubble's WIRE-SIDE outer edge.
      const bubbleCx =
        sign > 0 ? relPortX + BUBBLE_R : relPortX - BUBBLE_R
      stubStartX = bubbleCx + sign * BUBBLE_R
    }
    const stubEnd = relPortX + sign * NET_LABEL_STUB

    // Stub line — same stroke width as body/leads for visual consistency.
    const stubSel = `netStub${index}`
    markup.push({ tagName: 'line', selector: stubSel })
    attrs[stubSel] = {
      x1: stubStartX,
      y1: relPortY,
      x2: stubEnd,
      y2: relPortY,
      stroke: '#000000',
      strokeWidth: LEAD_STROKE,
      vectorEffect: 'non-scaling-stroke',
    }

    // ГОСТ 2.708-81 п. 2.1.5 запрещает стрелки у контура УГО в стандартной
    // ориентации. В адресном методе оставляем только короткий обрыв линии
    // и метку имени сети — без стрелочного указателя.
    void isOut

    // Net name text — vertically centred on the stub line via
    // dominantBaseline='middle'.
    const txtSel = `netLabel${index}`
    markup.push({ tagName: 'text', selector: txtSel })
    attrs[txtSel] = {
      x: stubEnd + sign * NET_LABEL_PAD,
      y: relPortY,
      textAnchor: east ? 'start' : 'end',
      textVerticalAnchor: 'middle',
      dominantBaseline: 'middle',
      fontFamily: FONT,
      fontStyle: 'italic',
      fontSize: 9,
      fill: '#000000',
      text: label,
    }
  }
}

// Simple gates (AND/OR/XOR/NOT family by function symbol) get an inset body
// so the УГО rectangle reads as a compact gate symbol rather than filling the
// whole grid cell. Pins stay on the original grid positions (outside the
// body) and are joined to the body edge by a short lead line. Registers and
// chips keep their original size.
const SIMPLE_GATE_FUNCS = new Set(['1', '&', '>=1', '=1', 'BUF'])
const BODY_INSET_X = 8
// Bubble (ГОСТ "указатель инверсного вывода" — диаметр ≈ 1M = 8px → r=2.6),
// triangle (ГОСТ "указатель динамического вывода" — 1M × 0.5M).
const BUBBLE_R = 2.6
const BUBBLE_STROKE = 2.0
const BODY_STROKE = 2.0
const HEADER_STROKE = 2.0
const LEAD_STROKE = 2.0

function isSimpleGate(node) {
  return !isTerminal(node) && SIMPLE_GATE_FUNCS.has(node.function)
}

function bodyInsetX(node) {
  return isSimpleGate(node) ? BODY_INSET_X : 0
}

function createNodeElement(node) {
  const head = headerHeight(node)
  const inset = bodyInsetX(node)
  const bodyW = node.width - inset * 2

  const markup = [
    { tagName: 'rect', selector: 'body' },
    { tagName: 'line', selector: 'headerLine' },
    { tagName: 'text', selector: 'functionText' },
  ]
  const attrs = {
    body: {
      x: inset,
      y: 0,
      width: bodyW,
      height: node.height,
      fill: '#ffffff',
      stroke: '#000000',
      strokeWidth: BODY_STROKE,
      vectorEffect: 'non-scaling-stroke',
    },
    headerLine: {
      x1: inset,
      y1: head,
      x2: inset + bodyW,
      y2: head,
      stroke: '#000000',
      strokeWidth: HEADER_STROKE,
      vectorEffect: 'non-scaling-stroke',
    },
    functionText: {
      // Lift the function symbol clearly above the headerLine. ~38% of the
      // header height puts the visual centre well inside the upper half,
      // leaving room between the descenders and the divider.
      ...textAttrs(inset + bodyW / 2, head * 0.38, functionFontSize(node)),
      text: node.function,
      fontWeight: 700,
    },
  }

  if (shouldShowNodeLabel(node)) {
    markup.push({ tagName: 'text', selector: 'labelText' })
    attrs.labelText = {
      ...textAttrs(inset + bodyW / 2, nodeLabelY(node) - node.y, nodeLabelSize(node)),
      text: nodeLabel(node),
    }
  }

  for (const [index, port] of (node.ports || []).entries()) {
    if (!shouldShowPortName(node, port)) continue
    const selector = `portText${index}`
    markup.push({ tagName: 'text', selector })
    attrs[selector] = {
      ...textAttrs(portTextX(node, port), port.y - node.y, portFontSize(node), portAnchor(port)),
      text: shortPortName(port.name),
    }
  }

  // Inversion bubbles — repositioned to sit at the body edge (not bounds
  // edge) so they hug the УГО for inset (simple-gate) blocks too.
  for (const [index, port] of (node.inverted ? outputPorts(node) : []).entries()) {
    const selector = `bubble${index}`
    const east = (port.x - node.x) >= node.width / 2
    const sign = east ? +1 : -1
    const bodyEdgeRel = sign > 0 ? node.width - inset : inset
    markup.push({ tagName: 'circle', selector })
    attrs[selector] = {
      cx: bodyEdgeRel + sign * BUBBLE_R,
      cy: port.y - node.y,
      r: BUBBLE_R,
      fill: '#ffffff',
      stroke: '#000000',
      strokeWidth: BUBBLE_STROKE,
      vectorEffect: 'non-scaling-stroke',
    }
  }

  // ГОСТ 2.743-91 indicators per port (clock triangle, input bubble).
  addPortIndicators(node, markup, attrs)

  // Lead lines: short stub from the body edge (or bubble outer edge for
  // inverted outputs/inputs) to the actual pin coordinate.
  addPortLeads(node, markup, attrs)

  // Labels-mode decoration: a small arrow + net-name next to every pin,
  // pointing outward. Co-located with the node so it follows on drag/zoom.
  if (props.connectionStyle === 'labels') {
    decorateWithNetLabels(node, markup, attrs)
  }

  return new cellNamespace.gost.Node({
    id: node.id,
    position: { x: node.x, y: node.y },
    size: { width: node.width, height: node.height },
    markup,
    attrs,
    ports: {
      groups: portGroups(),
      items: portItems(node),
    },
    z: 40,
  })
}

function terminalPort(node) {
  return node.ports?.[0] || { x: node.x, y: node.y + node.height / 2, direction: node.kind === 'input' ? 'out' : 'in' }
}

function createTerminalElement(node) {
  const port = terminalPort(node)
  const lineY = port.y - node.y
  const labelX = node.kind === 'input' ? 0 : node.width
  const anchor = node.kind === 'input' ? 'start' : 'end'
  const labels = props.connectionStyle === 'labels'
  const arrowLen = 6

  // In labels mode the line stops at the BASE of the arrow head (not at
  // the outer end / label position). Otherwise the square line cap would
  // peek past the arrow triangle. Wires mode keeps the original full line.
  const pinX = port.x - node.x
  let lineX1 = node.kind === 'input' ? 0 : pinX
  let lineX2 = node.kind === 'input' ? pinX : node.width
  if (labels) {
    if (node.kind === 'input') lineX1 = arrowLen
    else lineX2 = node.width - arrowLen
  }

  const markup = [
    { tagName: 'rect', selector: 'hit' },
    { tagName: 'line', selector: 'line' },
    { tagName: 'text', selector: 'label' },
  ]
  const attrs = {
    hit: {
      x: -8,
      y: lineY - 18,
      width: node.width + 16,
      height: 36,
      fill: '#ffffff',
      fillOpacity: 0,
      stroke: 'none',
    },
    line: {
      x1: lineX1,
      y1: lineY,
      x2: lineX2,
      y2: lineY,
      stroke: '#000000',
      strokeWidth: 2,
      strokeLinecap: labels ? 'butt' : 'square',
      vectorEffect: 'non-scaling-stroke',
    },
    label: {
      // Unified style — same font size and weight in both wires and labels
      // modes. textAttrs centres the text vertically (dominantBaseline=middle)
      // and Y = lineY - 9 lifts it just above the terminal line so the text
      // never collides with the line or arrow head.
      ...textAttrs(labelX, lineY - 9, 11, anchor),
      text: nodeLabel(node),
      fontWeight: 700,
    },
  }

  if (labels) {
    const tipX = labelX
    const baseX = node.kind === 'input' ? arrowLen : node.width - arrowLen
    markup.push({ tagName: 'polygon', selector: 'arrow' })
    attrs.arrow = {
      points: `${tipX},${lineY} ${baseX},${lineY - 3} ${baseX},${lineY + 3}`,
      fill: '#000000',
    }
  }

  return new cellNamespace.gost.Terminal({
    id: node.id,
    position: { x: node.x, y: node.y },
    size: { width: node.width, height: node.height },
    markup,
    attrs,
    ports: {
      groups: portGroups(),
      items: portItems(node),
    },
    z: 40,
  })
}

function busRailGeometry(wire) {
  const trunk = (wire.segments || []).reduce((a, b) => (segLen(a) >= segLen(b) ? a : b))
  const horiz = trunk.y1 === trunk.y2
  return {
    horiz,
    x: Math.min(trunk.x1, trunk.x2),
    y: Math.min(trunk.y1, trunk.y2),
    len: segLen(trunk),
  }
}

// The server emits each tap as a polyline whose final segment is the short
// oblique stub joining the bus trunk: (approach) -> (busPt on trunk). The
// approach point is where the thin orthogonal wire ends and the oblique begins.
function tapGeom(tap) {
  const segs = tap.segments || []
  const last = segs[segs.length - 1] || { x1: 0, y1: 0, x2: 0, y2: 0 }
  return {
    approach: { x: last.x1, y: last.y1 },
    busPt: { x: last.x2, y: last.y2 },
  }
}

function tapPortId(tap, index = 0) {
  return `${tap.netId}__tap__${index}`
}

// Pre-pass over a bus's taps: group taps that share the same net AND the same
// vertical climb column (so their pin-to-bus polylines would otherwise overlap
// on the climb segment). For each multi-member group, the renderer draws the
// climb ONCE inside the BusRail markup and turns each pin's tap-link into a
// short horizontal stub from the pin to a branch port on that shared column.
function findClimbInfo(tap) {
  const segs = tap.segments || []
  if (segs.length < 3) return null
  const climb = segs[1]
  if (Math.abs(climb.x1 - climb.x2) > 0.5) return null
  return {
    climbX: climb.x1,
    topY: Math.min(climb.y1, climb.y2),
    bottomY: Math.max(climb.y1, climb.y2),
  }
}

function findMergedTapGroups(wire) {
  const groups = new Map()
  for (let i = 0; i < (wire.taps || []).length; i += 1) {
    const tap = wire.taps[i]
    const climb = findClimbInfo(tap)
    if (!climb) continue
    const key = `${tap.netId}|${Math.round(climb.climbX)}|${Math.round(climb.topY)}`
    if (!groups.has(key)) {
      groups.set(key, {
        netId: tap.netId,
        climbX: climb.climbX,
        topY: climb.topY,
        members: [],
      })
    }
    groups.get(key).members.push({
      tapIndex: i,
      branchY: climb.bottomY,
    })
  }
  return [...groups.values()].filter((g) => g.members.length >= 2)
}

function buildMergedTapIndex(wire) {
  const map = new Map()
  const groups = findMergedTapGroups(wire)
  for (const group of groups) {
    const sorted = [...group.members].sort((a, b) => a.branchY - b.branchY)
    const primaryTapIndex = sorted[0].tapIndex
    const bottomBranchY = sorted[sorted.length - 1].branchY
    for (const m of group.members) {
      map.set(m.tapIndex, {
        group,
        climbX: group.climbX,
        topY: group.topY,
        branchY: m.branchY,
        members: sorted,
        bottomBranchY,
        primaryTapIndex,
        isPrimary: m.tapIndex === primaryTapIndex,
      })
    }
  }
  return map
}

function wireColumnPortGroups() {
  return {
    top: {
      position: { name: 'absolute' },
      markup: PORT_MARKUP,
      attrs: PORT_ATTRS,
    },
    branch: {
      position: { name: 'absolute' },
      markup: PORT_MARKUP,
      attrs: PORT_ATTRS,
    },
  }
}

// Build a standalone vertical-column element for a merged-tap group. It owns
// the shared climb segment, the per-branch junction dots, and the ports the
// stub-links and oblique-link connect to. Because it's a real element, the
// user can grab a Segments-tool handle on the column line and drag it
// horizontally to reroute the entire bundle of taps.
function createWireColumnElement(wire, group) {
  const sorted = [...group.members].sort((a, b) => a.branchY - b.branchY)
  const bottomBranchY = sorted[sorted.length - 1].branchY
  const topY = group.topY
  const climbX = group.climbX

  const PAD = 6
  const posX = climbX - PAD
  const posY = topY - PAD
  const width = 2 * PAD
  const height = (bottomBranchY - topY) + 2 * PAD

  const markup = [{ tagName: 'rect', selector: 'hit' }]
  const attrs = {
    hit: {
      x: 0, y: 0, width, height,
      fill: '#ffffff', fillOpacity: 0, stroke: 'none',
    },
    column: {
      x1: climbX - posX, y1: topY - posY,
      x2: climbX - posX, y2: bottomBranchY - posY,
      stroke: '#000000', strokeWidth: 1.85, strokeLinecap: 'square',
      vectorEffect: 'non-scaling-stroke',
    },
  }
  markup.push({ tagName: 'line', selector: 'column' })

  for (let j = 0; j < sorted.length; j += 1) {
    const m = sorted[j]
    if (Math.abs(m.branchY - bottomBranchY) < 0.5) continue
    const sel = `dot${j}`
    markup.push({ tagName: 'circle', selector: sel })
    attrs[sel] = {
      cx: climbX - posX, cy: m.branchY - posY, r: 2,
      fill: '#000000', stroke: 'none',
      vectorEffect: 'non-scaling-stroke',
    }
  }

  const ports = [
    { id: 'top', group: 'top', args: { x: climbX - posX, y: topY - posY } },
  ]
  const branchPortByTapIndex = new Map()
  for (const m of group.members) {
    const pid = `branch-${m.tapIndex}`
    branchPortByTapIndex.set(m.tapIndex, pid)
    ports.push({
      id: pid,
      group: 'branch',
      args: { x: climbX - posX, y: m.branchY - posY },
    })
  }

  const element = new cellNamespace.gost.WireColumn({
    id: `${wire.id}__col__${group.netId}__${Math.round(climbX)}`,
    _busRailId: wire.id,
    position: { x: posX, y: posY },
    size: { width, height },
    markup,
    attrs,
    ports: { groups: wireColumnPortGroups(), items: ports },
    z: 10,
  })

  return { element, branchPortByTapIndex, posX, posY }
}

// The oblique stub between a WireColumn's top and its bus pin is a straight
// JointJS link. Source/target are ports on the WireColumn and the BusRail
// respectively, so when either element moves the oblique re-renders for free.
function createObliqueLink(busRailId, busPortId, columnId, columnTopPortId, netId, suffix) {
  return new joint.dia.Link({
    type: 'gost.Link',
    id: `${busRailId}__oblique__${suffix}`,
    netId,
    markup: LINK_MARKUP,
    source: { id: columnId, port: columnTopPortId },
    target: { id: busRailId, port: busPortId },
    router: { name: 'normal' },
    connector: { name: 'straight' },
    attrs: {
      wrapper: wrapperAttrs(),
      line: lineAttrs(1.85),
    },
    z: 18,
  })
}

// Tap-stub fallback path after a Y-move of the pin's element: the default
// tapVertices puts the elbow on (branch.x, pin.y), which means the vertical
// leg sits on the shared column's x — visually merging with it. For merged
// taps we instead Z-route the stub with the vertical leg at the midpoint
// between pin and column, so neither overlaps the column nor the element.
function mergedTapVertices(pin, branch) {
  const dx = branch.x - pin.x
  const dy = branch.y - pin.y
  const alignedX = Math.abs(dx) < 0.5
  const alignedY = Math.abs(dy) < 0.5
  if (alignedX || alignedY) return []
  const room = Math.abs(dx)
  const stub = adaptiveStub(room) * Math.sign(dx)
  const sExit = { x: pin.x + stub, y: pin.y }
  const tEnter = { x: branch.x - stub, y: branch.y }
  const verts = [sExit]
  if (Math.abs(sExit.x - tEnter.x) > 0.1 && Math.abs(sExit.y - tEnter.y) > 0.1) {
    const midX = Math.round((sExit.x + tEnter.x) / 2)
    verts.push({ x: midX, y: pin.y }, { x: midX, y: branch.y })
  }
  verts.push(tEnter)
  return dedupePoints(verts)
}

function createBusRailElement(wire, geometry, mergedTapIndex) {
  // The bus rail owns the trunk line, the GOST identifier slash + width label,
  // and every per-tap oblique stub + bit label. Because those tap decorations
  // are part of this element, they always travel with the bus when it is moved.
  const trunkEndX = geometry.horiz ? geometry.x + geometry.len : geometry.x
  const trunkEndY = geometry.horiz ? geometry.y : geometry.y + geometry.len

  let minX = Math.min(geometry.x, trunkEndX)
  let maxX = Math.max(geometry.x, trunkEndX)
  let minY = Math.min(geometry.y, trunkEndY)
  let maxY = Math.max(geometry.y, trunkEndY)
  for (const tap of wire.taps || []) {
    const { approach, busPt } = tapGeom(tap)
    for (const p of [approach, busPt, { x: tap.labelX, y: tap.labelY }]) {
      minX = Math.min(minX, p.x)
      maxX = Math.max(maxX, p.x)
      minY = Math.min(minY, p.y)
      maxY = Math.max(maxY, p.y)
    }
  }

  const posX = minX - BUS_PAD
  const posY = minY - BUS_PAD
  const width = (maxX - minX) + 2 * BUS_PAD
  const height = (maxY - minY) + 2 * BUS_PAD
  const railStart = { x: geometry.x - posX, y: geometry.y - posY }
  const railEnd = { x: trunkEndX - posX, y: trunkEndY - posY }
  const centerX = (railStart.x + railEnd.x) / 2
  const centerY = (railStart.y + railEnd.y) / 2

  const markup = [
    { tagName: 'rect', selector: 'hit' },
    { tagName: 'line', selector: 'rail' },
    { tagName: 'line', selector: 'slash' },
    { tagName: 'text', selector: 'widthLabel' },
    { tagName: 'text', selector: 'busLabel' },
  ]
  const attrs = {
    hit: {
      x: 0,
      y: 0,
      width,
      height,
      fill: '#ffffff',
      fillOpacity: 0,
      stroke: 'none',
    },
    rail: {
      x1: railStart.x,
      y1: railStart.y,
      x2: railEnd.x,
      y2: railEnd.y,
      stroke: '#000000',
      strokeWidth: 4,
      strokeLinecap: 'square',
      vectorEffect: 'non-scaling-stroke',
    },
    slash: {
      x1: centerX - 8,
      y1: centerY - 8,
      x2: centerX + 8,
      y2: centerY + 8,
      stroke: '#000000',
      strokeWidth: 1.8,
      strokeLinecap: 'square',
      vectorEffect: 'non-scaling-stroke',
    },
    widthLabel: {
      ...textAttrs(centerX - 15, centerY - 12, 9),
      text: String(wire.width),
    },
    busLabel: {
      ...textAttrs(geometry.horiz ? centerX : centerX + 22, geometry.horiz ? centerY - 18 : centerY, 12, geometry.horiz ? 'middle' : 'start'),
      text: wire.label,
      fontWeight: 700,
    },
  }

  const busPorts = [
    { id: 'rail-start', group: 'endpoint', args: { x: railStart.x, y: railStart.y } },
    { id: 'rail-end', group: 'endpoint', args: { x: railEnd.x, y: railEnd.y } },
  ]

  const labelledNets = new Set()
  for (const [i, tap] of (wire.taps || []).entries()) {
    const merged = mergedTapIndex?.get(i)
    const { approach, busPt } = tapGeom(tap)

    if (merged) {
      if (!merged.isPrimary) {
        // Non-primary of a merged group: column, branch dots, oblique, bit
        // label are all rendered by the WireColumn element / oblique link
        // attached to the primary. Nothing to do on BusRail.
        continue
      }
      // Primary of a merged group: keep the bit label here (close to the
      // bus), and add a port at busPt for the oblique link to anchor against.
      // The column line, branch dots, and the oblique itself live elsewhere.
      if (tap.label && !labelledNets.has(tap.netId)) {
        labelledNets.add(tap.netId)
        markup.push({ tagName: 'text', selector: `bitLabel${i}` })
        attrs[`bitLabel${i}`] = {
          ...textAttrs(tap.labelX - posX, tap.labelY - posY, 8),
          text: tap.label,
        }
      }
      busPorts.push({
        id: `busTap-${i}`,
        group: 'tap',
        args: { x: busPt.x - posX, y: busPt.y - posY },
      })
      continue
    }

    // Singleton tap — keep existing GOST oblique + bit label + approach port.
    markup.push({ tagName: 'line', selector: `oblique${i}` })
    attrs[`oblique${i}`] = {
      x1: approach.x - posX,
      y1: approach.y - posY,
      x2: busPt.x - posX,
      y2: busPt.y - posY,
      stroke: '#000000',
      strokeWidth: 1.85,
      strokeLinecap: 'square',
      vectorEffect: 'non-scaling-stroke',
    }
    if (tap.label && !labelledNets.has(tap.netId)) {
      labelledNets.add(tap.netId)
      markup.push({ tagName: 'text', selector: `bitLabel${i}` })
      attrs[`bitLabel${i}`] = {
        ...textAttrs(tap.labelX - posX, tap.labelY - posY, 8),
        text: tap.label,
      }
    }
    busPorts.push({
      id: tapPortId(tap, i),
      group: 'tap',
      args: { x: approach.x - posX, y: approach.y - posY },
    })
  }

  return new cellNamespace.gost.BusRail({
    id: wire.id,
    position: { x: posX, y: posY },
    size: { width, height },
    markup,
    attrs,
    ports: {
      groups: busPortGroups(),
      items: busPorts,
    },
    z: 10,
  })
}

function lineAttrs(strokeWidth) {
  return {
    connection: true,
    stroke: '#000000',
    strokeWidth,
    strokeLinecap: 'square',
    strokeLinejoin: 'miter',
    fill: 'none',
    targetMarker: null,
    sourceMarker: null,
    vectorEffect: 'non-scaling-stroke',
  }
}

function wrapperAttrs() {
  return {
    connection: true,
    stroke: 'transparent',
    strokeWidth: 12,
    strokeLinecap: 'square',
    strokeLinejoin: 'miter',
    fill: 'none',
    cursor: 'pointer',
  }
}

function endpoint(ref) {
  return { id: ref.elementId, port: ref.portId }
}

function routerDirectionForRef(ref, role) {
  if (role === 'source') return ref.direction === 'out' ? ['right'] : ['left']
  return ref.direction === 'in' ? ['left'] : ['right']
}

function filterBends(pts) {
  if (pts.length <= 1) return pts
  const out = []
  for (let i = 0; i < pts.length; i += 1) {
    const prev = i === 0 ? null : pts[i - 1]
    const next = i === pts.length - 1 ? null : pts[i + 1]
    if (!prev || !next) {
      out.push(pts[i])
      continue
    }
    const sameDirX = prev.x === pts[i].x && pts[i].x === next.x
    const sameDirY = prev.y === pts[i].y && pts[i].y === next.y
    if (!sameDirX && !sameDirY) out.push(pts[i])
  }
  return out
}

function segmentsToPath(segments, srcPt, tgtPt) {
  const EPS = 3
  const key = (point) => `${Math.round(point.x)},${Math.round(point.y)}`
  const close = (a, b) => Math.abs(a.x - b.x) <= EPS && Math.abs(a.y - b.y) <= EPS
  const adj = new Map()

  const addEdge = (a, b) => {
    const ka = key(a)
    if (!adj.has(ka)) adj.set(ka, [])
    adj.get(ka).push(b)
  }

  for (const seg of segments || []) {
    const a = { x: seg.x1, y: seg.y1 }
    const b = { x: seg.x2, y: seg.y2 }
    addEdge(a, b)
    addEdge(b, a)
  }

  const visited = new Set([key(srcPt)])
  const queue = [{ cur: srcPt, path: [srcPt] }]

  while (queue.length) {
    const { cur, path } = queue.shift()
    if (close(cur, tgtPt)) return filterBends(path.slice(1, -1))

    for (const next of adj.get(key(cur)) ?? []) {
      const nk = key(next)
      if (visited.has(nk)) continue
      visited.add(nk)
      queue.push({ cur: next, path: [...path, next] })
    }
  }

  return []
}

function createWireLink(wire, driver, sink, nodesById) {
  const srcPt = { x: driver.port.x, y: driver.port.y }
  const tgtPt = { x: sink.port.x, y: sink.port.y }
  const vertices = segmentsToPath(wire.segments || [], srcPt, tgtPt)
  const hasRoute = vertices.length > 0

  const link = new joint.dia.Link({
    type: 'gost.Link',
    id: `${wire.id}__${sink.portId}`,
    netId: wire.netIds[0],
    markup: LINK_MARKUP,
    source: endpoint(driver),
    target: endpoint(sink),
    // The server route is already a clean orthogonal polyline; draw it verbatim
    // with the 'normal' router. The 'orthogonal' router re-interprets vertices
    // against element bounding boxes and inserts detour loops when a vertex sits
    // close to an element.
    router: hasRoute
      ? { name: 'normal' }
      : {
          name: 'manhattan',
          args: {
            step: 8,
            padding: 28,
            startDirections: routerDirectionForRef(driver, 'source'),
            endDirections: routerDirectionForRef(sink, 'target'),
          },
        },
    connector: { name: 'straight' },
    attrs: {
      wrapper: wrapperAttrs(),
      line: lineAttrs(2),
    },
    z: 20,
  })

  if (hasRoute) link.vertices(vertices)

  const srcNode = nodesById.get(driver.elementId)
  const tgtNode = nodesById.get(sink.elementId)
  link.set('isWireLink', true)
  link.set('srcRelX', srcNode ? srcPt.x - srcNode.x : 0)
  link.set('srcRelY', srcNode ? srcPt.y - srcNode.y : 0)
  link.set('tgtRelX', tgtNode ? tgtPt.x - tgtNode.x : 0)
  link.set('tgtRelY', tgtNode ? tgtPt.y - tgtNode.y : 0)
  link.set('serverVertices', hasRoute ? vertices.map((vertex) => ({ x: vertex.x, y: vertex.y })) : [])
  link.set('srcOrigPortX', srcPt.x)
  link.set('srcOrigPortY', srcPt.y)
  link.set('tgtOrigPortX', tgtPt.x)
  link.set('tgtOrigPortY', tgtPt.y)

  return link
}

// A tap link is the thin orthogonal wire from a logic pin to the bus tap's
// approach port (the inner end of the oblique stub, which the bus rail draws).
// Shape is a clean L: horizontal from the pin, then vertical into the approach
// port. The oblique itself is never part of this link, so it cannot stretch.
function tapVertices(pin, approach) {
  const alignedX = Math.abs(pin.x - approach.x) < 0.5
  const alignedY = Math.abs(pin.y - approach.y) < 0.5
  if (alignedX || alignedY) return []
  return [{ x: approach.x, y: pin.y }]
}

function createTapLink(wire, tap, ref, tapIndex, nodesById, railPos, mergedTarget) {
  // For merged-tap groups mergedTarget = { cellId, portId, point, posX, posY }
  // points to the branch port on a WireColumn (pin and branch share Y, so the
  // initial polyline is a single horizontal stub; mergedTapVertices handles
  // the Z-route after a Y-move). For singletons the link targets the BusRail's
  // approach port and tapVertices renders the standard GOST elbow.
  const pin = { x: ref.port.x, y: ref.port.y }
  const tgt = mergedTarget?.point || tapGeom(tap).approach
  const isMerged = Boolean(mergedTarget)
  const targetCellId = isMerged ? mergedTarget.cellId : wire.id
  const targetPortId = isMerged ? mergedTarget.portId : tapPortId(tap, tapIndex)
  const tgtAnchorPosX = isMerged ? mergedTarget.posX : railPos.x
  const tgtAnchorPosY = isMerged ? mergedTarget.posY : railPos.y
  const initialVerts = isMerged
    ? mergedTapVertices(pin, tgt)
    : tapVertices(pin, tgt)

  const link = new joint.dia.Link({
    type: 'gost.Link',
    id: `${wire.id}__tap__${tapIndex}__${tap.netId}__${ref.portId}`,
    netId: tap.netId,
    markup: LINK_MARKUP,
    source: endpoint(ref),
    target: { id: targetCellId, port: targetPortId },
    router: { name: 'normal' },
    connector: { name: 'straight' },
    vertices: initialVerts,
    attrs: {
      wrapper: wrapperAttrs(),
      line: lineAttrs(1.85),
    },
    z: 20,
  })

  const srcNode = nodesById.get(ref.elementId)
  link.set('isTapLink', true)
  if (isMerged) link.set('_mergedTap', true)
  link.set('srcRelX', srcNode ? pin.x - srcNode.x : 0)
  link.set('srcRelY', srcNode ? pin.y - srcNode.y : 0)
  link.set('tgtRelX', tgt.x - tgtAnchorPosX)
  link.set('tgtRelY', tgt.y - tgtAnchorPosY)

  return link
}

function recomputeTapVertex(link) {
  if (!graph) return
  const srcCell = graph.getCell(link.get('source')?.id)
  const tgtCell = graph.getCell(link.get('target')?.id)
  if (!srcCell || !tgtCell) return

  const pin = {
    x: srcCell.position().x + link.get('srcRelX'),
    y: srcCell.position().y + link.get('srcRelY'),
  }
  const approach = {
    x: tgtCell.position().x + link.get('tgtRelX'),
    y: tgtCell.position().y + link.get('tgtRelY'),
  }

  // For merged taps the default elbow at (column.x, pin.y) would pin the
  // vertical leg to the shared column's axis (visually merging with it).
  // Use a Z-route via mergedTapVertices instead.
  if (link.get('_mergedTap') && !link.get('_userEdited')) {
    link.vertices(mergedTapVertices(pin, approach))
    return
  }

  // If the user has manually re-routed this tap, slide their vertices instead
  // of regenerating the default L-shape from scratch.
  if (link.get('_userEdited')) {
    const userVerts = link.get('serverVertices') || []
    if (userVerts.length >= 2) {
      const origSrcX = link.get('srcOrigPortX') ?? pin.x
      const origSrcY = link.get('srcOrigPortY') ?? pin.y
      const origTgtX = link.get('tgtOrigPortX') ?? approach.x
      const origTgtY = link.get('tgtOrigPortY') ?? approach.y
      const last = userVerts.length - 1
      const anchors = userVerts.map((vertex, i) => {
        if (i === 0) return slideEndAnchor(vertex, origSrcX, origSrcY, pin.x, pin.y)
        if (i === last) return slideEndAnchor(vertex, origTgtX, origTgtY, approach.x, approach.y)
        return { x: vertex.x, y: vertex.y }
      })
      link.vertices(dedupePoints(orthogonalize(anchors)))
      return
    }
    if (userVerts.length === 1) {
      // Single elbow: keep it on whichever axis it originally aligned with.
      const v = userVerts[0]
      const origSrcX = link.get('srcOrigPortX') ?? pin.x
      const origSrcY = link.get('srcOrigPortY') ?? pin.y
      const origTgtX = link.get('tgtOrigPortX') ?? approach.x
      const origTgtY = link.get('tgtOrigPortY') ?? approach.y
      let nv = { x: v.x, y: v.y }
      if (Math.abs(v.x - origTgtX) < 0.5 && Math.abs(v.y - origSrcY) < 0.5) {
        nv = { x: approach.x, y: pin.y }
      } else if (Math.abs(v.x - origSrcX) < 0.5 && Math.abs(v.y - origTgtY) < 0.5) {
        nv = { x: pin.x, y: approach.y }
      }
      link.vertices([nv])
      return
    }
  }

  link.vertices(tapVertices(pin, approach))
}

// Stub length that adapts to the horizontal space available before the next
// route point: shrink so the stub never overshoots that point; keep the full
// stub when the wire has to loop back anyway.
function adaptiveStub(room) {
  return room > 0 ? Math.min(WIRE_STUB, room / 2) : WIRE_STUB
}

// Insert an L-corner between any two consecutive points that are neither
// horizontally nor vertically aligned, so a `router: 'normal'` link stays
// strictly orthogonal.
function orthogonalize(points) {
  const out = []
  for (let i = 0; i < points.length; i += 1) {
    const point = points[i]
    out.push(point)
    const next = points[i + 1]
    if (!next) continue
    const aligned = Math.abs(point.x - next.x) < 0.1 || Math.abs(point.y - next.y) < 0.1
    if (!aligned) out.push({ x: next.x, y: point.y })
  }
  return out
}

// Snapshot the link's current vertices and port positions as the baseline for
// future recomputations. Called after the user finishes a segment-drag (or any
// other manual edit) so subsequent element drags shift from the user's manual
// route rather than reverting to the server's original layout.
function captureLinkBaseline(link) {
  if (!link || !graph) return
  if (!link.get('isWireLink') && !link.get('isWireTreeStub') && !link.get('isTapLink')) return

  // Snapshot whatever vertices the user has just finalized (post-segment-drag
  // and post-redundancyRemoval). No orthogonalize here — that would diverge
  // the baseline from the visible polyline and snap the wire on next move.
  const vertices = (link.vertices() || []).map((v) => ({ x: v.x, y: v.y }))
  link.set('serverVertices', vertices)
  if (link.has('treeVertices')) {
    link.set('treeVertices', vertices)
  }
  // Tap-link recompute defaults to regenerating an L-shape; flag the user
  // edit so it falls through to the slide branch instead.
  link.set('_userEdited', true)

  const srcCell = graph.getCell(link.get('source')?.id)
  const tgtCell = graph.getCell(link.get('target')?.id)
  if (srcCell) {
    link.set('srcOrigPortX', srcCell.position().x + link.get('srcRelX'))
    link.set('srcOrigPortY', srcCell.position().y + link.get('srcRelY'))
  }
  if (tgtCell) {
    link.set('tgtOrigPortX', tgtCell.position().x + link.get('tgtRelX'))
    link.set('tgtOrigPortY', tgtCell.position().y + link.get('tgtRelY'))
  }
}

// Slide the anchor adjacent to a moved endpoint along ONLY the axis
// perpendicular to its connecting segment. The parallel coordinate keeps the
// column/row that the next anchor depends on; shifting it would either extend
// the stub past the bend (leaving a tail) or pull the bend inside an element.
function slideEndAnchor(vertex, origRefX, origRefY, newRefX, newRefY) {
  const wasHoriz = Math.abs(vertex.y - origRefY) < 0.5
  const wasVert = Math.abs(vertex.x - origRefX) < 0.5
  if (wasHoriz) return { x: vertex.x, y: newRefY }
  if (wasVert) return { x: newRefX, y: vertex.y }
  return { x: vertex.x + (newRefX - origRefX), y: vertex.y + (newRefY - origRefY) }
}

// Re-route a regular wire after one of its endpoints moved. The server route's
// first vertex travels with the source and its last vertex with the target;
// the middle keeps the server's non-overlapping layout. Each port and its
// adjacent vertex shift by the same delta along the perpendicular axis only,
// so the port->vertex segment stays axis-aligned and the next bend's column
// (or row) is preserved.
function recomputeWireRoute(link) {
  if (!graph) return
  const srcCell = graph.getCell(link.get('source')?.id)
  const tgtCell = graph.getCell(link.get('target')?.id)
  if (!srcCell || !tgtCell) return

  const srcX = srcCell.position().x + link.get('srcRelX')
  const srcY = srcCell.position().y + link.get('srcRelY')
  const tgtX = tgtCell.position().x + link.get('tgtRelX')
  const tgtY = tgtCell.position().y + link.get('tgtRelY')

  const serverVertices = link.get('serverVertices') || []

  if (serverVertices.length >= 2) {
    const origSrcX = link.get('srcOrigPortX')
    const origSrcY = link.get('srcOrigPortY')
    const origTgtX = link.get('tgtOrigPortX')
    const origTgtY = link.get('tgtOrigPortY')
    const last = serverVertices.length - 1
    const anchors = serverVertices.map((vertex, i) => {
      if (i === 0) return slideEndAnchor(vertex, origSrcX, origSrcY, srcX, srcY)
      if (i === last) return slideEndAnchor(vertex, origTgtX, origTgtY, tgtX, tgtY)
      return { x: vertex.x, y: vertex.y }
    })
    link.router({ name: 'normal' })
    link.vertices(dedupePoints(orthogonalize(anchors)))
    return
  }

  // No usable server route (Manhattan-fallback wires): clean stub Z.
  const room = tgtX - srcX
  const sExit = { x: srcX + adaptiveStub(room), y: srcY }
  const tEnter = { x: tgtX - adaptiveStub(room), y: tgtY }
  const vertices = [sExit]
  if (Math.abs(sExit.x - tEnter.x) > 0.1 && Math.abs(sExit.y - tEnter.y) > 0.1) {
    const midX = Math.round((sExit.x + tEnter.x) / 2)
    vertices.push({ x: midX, y: srcY }, { x: midX, y: tgtY })
  }
  vertices.push(tEnter)
  link.router({ name: 'normal' })
  link.vertices(dedupePoints(vertices))
}

// A WireTree represents a multi-pin single-bit net as one shared element that
// owns the trunk segments between branch points plus a junction dot at each
// degree-3+ node. The thin segments from each pin to its nearest junction
// become per-pin "stub" links, so dragging a logic element re-routes only that
// pin's stub while the trunk stays put — analogous to how BusRail + tap-links
// work for multi-bit nets.
function wireTreePortGroups() {
  return {
    entry: {
      position: { name: 'absolute' },
      markup: PORT_MARKUP,
      attrs: PORT_ATTRS,
    },
  }
}

// Walk the segment tree of a wire to identify branch points (junctions) and
// classify each segment as either belonging to a pin's stub (leaf-to-nearest-
// junction chain) or to the inter-junction trunk. Returns null when the net
// has no branches — those wires keep the simple driver→sink rendering.
function buildWireTreeStructure(wire, refs) {
  const segments = wire.segments || []
  if (segments.length === 0) return null

  const KEY = (x, y) => `${Math.round(x)},${Math.round(y)}`
  const canonKey = (a, b) => {
    const ka = KEY(a.x, a.y)
    const kb = KEY(b.x, b.y)
    return ka < kb ? `${ka}|${kb}` : `${kb}|${ka}`
  }

  const nodeMap = new Map()
  for (const seg of segments) {
    const ka = KEY(seg.x1, seg.y1)
    const kb = KEY(seg.x2, seg.y2)
    if (ka === kb) continue
    if (!nodeMap.has(ka)) nodeMap.set(ka, { x: seg.x1, y: seg.y1, neighbors: [] })
    if (!nodeMap.has(kb)) nodeMap.set(kb, { x: seg.x2, y: seg.y2, neighbors: [] })
    nodeMap.get(ka).neighbors.push(kb)
    nodeMap.get(kb).neighbors.push(ka)
  }

  const junctionKeys = new Set()
  for (const [k, node] of nodeMap.entries()) {
    if (node.neighbors.length >= 3) junctionKeys.add(k)
  }
  if (junctionKeys.size === 0) return null

  for (const ref of refs) {
    if (!nodeMap.has(KEY(ref.port.x, ref.port.y))) return null
  }

  const pinStubs = []
  const usedCanon = new Set()

  for (const ref of refs) {
    const startKey = KEY(ref.port.x, ref.port.y)
    const stubSegs = []
    let prevKey = null
    let curKey = startKey

    while (true) {
      const curNode = nodeMap.get(curKey)
      const nextKey = curNode.neighbors.find((k) => k !== prevKey)
      if (!nextKey) break
      const nextNode = nodeMap.get(nextKey)
      stubSegs.push({
        x1: curNode.x, y1: curNode.y,
        x2: nextNode.x, y2: nextNode.y,
        canon: canonKey({ x: curNode.x, y: curNode.y }, { x: nextNode.x, y: nextNode.y }),
      })
      prevKey = curKey
      curKey = nextKey
      if (junctionKeys.has(curKey)) break
      if (nodeMap.get(curKey).neighbors.length === 1) break
    }

    const endNode = nodeMap.get(curKey)
    pinStubs.push({
      ref,
      segments: stubSegs,
      entryPoint: { x: endNode.x, y: endNode.y },
    })
    for (const s of stubSegs) usedCanon.add(s.canon)
  }

  const trunkSegments = []
  for (const seg of segments) {
    const c = canonKey({ x: seg.x1, y: seg.y1 }, { x: seg.x2, y: seg.y2 })
    if (!usedCanon.has(c)) trunkSegments.push({ x1: seg.x1, y1: seg.y1, x2: seg.x2, y2: seg.y2 })
  }

  const junctions = []
  for (const key of junctionKeys) {
    const node = nodeMap.get(key)
    junctions.push({ x: node.x, y: node.y })
  }

  return { pinStubs, trunkSegments, junctions }
}

function createWireTreeElement(decomposition, wire) {
  const { trunkSegments, junctions, pinStubs } = decomposition

  const points = []
  for (const seg of trunkSegments) {
    points.push({ x: seg.x1, y: seg.y1 }, { x: seg.x2, y: seg.y2 })
  }
  for (const j of junctions) points.push(j)
  for (const stub of pinStubs) points.push(stub.entryPoint)
  if (points.length === 0) return null

  let minX = points[0].x, maxX = points[0].x
  let minY = points[0].y, maxY = points[0].y
  for (const p of points) {
    if (p.x < minX) minX = p.x
    if (p.x > maxX) maxX = p.x
    if (p.y < minY) minY = p.y
    if (p.y > maxY) maxY = p.y
  }

  const PAD = 6
  const posX = minX - PAD
  const posY = minY - PAD
  const width = Math.max(20, maxX - minX + 2 * PAD)
  const height = Math.max(20, maxY - minY + 2 * PAD)

  const markup = [{ tagName: 'rect', selector: 'hit' }]
  const attrs = {
    hit: {
      x: 0, y: 0, width, height,
      fill: '#ffffff', fillOpacity: 0, stroke: 'none',
    },
  }

  for (let i = 0; i < trunkSegments.length; i += 1) {
    const seg = trunkSegments[i]
    const sel = `trunk${i}`
    markup.push({ tagName: 'line', selector: sel })
    attrs[sel] = {
      x1: seg.x1 - posX, y1: seg.y1 - posY,
      x2: seg.x2 - posX, y2: seg.y2 - posY,
      stroke: '#000000', strokeWidth: 1.85, strokeLinecap: 'square',
      vectorEffect: 'non-scaling-stroke',
    }
  }

  for (let i = 0; i < junctions.length; i += 1) {
    const j = junctions[i]
    const sel = `dot${i}`
    markup.push({ tagName: 'circle', selector: sel })
    attrs[sel] = {
      cx: j.x - posX, cy: j.y - posY, r: 2,
      fill: '#000000', stroke: 'none',
      vectorEffect: 'non-scaling-stroke',
    }
  }

  const ports = []
  const portIdByIndex = []
  for (let i = 0; i < pinStubs.length; i += 1) {
    const stub = pinStubs[i]
    const pid = `entry-${i}`
    portIdByIndex.push(pid)
    ports.push({
      id: pid,
      group: 'entry',
      args: { x: stub.entryPoint.x - posX, y: stub.entryPoint.y - posY },
    })
  }

  const element = new cellNamespace.gost.WireTree({
    id: wire.id,
    position: { x: posX, y: posY },
    size: { width, height },
    markup,
    attrs,
    ports: { groups: wireTreePortGroups(), items: ports },
    z: 10,
  })

  return { element, portIdByIndex, posX, posY }
}

function wireTreeStubVertices(pinStub) {
  const segs = pinStub.segments
  if (segs.length === 0) return []
  if (segs.length === 1) {
    const a = { x: segs[0].x1, y: segs[0].y1 }
    const b = { x: segs[0].x2, y: segs[0].y2 }
    const alignedX = Math.abs(a.x - b.x) < 0.5
    const alignedY = Math.abs(a.y - b.y) < 0.5
    if (alignedX || alignedY) return []
    return [{ x: b.x, y: a.y }]
  }

  const pts = []
  for (let i = 0; i < segs.length - 1; i += 1) {
    pts.push({ x: segs[i].x2, y: segs[i].y2 })
  }
  const head = { x: segs[0].x1, y: segs[0].y1 }
  const tail = { x: segs[segs.length - 1].x2, y: segs[segs.length - 1].y2 }
  return filterBends([head, ...pts, tail]).slice(1, -1)
}

function createWireTreeStubLink(wire, pinStub, treeId, treePosX, treePosY, nodesById, portIdOnTree) {
  const ref = pinStub.ref
  const pinPt = { x: ref.port.x, y: ref.port.y }
  const entryPt = pinStub.entryPoint
  const stubVerts = wireTreeStubVertices(pinStub)
  const refIsSource = ref.direction === 'out'

  const source = refIsSource ? endpoint(ref) : { id: treeId, port: portIdOnTree }
  const target = refIsSource ? { id: treeId, port: portIdOnTree } : endpoint(ref)
  const orderedVerts = refIsSource ? stubVerts : stubVerts.slice().reverse()

  const link = new joint.dia.Link({
    type: 'gost.Link',
    id: `${wire.id}__tree__${ref.portId}`,
    netId: wire.netIds[0],
    markup: LINK_MARKUP,
    source,
    target,
    router: { name: 'normal' },
    connector: { name: 'straight' },
    attrs: {
      wrapper: wrapperAttrs(),
      line: lineAttrs(2),
    },
    z: 20,
  })
  if (orderedVerts.length > 0) link.vertices(orderedVerts)

  const srcNode = nodesById.get(ref.elementId)
  const pinRelX = srcNode ? pinPt.x - srcNode.x : 0
  const pinRelY = srcNode ? pinPt.y - srcNode.y : 0
  const entryRelX = entryPt.x - treePosX
  const entryRelY = entryPt.y - treePosY

  link.set('isWireTreeStub', true)
  link.set('refIsSource', refIsSource)
  link.set('srcRelX', refIsSource ? pinRelX : entryRelX)
  link.set('srcRelY', refIsSource ? pinRelY : entryRelY)
  link.set('tgtRelX', refIsSource ? entryRelX : pinRelX)
  link.set('tgtRelY', refIsSource ? entryRelY : pinRelY)
  link.set('srcOrigPortX', refIsSource ? pinPt.x : entryPt.x)
  link.set('srcOrigPortY', refIsSource ? pinPt.y : entryPt.y)
  link.set('tgtOrigPortX', refIsSource ? entryPt.x : pinPt.x)
  link.set('tgtOrigPortY', refIsSource ? entryPt.y : pinPt.y)
  link.set('treeVertices', orderedVerts.map((v) => ({ x: v.x, y: v.y })))

  return link
}

function recomputeWireTreeStubRoute(link) {
  if (!graph) return
  const srcCell = graph.getCell(link.get('source')?.id)
  const tgtCell = graph.getCell(link.get('target')?.id)
  if (!srcCell || !tgtCell) return

  const srcX = srcCell.position().x + link.get('srcRelX')
  const srcY = srcCell.position().y + link.get('srcRelY')
  const tgtX = tgtCell.position().x + link.get('tgtRelX')
  const tgtY = tgtCell.position().y + link.get('tgtRelY')
  const verts = link.get('treeVertices') || []

  if (verts.length === 0) {
    const alignedX = Math.abs(srcX - tgtX) < 0.5
    const alignedY = Math.abs(srcY - tgtY) < 0.5
    if (alignedX || alignedY) {
      link.vertices([])
      link.router({ name: 'normal' })
      return
    }
    // Original stub was straight; after a move the endpoints no longer align.
    // Place the perpendicular leg on a middle column/row between trunk and
    // element so it overlaps neither the trunk's axis nor the element's edge.
    const wasHoriz = Math.abs(link.get('srcOrigPortY') - link.get('tgtOrigPortY')) < 0.5
    const zVertices = []
    if (wasHoriz) {
      const room = tgtX - srcX
      const stub = adaptiveStub(Math.abs(room)) * (room >= 0 ? 1 : -1)
      const sExit = { x: srcX + stub, y: srcY }
      const tEnter = { x: tgtX - stub, y: tgtY }
      zVertices.push(sExit)
      if (Math.abs(sExit.x - tEnter.x) > 0.1 && Math.abs(sExit.y - tEnter.y) > 0.1) {
        const midX = Math.round((sExit.x + tEnter.x) / 2)
        zVertices.push({ x: midX, y: srcY }, { x: midX, y: tgtY })
      }
      zVertices.push(tEnter)
    } else {
      const room = tgtY - srcY
      const stub = adaptiveStub(Math.abs(room)) * (room >= 0 ? 1 : -1)
      const sExit = { x: srcX, y: srcY + stub }
      const tEnter = { x: tgtX, y: tgtY - stub }
      zVertices.push(sExit)
      if (Math.abs(sExit.x - tEnter.x) > 0.1 && Math.abs(sExit.y - tEnter.y) > 0.1) {
        const midY = Math.round((sExit.y + tEnter.y) / 2)
        zVertices.push({ x: srcX, y: midY }, { x: tgtX, y: midY })
      }
      zVertices.push(tEnter)
    }
    link.router({ name: 'normal' })
    link.vertices(dedupePoints(zVertices))
    return
  }

  if (verts.length === 1) {
    const v = verts[0]
    const origSrcX = link.get('srcOrigPortX')
    const origSrcY = link.get('srcOrigPortY')
    const origTgtX = link.get('tgtOrigPortX')
    const origTgtY = link.get('tgtOrigPortY')
    let nv = { x: v.x, y: v.y }
    if (Math.abs(v.x - origSrcX) < 0.5 && Math.abs(v.y - origTgtY) < 0.5) {
      nv = { x: srcX, y: tgtY }
    } else if (Math.abs(v.x - origTgtX) < 0.5 && Math.abs(v.y - origSrcY) < 0.5) {
      nv = { x: tgtX, y: srcY }
    }
    link.vertices([nv])
    link.router({ name: 'normal' })
    return
  }

  const origSrcX = link.get('srcOrigPortX')
  const origSrcY = link.get('srcOrigPortY')
  const origTgtX = link.get('tgtOrigPortX')
  const origTgtY = link.get('tgtOrigPortY')
  const last = verts.length - 1
  const anchors = verts.map((vertex, i) => {
    if (i === 0) return slideEndAnchor(vertex, origSrcX, origSrcY, srcX, srcY)
    if (i === last) return slideEndAnchor(vertex, origTgtX, origTgtY, tgtX, tgtY)
    return { x: vertex.x, y: vertex.y }
  })
  link.router({ name: 'normal' })
  link.vertices(dedupePoints(orthogonalize(anchors)))
}

function nearestRailEndpoint(ref, geometry) {
  const point = ref.port || { x: geometry.x, y: geometry.y }
  if (geometry.horiz) {
    return point.x <= geometry.x + geometry.len / 2 ? 'rail-start' : 'rail-end'
  }
  return point.y <= geometry.y + geometry.len / 2 ? 'rail-start' : 'rail-end'
}

function busDirectionForPort(portId, geometry, role) {
  if (geometry.horiz) {
    if (portId === 'rail-start') return ['left']
    if (portId === 'rail-end') return ['right']
    return role === 'source' ? ['right'] : ['left']
  }
  if (portId === 'rail-start') return ['top']
  if (portId === 'rail-end') return ['bottom']
  return role === 'source' ? ['bottom'] : ['top']
}

function createTerminalBusLink(ref, busId, geometry) {
  const railPort = nearestRailEndpoint(ref, geometry)
  const refIsSource = ref.direction === 'out'
  const source = refIsSource ? endpoint(ref) : { id: busId, port: railPort }
  const target = refIsSource ? { id: busId, port: railPort } : endpoint(ref)
  const startDirections = refIsSource ? routerDirectionForRef(ref, 'source') : busDirectionForPort(railPort, geometry, 'source')
  const endDirections = refIsSource ? busDirectionForPort(railPort, geometry, 'target') : routerDirectionForRef(ref, 'target')

  return new joint.dia.Link({
    type: 'gost.Link',
    id: `${busId}__terminal__${ref.portId}`,
    netId: ref.netId,
    markup: LINK_MARKUP,
    source,
    target,
    router: {
      name: 'manhattan',
      args: {
        step: 8,
        padding: 28,
        startDirections,
        endDirections,
      },
    },
    connector: { name: 'straight' },
    attrs: {
      wrapper: wrapperAttrs(),
      line: lineAttrs(3.9),
    },
    z: 20,
  })
}

function tapStartPoint(tap) {
  const first = tap.segments?.[0]
  if (!first) return null
  return { x: first.x1, y: first.y1 }
}

function squaredDistance(a, b) {
  const dx = a.x - b.x
  const dy = a.y - b.y
  return dx * dx + dy * dy
}

function logicRefsForTap(entry, tap, nodesById) {
  const refs = [...(entry.driver ? [entry.driver] : []), ...entry.sinks]
    .filter((ref) => {
      const node = nodesById.get(ref.elementId)
      return node && !isTerminal(node)
    })

  const start = tapStartPoint(tap)
  if (!start || refs.length <= 1) return refs

  let closest = null
  for (const ref of refs) {
    const d = squaredDistance(start, ref.port)
    if (!closest || d < closest.d) closest = { ref, d }
  }

  return closest ? [closest.ref] : []
}

function createDot(x, y) {
  return new cellNamespace.gost.Annotation({
    id: `junction__${Math.round(x)}__${Math.round(y)}`,
    isDot: true,
    position: { x: x - 2, y: y - 2 },
    size: { width: 4, height: 4 },
    markup: [{ tagName: 'circle', selector: 'dot' }],
    attrs: {
      dot: {
        cx: 2,
        cy: 2,
        r: 1.8,
        fill: '#000000',
        stroke: 'none',
        vectorEffect: 'non-scaling-stroke',
      },
    },
    z: 30,
  })
}

function buildNetIndex(diagram) {
  const netIndex = new Map()
  const nodesById = new Map()

  for (const node of diagram.nodes || []) {
    nodesById.set(node.id, node)
    for (const port of node.ports || []) {
      if (!port.netId) continue
      if (!netIndex.has(port.netId)) netIndex.set(port.netId, { driver: null, sinks: [] })
      const entry = netIndex.get(port.netId)
      const ref = {
        elementId: node.id,
        portId: portId(node.id, port),
        netId: port.netId,
        direction: port.direction,
        nodeKind: node.kind,
        port,
      }
      if (port.direction === 'out') {
        if (!entry.driver) entry.driver = ref
      } else {
        entry.sinks.push(ref)
      }
    }
  }

  return { netIndex, nodesById }
}

async function buildGraph() {
  if (!graph || !props.diagram) return

  const diagram = props.diagram
  const cells = []
  const busNetIds = new Set()
  activeLinkView = null

  for (const wire of diagram.wires || []) {
    if (wire.bus) wire.netIds.forEach((id) => busNetIds.add(id))
  }

  const { netIndex, nodesById } = buildNetIndex(diagram)

  for (const node of diagram.nodes || []) {
    cells.push(isTerminal(node) ? createTerminalElement(node) : createNodeElement(node))
  }

  // Bus rails and wire trees are skipped entirely in labels-mode.
  const skipWires = props.connectionStyle === 'labels'
  for (const wire of skipWires ? [] : diagram.wires || []) {
    if (!wire.bus || !wire.segments?.length) continue
    const geometry = busRailGeometry(wire)
    const mergedTapIndex = buildMergedTapIndex(wire)
    const busRail = createBusRailElement(wire, geometry, mergedTapIndex)
    cells.push(busRail)
    const railPos = busRail.position()

    // For each merged-tap group build a standalone WireColumn + the oblique
    // link tying its top to the bus rail. mergedTargetByTapIndex tells the
    // tap-link factory which branch port on the WireColumn to anchor to.
    const mergedTargetByTapIndex = new Map()
    const mergedGroups = findMergedTapGroups(wire)
    for (const group of mergedGroups) {
      const built = createWireColumnElement(wire, group)
      cells.push(built.element)
      const primaryTap = wire.taps[group.members[0].tapIndex]
      const obliqueLink = createObliqueLink(
        busRail.id,
        `busTap-${group.members[0].tapIndex}`,
        built.element.id,
        'top',
        primaryTap?.netId ?? wire.netIds[0],
        `${group.netId}_${Math.round(group.climbX)}`,
      )
      cells.push(obliqueLink)

      for (const m of group.members) {
        const branchPortId = built.branchPortByTapIndex.get(m.tapIndex)
        if (!branchPortId) continue
        mergedTargetByTapIndex.set(m.tapIndex, {
          cellId: built.element.id,
          portId: branchPortId,
          point: { x: group.climbX, y: m.branchY },
          posX: built.posX,
          posY: built.posY,
        })
      }
    }

    for (const [tapIndex, tap] of (wire.taps || []).entries()) {
      const entry = netIndex.get(tap.netId)
      if (!entry) continue
      const mergedTarget = mergedTargetByTapIndex.get(tapIndex) || null
      for (const ref of logicRefsForTap(entry, tap, nodesById)) {
        cells.push(createTapLink(wire, tap, ref, tapIndex, nodesById, railPos, mergedTarget))
      }
    }

    const firstNetId = wire.netIds[0]
    const terminalEntry = netIndex.get(firstNetId)
    if (terminalEntry) {
      const refs = [...(terminalEntry.driver ? [terminalEntry.driver] : []), ...terminalEntry.sinks]
      for (const ref of refs) {
        const node = nodesById.get(ref.elementId)
        if (node && isTerminal(node)) cells.push(createTerminalBusLink(ref, wire.id, geometry))
      }
    }
  }

  const wireTreeNetIds = new Set()
  for (const wire of skipWires ? [] : diagram.wires || []) {
    if (wire.bus || !wire.segments?.length) continue
    const netId = wire.netIds[0]
    if (!netId || busNetIds.has(netId)) continue
    const entry = netIndex.get(netId)
    if (!entry?.driver) continue
    const refs = [entry.driver, ...entry.sinks]
    if (refs.length < 3) continue

    const decomposition = buildWireTreeStructure(wire, refs)
    if (!decomposition || decomposition.junctions.length === 0) continue

    const built = createWireTreeElement(decomposition, wire)
    if (!built) continue

    cells.push(built.element)
    wireTreeNetIds.add(netId)

    for (let i = 0; i < decomposition.pinStubs.length; i += 1) {
      const stub = decomposition.pinStubs[i]
      const portIdOnTree = built.portIdByIndex[i]
      if (!portIdOnTree) continue
      cells.push(createWireTreeStubLink(wire, stub, built.element.id, built.posX, built.posY, nodesById, portIdOnTree))
    }
  }

  // In labels-mode wires are not drawn — every pin carries its own net-name
  // label instead. Skip every wire-related link/tree element.
  if (props.connectionStyle !== 'labels') {
    for (const wire of diagram.wires || []) {
      if (wire.bus) continue
      const netId = wire.netIds[0]
      if (!netId || busNetIds.has(netId) || wireTreeNetIds.has(netId)) continue
      const entry = netIndex.get(netId)
      if (!entry?.driver) continue
      for (const sink of entry.sinks) {
        cells.push(createWireLink(wire, entry.driver, sink, nodesById))
      }
    }
  }

  building = true
  graph.startBatch('rebuild')
  try {
    graph.clear()
    graph.resetCells(cells)
  } finally {
    graph.stopBatch('rebuild')
    building = false
  }

  await nextTick()
  setTimeout(() => updateJunctionDots(), 0)
}

function selectNode(id) {
  if (selectedId.value) {
    const prev = graph?.getCell(selectedId.value)
    if (prev?.get('type') === 'gost.Node') {
      prev.attr('body/strokeDasharray', null)
      prev.attr('body/strokeWidth', 2)
    }
  }

  selectedId.value = id

  if (id) {
    const el = graph?.getCell(id)
    if (el?.get('type') === 'gost.Node') {
      el.attr('body/strokeDasharray', '6 4')
      el.attr('body/strokeWidth', 3.3)
    }
  }
}

function clearLinkTools() {
  activeLinkView?.removeTools()
  activeLinkView = null
}

const HANDLE_VISUAL_SCALE = 0.55

// Segments tool that forbids vertical drag motion (the only kind of drag that
// can move a horizontal trunk into a bus-tap tangle). Standard tool exposes
// `handle.options.axis = 'y'` for horizontal segments — we keep the handle
// visible (so the user still sees the wire is editable) but override
// onHandleChanging to drop changes on the y axis. Vertical segments stay
// fully draggable; horizontal segments are visually locked in place.
//
// In addition, captureLinkBaseline runs ONLY when a real drag actually
// happened: onHandleChangeStart resets a flag, onHandleChanging sets it on
// allowed drags, and onHandleChangeEnd captures only if the flag is set.
// That prevents a pure click (no edit) from snapshotting recompute-modified
// vertices and breaking subsequent element-driven recomputes.
const VerticalDragLockedSegments = joint.linkTools.Segments.extend({
  onHandleChangeStart: function (handle, evt) {
    this._dragMoved = false
    joint.linkTools.Segments.prototype.onHandleChangeStart.call(this, handle, evt)
  },
  onHandleChanging: function (handle, evt) {
    if (handle.options.axis === 'y') {
      // Vertical drag forbidden — drop the move entirely.
      return
    }
    this._dragMoved = true
    joint.linkTools.Segments.prototype.onHandleChanging.call(this, handle, evt)
  },
  onHandleChangeEnd: function (handle, evt) {
    joint.linkTools.Segments.prototype.onHandleChangeEnd.call(this, handle, evt)
    if (!this._dragMoved) return
    this._dragMoved = false

    // Post-drag guard: if any segment came out diagonal (e.g. due to a corner
    // not being inserted in time), split it into an L. Then snapshot the
    // cleaned route as the new recompute baseline.
    const link = this.relatedView.model
    let vertices = (link.vertices() || []).map((v) => ({ x: v.x, y: v.y }))
    const cleaned = dedupePoints(orthogonalize(vertices))
    if (cleaned.length !== vertices.length
      || cleaned.some((p, i) => p.x !== vertices[i].x || p.y !== vertices[i].y)) {
      link.vertices(cleaned)
    }
    captureLinkBaseline(link)
  },
})

function currentPaperScale() {
  if (!paper) return 1
  const s = paper.scale()
  return s?.sx || 1
}

function showLinkTools(view) {
  clearLinkTools()
  // stopPropagation:false makes the paper-level link:pointerup fire after a
  // segment drag so captureLinkBaseline can persist the user's edit into the
  // recompute baseline (otherwise the next element move reverts the route).
  // scale = constant / paperScale keeps handles at constant visual size as
  // the canvas zooms in or out.
  const inverseScale = HANDLE_VISUAL_SCALE / currentPaperScale()
  const toolsView = new joint.dia.ToolsView({
    tools: [
      new VerticalDragLockedSegments({
        // Disable snap so the wire follows the cursor 1:1. Default snap=10
        // makes the handle and wire stick to neighbour columns until the
        // pointer leaves an 8-pixel zone — feels broken on tight schematics.
        snapRadius: 0,
        snapHandle: false,
        // Drop anchor manipulation on first/last segment drags. Otherwise
        // dragging the segment touching a port can rewrite source/target
        // anchors and pull the polyline off the orthogonal grid.
        anchor: null,
        redundancyRemoval: true,
        segmentLengthThreshold: 16,
        scale: inverseScale,
        stopPropagation: false,
      }),
    ],
  })
  view.addTools(toolsView)
  view.showTools()
  activeLinkView = view
}

function refreshLinkToolsScale() {
  if (activeLinkView) showLinkTools(activeLinkView)
  if (activeElementToolsView) showWireColumnTool(activeElementToolsView)
}

// Element-tool oval for WireColumn. Inherits joint.elementTools.Control so the
// user sees the same draggable-circle affordance as with link Segments. The
// handle sits at the column's midpoint; setPosition translates the WireColumn
// strictly on X (Y stays locked so the bus relationship is preserved).
const WireColumnHandle = joint.elementTools.Control.extend({
  options: {
    handleAttributes: {
      r: 6,
      fill: '#33334F',
      stroke: '#FFFFFF',
      'stroke-width': 2,
    },
    selector: 'hit',
  },
  getPosition: function (view) {
    const bbox = view.model.getBBox()
    return { x: bbox.width / 2, y: bbox.height / 2 }
  },
  setPosition: function (view, position) {
    const bbox = view.model.getBBox()
    const dx = position.x - bbox.width / 2
    if (Math.abs(dx) < 0.5) return
    view.model.translate(dx, 0, { ui: true, tool: this.cid })
  },
})

function showWireColumnTool(view) {
  clearElementTools()
  if (!view) return
  const scale = HANDLE_VISUAL_SCALE / currentPaperScale()
  const toolsView = new joint.dia.ToolsView({
    tools: [new WireColumnHandle({ scale })],
  })
  view.addTools(toolsView)
  view.showTools()
  activeElementToolsView = view
}

function clearElementTools() {
  if (activeElementToolsView) {
    activeElementToolsView.removeTools?.()
    activeElementToolsView = null
  }
}

function onLabelInput(nodeId, value) {
  labelEdits[nodeId] = value
  const el = graph?.getCell(nodeId)
  if (!el) return
  const type = el.get('type')
  if (type === 'gost.Terminal') {
    el.attr('label/text', value)
  } else if (type === 'gost.Node') {
    el.attr('labelText/text', value)
  }
}

function pathSegments(connection) {
  if (!connection) return []
  if (typeof connection.getSegments === 'function') return connection.getSegments()
  return connection.segments || []
}

function routePointsFromConnection(connection) {
  const pts = []
  for (const segment of pathSegments(connection)) {
    if (segment.type !== 'L') continue
    try {
      pts.push({ x: segment.start.x, y: segment.start.y })
      pts.push({ x: segment.end.x, y: segment.end.y })
    } catch {
      // Ignore malformed path segment from an unfinished LinkView render.
    }
  }
  return pts.length ? dedupePoints(pts) : []
}

async function updateJunctionDots() {
  if (!graph || !paper) return

  graph.getCells()
    .filter((cell) => cell.get('type') === 'gost.Annotation' && cell.get('isDot'))
    .forEach((cell) => cell.remove())

  await nextTick()

  const routes = []
  for (const link of graph.getLinks()) {
    const view = paper.findViewByModel(link)
    if (!view) continue
    const points = routePointsFromConnection(view.getConnection())
    if (points.length > 1) {
      routes.push({
        netId: link.get('netId'),
        points,
      })
    }
  }

  const dots = new Map()

  for (let i = 0; i < routes.length; i += 1) {
    const routeA = routes[i]
    const candidates = routeA.points.slice(1)

    for (let j = 0; j < routes.length; j += 1) {
      if (i === j) continue
      const routeB = routes[j]
      if (routeA.netId !== routeB.netId) continue

      for (const candidate of candidates) {
        for (let k = 0; k + 1 < routeB.points.length; k += 1) {
          if (pointOnSegmentStrict(candidate, routeB.points[k], routeB.points[k + 1])) {
            dots.set(pointKey(candidate), candidate)
          }
        }
      }
    }
  }

  for (const { x, y } of dots.values()) {
    graph.addCell(createDot(x, y))
  }
}

function pointOnSegmentStrict(point, p1, p2) {
  const eps = 1.5
  const minX = Math.min(p1.x, p2.x)
  const maxX = Math.max(p1.x, p2.x)
  const minY = Math.min(p1.y, p2.y)
  const maxY = Math.max(p1.y, p2.y)
  const onX = point.x >= minX - eps && point.x <= maxX + eps
  const onY = point.y >= minY - eps && point.y <= maxY + eps
  if (!onX || !onY) return false

  const horizSeg = Math.abs(p1.y - p2.y) < eps
  const vertSeg = Math.abs(p1.x - p2.x) < eps
  if (horizSeg && Math.abs(point.y - p1.y) > eps) return false
  if (vertSeg && Math.abs(point.x - p1.x) > eps) return false

  const atP1 = Math.abs(point.x - p1.x) < eps && Math.abs(point.y - p1.y) < eps
  const atP2 = Math.abs(point.x - p2.x) < eps && Math.abs(point.y - p2.y) < eps
  return !atP1 && !atP2
}

function dedupePoints(points) {
  if (!points.length) return []
  const result = [points[0]]
  for (let i = 1; i < points.length; i += 1) {
    const prev = result[result.length - 1]
    if (Math.abs(points[i].x - prev.x) > 0.1 || Math.abs(points[i].y - prev.y) > 0.1) {
      result.push(points[i])
    }
  }
  return result
}

function updatePaperSize() {
  if (!paper || !paperHost.value) return
  // JointJS Paper устанавливает inline width/height на сам paper-host —
  // поэтому брать его собственный rect = читать своё же значение.
  // Считаем из родителя минус inset (18px с каждой стороны).
  const parent = paperHost.value.parentElement
  if (!parent) return
  const PAD = 18 * 2
  const w = Math.max(1, parent.clientWidth - PAD)
  const h = Math.max(1, parent.clientHeight - PAD)
  paper.setDimensions(w, h)
}

function applyCamera() {
  if (!paper || !paperHost.value) return
  // updatePaperSize() sets paper-host to (parent − 36, parent − 36); use the
  // same dimensions to keep scale calculations consistent.
  const parent = paperHost.value.parentElement
  const PAD = 18 * 2
  const w = parent ? parent.clientWidth - PAD : paperHost.value.clientWidth
  const h = parent ? parent.clientHeight - PAD : paperHost.value.clientHeight
  const scale = Math.min(w / Math.max(1, camera.width), h / Math.max(1, camera.height))
  paper.scale(scale, scale)
  paper.translate(-camera.x * scale, -camera.y * scale)
  refreshLinkToolsScale()
}

function fitToBounds() {
  const bounds = props.diagram?.bounds || { x: 0, y: 0, width: 1000, height: 640 }
  Object.assign(camera, {
    x: bounds.x,
    y: bounds.y,
    width: Math.max(1, bounds.width),
    height: Math.max(1, bounds.height),
  })
  applyCamera()
}

function localPoint(event) {
  const rect = paperHost.value?.getBoundingClientRect()
  if (!rect) return { x: camera.x + camera.width / 2, y: camera.y + camera.height / 2 }
  return {
    x: camera.x + ((event.clientX - rect.left) / rect.width) * camera.width,
    y: camera.y + ((event.clientY - rect.top) / rect.height) * camera.height,
  }
}

function zoomBy(factor, focus = null) {
  const bounds = props.diagram?.bounds || { width: 1000 }
  const nextWidth = Math.min(bounds.width * 3, Math.max(bounds.width * 0.12, camera.width * factor))
  const ratio = nextWidth / camera.width
  const point = focus || { x: camera.x + camera.width / 2, y: camera.y + camera.height / 2 }
  camera.x = point.x - (point.x - camera.x) * ratio
  camera.y = point.y - (point.y - camera.y) * ratio
  camera.width = nextWidth
  camera.height *= ratio
  applyCamera()
}

function onWheel(event) {
  zoomBy(event.deltaY < 0 ? 0.86 : 1.16, localPoint(event))
}

function onPaperPointerDown(event) {
  if (event.button !== 0) return
  // Skip panning when the pointerdown lands on a cell, on any JointJS tool
  // handle, or any element marked as a tool — otherwise grabbing a segment
  // handle steals the event for canvas panning instead of editing the wire.
  if (event.target.closest('.joint-cell, .joint-tools-layer, .joint-tool, .joint-link-tool, [data-tool-name]')) return
  panning.value = true
  lastPointer.x = event.clientX
  lastPointer.y = event.clientY
  event.currentTarget.setPointerCapture?.(event.pointerId)
}

function onPaperPointerMove(event) {
  if (!panning.value || !paperHost.value) return
  const rect = paperHost.value.getBoundingClientRect()
  const dx = ((event.clientX - lastPointer.x) / rect.width) * camera.width
  const dy = ((event.clientY - lastPointer.y) / rect.height) * camera.height
  camera.x -= dx
  camera.y -= dy
  lastPointer.x = event.clientX
  lastPointer.y = event.clientY
  applyCamera()
}

function onPaperPointerUp(event) {
  panning.value = false
  event.currentTarget.releasePointerCapture?.(event.pointerId)
}

function editedDiagram() {
  const copy = JSON.parse(JSON.stringify(props.diagram || {}))
  for (const node of copy.nodes || []) {
    const el = graph?.getCell(node.id)
    if (!el) continue
    const pos = el.position()
    const origNode = props.diagram?.nodes?.find((item) => item.id === node.id)
    if (!origNode) continue
    const dx = pos.x - origNode.x
    const dy = pos.y - origNode.y
    node.x = pos.x
    node.y = pos.y
    node.label = labelEdits[node.id] ?? node.label
    for (const port of node.ports || []) {
      port.x += dx
      port.y += dy
    }
  }
  return copy
}

// Fetch a font once and cache it as a data: URI so we can embed it inline
// into exported SVGs (the standalone file should be viewable offline).
const fontDataUriCache = new Map()
async function loadFontDataUri(path) {
  if (fontDataUriCache.has(path)) return fontDataUriCache.get(path)
  try {
    const r = await fetch(path)
    if (!r.ok) throw new Error(`${path}: HTTP ${r.status}`)
    const buf = new Uint8Array(await r.arrayBuffer())
    // base64 encode without blowing the stack for large buffers
    let bin = ''
    for (let i = 0; i < buf.length; i += 0x8000) {
      bin += String.fromCharCode.apply(null, buf.subarray(i, i + 0x8000))
    }
    const uri = `data:font/ttf;base64,${btoa(bin)}`
    fontDataUriCache.set(path, uri)
    return uri
  } catch (e) {
    console.warn('GOST font embed failed', path, e)
    return null
  }
}

async function exportSvg(fileName = `gost-${props.level}.svg`) {
  if (!paperHost.value) return

  const prevSelected = selectedId.value
  selectNode(null)
  clearLinkTools()
  await nextTick()

  // Export EXACTLY what's visible on the canvas — preserve the current pan,
  // zoom and viewport size from the live SVG. We do NOT touch the joint-layers
  // transform here (that holds the camera) and we do NOT recompute viewBox
  // from getContentArea — the user's current view is the source of truth.
  const liveSvg = paperHost.value.querySelector('svg')
  if (!liveSvg) {
    if (prevSelected) selectNode(prevSelected)
    return
  }
  const rect = liveSvg.getBoundingClientRect()
  const w = Math.max(1, Math.round(rect.width))
  const h = Math.max(1, Math.round(rect.height))

  const svgEl = liveSvg.cloneNode(true)
  svgEl.setAttribute('xmlns', 'http://www.w3.org/2000/svg')
  svgEl.setAttribute('width', String(w))
  svgEl.setAttribute('height', String(h))
  svgEl.setAttribute('viewBox', `0 0 ${w} ${h}`)
  // JointJS leaves inline style="position:absolute;overflow:hidden" on the
  // root <svg> — strip both: overflow:hidden would clip the diagram even in
  // standalone viewers; position:absolute is meaningless outside of the app.
  svgEl.removeAttribute('style')

  // Embed the GOST fonts inline. Without this the exported SVG falls
  // back to the system sans-serif, which doesn't match the УГО style.
  const [italicUri, regularUri] = await Promise.all([
    loadFontDataUri('/fonts/GOST2304_TypeB_italic.ttf'),
    loadFontDataUri('/fonts/GOST2304_TypeB.ttf'),
  ])
  const fontFaces = []
  if (regularUri) {
    fontFaces.push(
      `@font-face{font-family:"GOST Type B";src:url(${regularUri}) format("truetype");font-style:normal;font-weight:normal;}`,
    )
  }
  if (italicUri) {
    fontFaces.push(
      `@font-face{font-family:"GOST Type B Italic";src:url(${italicUri}) format("truetype");font-style:italic;}`,
      // Also map regular GOST family to the italic file as a graceful fallback.
      `@font-face{font-family:"GOST Type B";src:url(${italicUri}) format("truetype");font-style:italic;}`,
    )
  }
  if (fontFaces.length) {
    const defs = document.createElementNS('http://www.w3.org/2000/svg', 'defs')
    const style = document.createElementNS('http://www.w3.org/2000/svg', 'style')
    style.textContent = fontFaces.join('\n')
    defs.appendChild(style)
    svgEl.insertBefore(defs, svgEl.firstChild)
  }

  const source = `<?xml version="1.0" encoding="UTF-8"?>\n${new XMLSerializer().serializeToString(svgEl)}`
  const blob = new Blob([source], { type: 'image/svg+xml;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  Object.assign(document.createElement('a'), { href: url, download: fileName }).click()
  URL.revokeObjectURL(url)

  if (prevSelected) selectNode(prevSelected)
}

defineExpose({ exportSvg, editedDiagram })
</script>

<template>
  <div class="gost-svg-editor">
    <div
      ref="paperHost"
      class="gost-paper-host"
      :class="{ panning }"
      @wheel.prevent="onWheel"
      @pointerdown="onPaperPointerDown"
      @pointermove="onPaperPointerMove"
      @pointerup="onPaperPointerUp"
      @pointercancel="onPaperPointerUp"
    ></div>

    <div class="zoom-controls">
      <button title="Увеличить" @click="zoomBy(0.82)">+</button>
      <button title="Уменьшить" @click="zoomBy(1.22)">-</button>
      <button title="Показать целиком" @click="fitToBounds">fit</button>
    </div>

    <aside v-if="selectedNode" class="gost-inspector">
      <div class="inspector-title">{{ selectedNode.id }}</div>
      <label>
        Обозначение
        <input :value="selectedNode.label" @input="onLabelInput(selectedNode.id, $event.target.value)" />
      </label>
      <div class="inspector-ports">
        <div v-for="port in selectedNode.ports" :key="port.name + port.netId">
          <span>{{ port.name }}</span>
          <b>{{ port.netId }}</b>
        </div>
      </div>
    </aside>
  </div>
</template>

<style scoped>
.gost-svg-editor {
  position: relative;
  width: 100%;
  height: 100%;
  min-height: 0;
  background: var(--ctp-base);
}

.gost-paper-host {
  position: absolute;
  inset: 18px;
  overflow: hidden;
  background: #ffffff;
  box-shadow: 0 0 0 1px var(--ctp-surface0);
  cursor: grab;
  user-select: none;
  touch-action: none;
}

.gost-paper-host.panning {
  cursor: grabbing;
}

.gost-paper-host :deep(svg) {
  display: block;
  width: 100%;
  height: 100%;
  background: #ffffff;
}

.gost-paper-host :deep(.joint-cell) {
  cursor: pointer;
}

.gost-paper-host :deep(.joint-type-gost-node),
.gost-paper-host :deep(.joint-type-gost-terminal),
.gost-paper-host :deep(.joint-type-gost-busrail) {
  cursor: move;
}

.gost-paper-host :deep(.joint-marker-vertex),
.gost-paper-host :deep(.marker-vertex) {
  fill: var(--ctp-blue);
  stroke: #ffffff;
}

.zoom-controls {
  position: absolute;
  left: 30px;
  bottom: 30px;
  display: flex;
  border: 1px solid var(--ctp-surface1);
  background: var(--ctp-mantle);
  z-index: 3;
}

.zoom-controls button {
  min-width: 34px;
  height: 30px;
  padding: 0 9px;
  border: 0;
  border-right: 1px solid var(--ctp-surface1);
  background: var(--ctp-surface0);
  color: var(--ctp-text);
}

.zoom-controls button:last-child {
  border-right: 0;
}

.gost-inspector {
  position: absolute;
  top: 16px;
  right: 16px;
  width: 244px;
  padding: 12px;
  border: 1px solid var(--ctp-surface1);
  background: var(--ctp-mantle);
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.35);
  z-index: 4;
}

.inspector-title {
  margin-bottom: 8px;
  font-size: 12px;
  color: var(--ctp-subtext);
}

.gost-inspector label {
  display: grid;
  gap: 6px;
  color: var(--ctp-subtext);
  font-size: 12px;
}

.gost-inspector input {
  width: 100%;
  height: 34px;
  border: 1px solid var(--ctp-surface1);
  background: var(--ctp-base);
  color: var(--ctp-text);
  padding: 0 8px;
}

.inspector-ports {
  margin-top: 10px;
  max-height: 180px;
  overflow: auto;
  font-size: 12px;
}

.inspector-ports div {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  padding: 3px 0;
  border-bottom: 1px solid var(--ctp-surface0);
}
</style>
