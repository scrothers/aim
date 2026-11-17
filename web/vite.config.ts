import path from 'node:path'
import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { tanstackRouter } from '@tanstack/router-plugin/vite'

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '')
  const target = env.AIM_API_TARGET || 'http://127.0.0.1:7172'
  // Extra hostnames (e.g. this machine's FQDN) the dev server answers to when
  // bound to 0.0.0.0; IPs and localhost are always allowed by Vite.
  const allowedHosts = (env.AIM_DEV_ALLOWED_HOSTS || '')
    .split(',')
    .map((h) => h.trim())
    .filter(Boolean)
  return {
    plugins: [
      tanstackRouter({ target: 'react', autoCodeSplitting: true }),
      react(),
      tailwindcss(),
    ],
    resolve: { alias: { '@': path.resolve(import.meta.dirname, 'src') } },
    server: {
      allowedHosts,
      proxy: {
        '/api': { target, changeOrigin: false },
        '/healthz': { target, changeOrigin: false },
      },
    },
    build: {
      outDir: '../internal/webui/dist',
      emptyOutDir: true,
      chunkSizeWarningLimit: 800,
    },
  }
})
