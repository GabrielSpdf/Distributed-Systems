import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        configure(proxy) {
          proxy.on('proxyRes', (proxyResponse, request, response) => {
            if (request.url?.split('?')[0] !== '/api/orders/events') return

            proxyResponse.on('close', () => {
              if (!proxyResponse.complete) {
                response.destroy()
              }
            })
          })
        },
      },
      '/healthz': 'http://localhost:8080',
    },
  },
})

