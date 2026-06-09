import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { resolve } from 'path'

export default defineConfig({
  plugins: [vue()],
  root: '.',
  server: {
    proxy: {
      '/api': 'http://127.0.0.1:8080'
    }
  },
  resolve: {
    alias: { '@': '/js' }
  },
  build: {
    rollupOptions: {
      input: {
        main: resolve(__dirname, 'index.html'),
        flow: resolve(__dirname, 'flow.html'),
      }
    }
  }
})
