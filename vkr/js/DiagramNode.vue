<script setup>
import { Handle, Position } from '@vue-flow/core'

const props = defineProps({
  id: String,
  data: Object,
})
</script>

<template>
  <div class="gost-node">
    <Handle type="target" :position="Position.Top" :id="`${id}-target-top`" class="h" />
    <Handle type="target" :position="Position.Left" :id="`${id}-target-left`" class="h" />
    <Handle type="target" :position="Position.Bottom" :id="`${id}-target-bottom`" class="h" />
    <Handle type="target" :position="Position.Right" :id="`${id}-target-right`" class="h" />

    <div class="gost-text">
      <div v-for="(line, i) in data.title.split('\n')" :key="i" class="gost-line">{{ line }}</div>
    </div>

    <Handle type="source" :position="Position.Top" :id="`${id}-source-top`" class="h" />
    <Handle type="source" :position="Position.Right" :id="`${id}-source-right`" class="h" />
    <Handle type="source" :position="Position.Bottom" :id="`${id}-source-bottom`" class="h" />
    <Handle type="source" :position="Position.Left" :id="`${id}-source-left`" class="h" />
  </div>
</template>

<style scoped>
.gost-node {
  min-width: 160px;
  max-width: 240px;
  padding: 14px 20px;
  border: 2px solid #000;
  background: #fff;
  text-align: center;
  font-family: 'GOST Type B', Arial, sans-serif;
  font-style: italic;
  cursor: grab;
  position: relative;
}

.gost-text {
  line-height: 1.35;
}

.gost-line {
  font-size: 15px;
  font-weight: 600;
  color: #000;
  white-space: nowrap;
}

.gost-node :deep(.h) {
  width: 8px;
  height: 8px;
  background: #000;
  border: 2px solid #fff;
  border-radius: 50%;
  opacity: 0;
  transition: opacity 0.15s;
}

.gost-node:hover :deep(.h) {
  opacity: 0.5;
}
</style>
