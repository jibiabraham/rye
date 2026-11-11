import { createMemoryHistory, createRouter } from "@tanstack/react-router"
import { expect, test } from "vitest"

import { routeTree } from "@/router"

// ryed serves the page at / and at every path under /vms/. A path past a known route opens that route.
test.each([
  ["/", "/"],
  ["/vms", "__root__"],
  ["/vms/box", "/vms/$vm"],
  ["/vms/box/sessions/s1", "/vms/$vm/sessions/$session"],
  ["/vms/box/other", "/vms/$vm"],
])("%s opens route %s", async (url, route) => {
  const router = createRouter({
    routeTree,
    history: createMemoryHistory({ initialEntries: [url] }),
  })
  await router.load()
  expect(router.state.matches.at(-1)?.routeId).toBe(route)
})
