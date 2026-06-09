<script setup>
import { Handle, Position } from '@vue-flow/core'
defineProps({ id: String, data: Object })
</script>
<template>
  <div class="ft">
    <Handle type="target" :position="Position.Top"  :id="`${id}-t-top`" class="h" />
    <Handle type="target" :position="Position.Left" :id="`${id}-t-left`" class="h" />
    <span class="ft-text">{{ data.label }}</span>
    <Handle type="source" :position="Position.Bottom" :id="`${id}-s-bot`" class="h" />
    <Handle type="source" :position="Position.Right"  :id="`${id}-s-right`" class="h" />
  </div>
</template>
<style scoped>
.ft {
  min-width: 120px;
  padding: 10px 30px;
  border: 2px solid #000; background: #fff;
  border-radius: 999px;
  text-align: center;
  font: italic 14px 'GOST Type B', 'GOST2304 Type B', Arial, sans-serif;
  color: #000;
  cursor: grab;
}
.ft :deep(.h) { width:8px;height:8px;background:#000;border:2px solid #fff;border-radius:50%;opacity:0; }
.ft:hover :deep(.h) { opacity:.4; }
</style>
