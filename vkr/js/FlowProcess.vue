<script setup>
import { Handle, Position } from '@vue-flow/core'
defineProps({ id: String, data: Object })
</script>
<template>
  <div class="fp">
    <Handle type="target" :position="Position.Top"    :id="`${id}-t-top`" class="h" />
    <Handle type="target" :position="Position.Left"   :id="`${id}-t-left`" class="h" />
    <Handle type="target" :position="Position.Right"  :id="`${id}-t-right`" class="h" />
    <Handle type="target" :position="Position.Bottom" :id="`${id}-t-bot`" class="h" />
    <div v-for="(l,i) in data.label.split('\n')" :key="i" class="fp-line">{{l}}</div>
    <Handle type="source" :position="Position.Bottom" :id="`${id}-s-bot`" class="h" />
    <Handle type="source" :position="Position.Left"   :id="`${id}-s-left`" class="h" />
    <Handle type="source" :position="Position.Right"  :id="`${id}-s-right`" class="h" />
    <Handle type="source" :position="Position.Top"    :id="`${id}-s-top`" class="h" />
  </div>
</template>
<style scoped>
.fp {
  min-width: 140px; max-width: 240px;
  padding: 10px 18px;
  border: 2px solid #000; background: #fff;
  text-align: center;
  font: italic 14px/1.3 'GOST Type B', 'GOST2304 Type B', Arial, sans-serif;
  cursor: grab;
}
.fp-line { color: #000; }
.fp :deep(.h) { width:8px;height:8px;background:#000;border:2px solid #fff;border-radius:50%;opacity:0; }
.fp:hover :deep(.h) { opacity:.4; }
</style>
