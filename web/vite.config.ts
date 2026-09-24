/// <reference types="vitest/config" />
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

// Em producao o Caddy serve o build e repassa /v1 para a API na mesma origem,
// entao nao ha CORS. Em desenvolvimento o proxy do Vite faz o mesmo papel.
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/v1': process.env.FONTECA_API ?? 'http://localhost:8080',
    },
  },
  test: {
    environment: 'node',
  },
})
