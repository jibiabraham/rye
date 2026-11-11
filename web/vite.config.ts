import path from "node:path"
import tailwindcss from "@tailwindcss/vite"
import react from "@vitejs/plugin-react"
import { defineConfig } from "vitest/config"

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: { "@": path.resolve(import.meta.dirname, "src") },
  },
  build: {
    // One chunk serves the single-user GUI. The limit flags growth in that chunk.
    chunkSizeWarningLimit: 850,
  },
  server: {
    host: "127.0.0.1",
    port: 5173,
    strictPort: true,
    // The API refuses other origins. ssh rye-gui forwards it to localhost:8080. The server listens on loopback
    // only, because the proxy sends every request as that origin.
    proxy: {
      "/api": {
        target: "http://localhost:8080",
        headers: { Host: "localhost:8080", Origin: "http://localhost:8080" },
      },
    },
  },
  test: {
    environment: "jsdom",
    setupFiles: ["src/test-setup.ts"],
    unstubGlobals: true,
    include: ["src/**/*.test.{ts,tsx}"],
  },
})
