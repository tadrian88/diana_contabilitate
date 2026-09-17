import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), 'VITE_')
  return {
    plugins: [react(), tailwindcss()],
    server: env.VITE_API_PROXY_TARGET ? { proxy: { '/api': env.VITE_API_PROXY_TARGET } } : undefined,
    test: {
      environment: 'jsdom',
      setupFiles: './src/test/setup.ts',
      css: true,
      globals: true,
      exclude: ['e2e/**', 'node_modules/**', 'dist/**'],
    },
  }
})
