import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    port: 46173,
    proxy: {
      '/api': 'http://localhost:48080',
      '/healthz': 'http://localhost:48080',
      '/readyz': 'http://localhost:48080',
    },
  },
})
