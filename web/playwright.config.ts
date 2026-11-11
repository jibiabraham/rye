import { defineConfig, devices } from "@playwright/test"

// Runs against the production build; make gate builds first.
export default defineConfig({
  testDir: "e2e",
  fullyParallel: true,
  forbidOnly: true,
  reporter: "list",
  use: { baseURL: "http://127.0.0.1:4173" },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
  webServer: {
    command: "pnpm exec vite preview --host 127.0.0.1 --port 4173 --strictPort",
    url: "http://127.0.0.1:4173",
    reuseExistingServer: false,
  },
})
