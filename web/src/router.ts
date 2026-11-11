import {
  createRootRoute,
  createRoute,
  createRouter,
} from "@tanstack/react-router"

import { HomePage } from "@/routes/home"
import { VMPage } from "@/routes/vm"
import { RootError, RootLayout, RootNotFound } from "@/routes/root"
import { SessionPage } from "@/routes/session"

const rootRoute = createRootRoute({
  component: RootLayout,
  errorComponent: RootError,
  notFoundComponent: RootNotFound,
})
const homeRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/",
  component: HomePage,
})
const vmRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/vms/$vm",
  component: VMPage,
})
const sessionRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/vms/$vm/sessions/$session",
  component: SessionPage,
})

/** The app's route tree. Tests build a router on it with an in-memory history. */
export const routeTree = rootRoute.addChildren([
  homeRoute,
  vmRoute,
  sessionRoute,
])

export const router = createRouter({ routeTree })

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router
  }
  interface HistoryState {
    /** The URL a push came from. Going up returns with Back when it matches the parent. */
    from?: string
    /** The model a new session started with. Its page shows it until a reply names the model. */
    startModel?: string
  }
}
