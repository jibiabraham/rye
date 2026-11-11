import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { RouterProvider } from "@tanstack/react-router"
import { StrictMode } from "react"
import { createRoot } from "react-dom/client"

import "@/index.css"
import { router } from "@/router"

// The page follows the system color scheme.
const dark = window.matchMedia("(prefers-color-scheme: dark)")
const applyScheme = () =>
  document.documentElement.classList.toggle("dark", dark.matches)
applyScheme()
dark.addEventListener("change", applyScheme)

const root = document.getElementById("root")
if (!root) throw new Error("no #root element")

// One retry absorbs a blip. A stopped VM or agent-manager fails every retry.
const queryClient = new QueryClient({
  defaultOptions: { queries: { retry: 1 } },
})

createRoot(root).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  </StrictMode>,
)
