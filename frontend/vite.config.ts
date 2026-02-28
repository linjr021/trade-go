import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import path from 'node:path'

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  build: {
    chunkSizeWarningLimit: 900,
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (!id.includes('node_modules')) return

          if (
            id.includes('/node_modules/react/') ||
            id.includes('/node_modules/react-dom/') ||
            id.includes('/node_modules/scheduler/')
          ) {
            return 'react-vendor'
          }

          if (
            id.includes('/node_modules/@radix-ui/') ||
            id.includes('/node_modules/lucide-react/') ||
            id.includes('/node_modules/class-variance-authority/') ||
            id.includes('/node_modules/tailwind-merge/') ||
            id.includes('/node_modules/clsx/')
          ) {
            return 'ui-vendor'
          }

          if (id.includes('/node_modules/axios/')) {
            return 'net-vendor'
          }

          return 'vendor'
        },
      },
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:8080',
        changeOrigin: true,
      },
    },
  },
})
