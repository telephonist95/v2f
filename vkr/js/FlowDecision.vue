<script setup>
import { Handle, Position } from '@vue-flow/core'
import { computed } from 'vue'
const props = defineProps({ id: String, data: Object })
const lines = computed(() => props.data.label.split('\n'))
const w = computed(() => Math.max(180, lines.value.reduce((m,l) => Math.max(m, l.length * 10), 0) + 60))
const h = computed(() => Math.max(100, lines.value.length * 20 + 50))
</script>
<template>
  <div class="fd" :style="{ width: w+'px', height: h+'px' }">
    <Handle type="target" :position="Position.Top"    :id="`${id}-t-top`" class="h" />
    <Handle type="target" :position="Position.Left"   :id="`${id}-t-left`" class="h" />
    <Handle type="target" :position="Position.Right"  :id="`${id}-t-right`" class="h" />
    <Handle type="target" :position="Position.Bottom" :id="`${id}-t-bot`" class="h" />
    <svg :width="w" :height="h" :viewBox="`0 0 ${w} ${h}`" class="fd-svg">
      <polygon :points="`${w/2},1 ${w-1},${h/2} ${w/2},${h-1} 1,${h/2}`"
               fill="#fff" stroke="#000" stroke-width="2"/>
      <text :x="w/2" :y="h/2 - (lines.length-1)*9" text-anchor="middle" dominant-baseline="middle"
            font-family="GOST2304 Type B, Arial" font-style="italic" font-size="13" fill="#000">
        <tspan v-for="(l,i) in lines" :key="i" :x="w/2" :dy="i===0 ? 0 : 18">{{l}}</tspan>
      </text>
    </svg>
    <Handle type="source" :position="Position.Bottom" :id="`${id}-s-bot`" class="h" />
    <Handle type="source" :position="Position.Left"   :id="`${id}-s-left`" class="h" />
    <Handle type="source" :position="Position.Right"  :id="`${id}-s-right`" class="h" />
    <Handle type="source" :position="Position.Top"    :id="`${id}-s-top`" class="h" />
  </div>
</template>
<style scoped>
.fd { position:relative; cursor:grab; }
.fd-svg { display:block; }
.fd :deep(.h) { width:8px;height:8px;background:#000;border:2px solid #fff;border-radius:50%;opacity:0; }
.fd:hover :deep(.h) { opacity:.4; }
</style>
