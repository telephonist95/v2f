<script setup>
import { computed } from 'vue'
import { Handle, Position } from '@vue-flow/core'

const props = defineProps({
  id: String,
  data: Object,
})

const kindText = computed(() => {
  switch (props.data.kind) {
    case 'input': return 'ВХ'
    case 'output': return 'ВЫХ'
    case 'register': return 'RG'
    case 'comb': return 'COMB'
    default: return 'LOG'
  }
})
</script>

<template>
  <div class="gost-block" :class="`kind-${data.kind}`">
    <Handle type="target" :position="Position.Left" :id="`${id}-in`" class="handle" />
    <div class="kind">{{ kindText }}</div>
    <div class="label">{{ data.label }}</div>
    <div class="type">{{ data.type }}</div>
    <Handle type="source" :position="Position.Right" :id="`${id}-out`" class="handle" />
  </div>
</template>

<style scoped>
.gost-block {
  min-width: 118px;
  min-height: 58px;
  padding: 8px 10px 7px;
  border: 2px solid #cdd6f4;
  background: #181825;
  color: #cdd6f4;
  font-family: "GOST Type B", Arial, sans-serif;
  text-align: center;
  cursor: grab;
}
.kind {
  font-size: 11px;
  line-height: 1;
  font-weight: 700;
  letter-spacing: 0;
  text-align: left;
}
.label {
  margin-top: 4px;
  font-size: 15px;
  line-height: 1.15;
  font-weight: 700;
  word-break: break-word;
}
.type {
  margin-top: 4px;
  font-size: 11px;
  line-height: 1.1;
  color: #a6adc8;
  word-break: break-word;
}
.kind-input,
.kind-output {
  min-width: 96px;
}
.kind-register {
  border-style: double;
  border-color: #a6e3a1;
}
.kind-input {
  border-color: #89b4fa;
}
.kind-output {
  border-color: #f9e2af;
}
.kind-comb {
  border-color: #cba6f7;
  background: #1e1e2e;
}
.handle {
  width: 8px;
  height: 8px;
  background: #89b4fa;
  border: 2px solid #1e1e2e;
  opacity: 0;
}
.gost-block:hover .handle {
  opacity: 0.45;
}
</style>
