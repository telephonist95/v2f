<script setup>
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import * as monaco from 'monaco-editor/esm/vs/editor/editor.api'
import EditorWorker from 'monaco-editor/esm/vs/editor/editor.worker?worker'

const props = defineProps({
  modelValue: { type: String, default: '' },
  language: { type: String, default: 'systemverilog' },
  readOnly: { type: Boolean, default: false },
})
const emit = defineEmits(['update:modelValue'])

const host = ref(null)
let editor = null
let internalUpdate = false

self.MonacoEnvironment = {
  getWorker() {
    return new EditorWorker()
  },
}

function registerSystemVerilog() {
  if (monaco.languages.getLanguages().some((l) => l.id === 'systemverilog')) return
  monaco.languages.register({ id: 'systemverilog' })
  monaco.languages.setMonarchTokensProvider('systemverilog', {
    defaultToken: '',
    tokenPostfix: '.sv',
    keywords: [
      'always', 'always_comb', 'always_ff', 'assign', 'begin', 'case', 'default',
      'else', 'end', 'endcase', 'endmodule', 'for', 'function', 'if', 'input',
      'localparam', 'logic', 'module', 'negedge', 'output', 'parameter', 'posedge',
      'reg', 'typedef', 'wire',
    ],
    operators: ['=', '<=', '==', '!=', '&&', '||', '+', '-', '*', '/', '&', '|', '^', '~'],
    tokenizer: {
      root: [
        [/\/\/.*$/, 'comment'],
        [/\/\*/, 'comment', '@comment'],
        [/[a-zA-Z_$][\w$]*/, { cases: { '@keywords': 'keyword', '@default': 'identifier' } }],
        [/\d+'[bdh][0-9a-fA-F_xzXZ]+/, 'number'],
        [/\d+/, 'number'],
        [/[{}()[\];,.]/, 'delimiter'],
        [/[=<>!~?:&|+\-*/^%]+/, 'operator'],
      ],
      comment: [
        [/[^/*]+/, 'comment'],
        [/\*\//, 'comment', '@pop'],
        [/[/*]/, 'comment'],
      ],
    },
  })
}

function registerCatppuccinTheme() {
  monaco.editor.defineTheme('catppuccin-mocha', {
    base: 'vs-dark',
    inherit: true,
    rules: [
      { token: '', foreground: 'cdd6f4', background: '1e1e2e' },
      { token: 'comment', foreground: '6c7086', fontStyle: 'italic' },
      { token: 'keyword', foreground: 'cba6f7', fontStyle: 'bold' },
      { token: 'number', foreground: 'fab387' },
      { token: 'operator', foreground: '89dceb' },
      { token: 'delimiter', foreground: 'bac2de' },
      { token: 'identifier', foreground: 'cdd6f4' },
    ],
    colors: {
      'editor.background': '#1e1e2e',
      'editor.foreground': '#cdd6f4',
      'editorLineNumber.foreground': '#6c7086',
      'editorLineNumber.activeForeground': '#b4befe',
      'editorCursor.foreground': '#f5e0dc',
      'editor.selectionBackground': '#585b70',
      'editor.inactiveSelectionBackground': '#313244',
      'editor.lineHighlightBackground': '#313244',
      'editorIndentGuide.background1': '#313244',
      'editorIndentGuide.activeBackground1': '#45475a',
      'editorWidget.background': '#181825',
      'editorWidget.border': '#45475a',
      'input.background': '#181825',
      'input.foreground': '#cdd6f4',
      'dropdown.background': '#181825',
      'dropdown.foreground': '#cdd6f4',
    },
  })
  // Catppuccin Latte — light counterpart.
  monaco.editor.defineTheme('catppuccin-latte', {
    base: 'vs',
    inherit: true,
    rules: [
      { token: '', foreground: '4c4f69', background: 'eff1f5' },
      { token: 'comment', foreground: '9ca0b0', fontStyle: 'italic' },
      { token: 'keyword', foreground: '8839ef', fontStyle: 'bold' },
      { token: 'number', foreground: 'fe640b' },
      { token: 'operator', foreground: '04a5e5' },
      { token: 'delimiter', foreground: '6c6f85' },
      { token: 'identifier', foreground: '4c4f69' },
    ],
    colors: {
      'editor.background': '#eff1f5',
      'editor.foreground': '#4c4f69',
      'editorLineNumber.foreground': '#9ca0b0',
      'editorLineNumber.activeForeground': '#7287fd',
      'editorCursor.foreground': '#dc8a78',
      'editor.selectionBackground': '#acb0be',
      'editor.inactiveSelectionBackground': '#ccd0da',
      'editor.lineHighlightBackground': '#ccd0da',
      'editorIndentGuide.background1': '#ccd0da',
      'editorIndentGuide.activeBackground1': '#bcc0cc',
      'editorWidget.background': '#e6e9ef',
      'editorWidget.border': '#bcc0cc',
      'input.background': '#e6e9ef',
      'input.foreground': '#4c4f69',
      'dropdown.background': '#e6e9ef',
      'dropdown.foreground': '#4c4f69',
    },
  })
}

function currentMonacoTheme() {
  return document.documentElement.getAttribute('data-theme') === 'light'
    ? 'catppuccin-latte'
    : 'catppuccin-mocha'
}

let themeObserver = null

onMounted(() => {
  registerSystemVerilog()
  registerCatppuccinTheme()
  editor = monaco.editor.create(host.value, {
    value: props.modelValue,
    language: props.language,
    automaticLayout: true,
    minimap: { enabled: false },
    readOnly: props.readOnly,
    fontSize: 14,
    lineHeight: 21,
    tabSize: 4,
    scrollBeyondLastLine: false,
    wordWrap: 'off',
    theme: currentMonacoTheme(),
  })
  editor.onDidChangeModelContent(() => {
    if (internalUpdate) return
    emit('update:modelValue', editor.getValue())
  })
  // React to global theme toggling by watching the <html> data-theme attribute.
  themeObserver = new MutationObserver(() => {
    monaco.editor.setTheme(currentMonacoTheme())
  })
  themeObserver.observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme'] })
})

watch(() => props.modelValue, (value) => {
  if (!editor || value === editor.getValue()) return
  internalUpdate = true
  editor.setValue(value)
  internalUpdate = false
})

watch(() => props.readOnly, (value) => {
  if (editor) editor.updateOptions({ readOnly: value })
})

onBeforeUnmount(() => {
  themeObserver?.disconnect()
  if (editor) editor.dispose()
})
</script>

<template>
  <div ref="host" class="code-editor"></div>
</template>

<style scoped>
.code-editor {
  width: 100%;
  height: 100%;
  min-height: 0;
  border: 0;
}
</style>
