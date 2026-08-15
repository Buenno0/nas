import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// O build sai direto dentro do pacote Go que o embute no binário.
const backend = 'http://127.0.0.1:8787'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: {
    outDir: '../internal/web/dist',
    emptyOutDir: true,
  },
  server: {
    port: 5173,
    proxy: {
      '/api': { target: backend, changeOrigin: true },
      '/stream': { target: backend, changeOrigin: true },
      '/img': { target: backend, changeOrigin: true },
      '/healthz': { target: backend, changeOrigin: true },
      // A página /ruinas continua no Vite; somente as portas numeradas saem
      // do SPA para receber do servidor o status HTTP e a tela de erro reais.
      '/ruinas/': { target: backend, changeOrigin: true },
    },
  },
})
