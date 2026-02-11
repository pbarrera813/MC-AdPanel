import { defineConfig } from 'vite'
import path from 'path'
import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [
    react(),
    tailwindcss(),
  ],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },

  // File types to support raw imports. Never add .css, .tsx, or .ts files to this.
  assetsInclude: ['**/*.svg', '**/*.csv'],

  // Dev server proxy: forward API calls and WebSocket to Go backend
  server: {
    proxy: {
      '/api': {
        target: 'http://localhost:4010',
        changeOrigin: true,
        ws: true,
      },
    },
  },
})
