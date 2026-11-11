import AxeBuilder from "@axe-core/playwright"
import { expect, test, type Page } from "@playwright/test"

const session = "s1"
const permission = {
  request: "r1",
  tool: "Bash",
  input: { command: "make test" },
  title: "Claude wants to run make test",
}

function sse(events: [string, object][]) {
  return events
    .map(([name, data]) => `event: ${name}\ndata: ${JSON.stringify(data)}\n\n`)
    .join("")
}

/** Checks the page as it stands, in the light and dark schemes, against WCAG 2.2 A and AA. */
async function expectAccessible(page: Page) {
  for (const colorScheme of ["light", "dark"] as const) {
    await page.emulateMedia({ colorScheme })
    // The page follows the scheme by a class on <html>, then colors transition. The check waits for the class and
    // the transitions because axe reads colors as they stand. Spinners animate forever, and scroll-linked animations
    // follow the scroll.
    await expect(page.locator("html")).toHaveClass(
      colorScheme === "dark" ? "dark" : "",
    )
    await page.waitForFunction(
      "document.getAnimations().every((a) => a.effect?.getTiming().iterations === Infinity || !(a.timeline instanceof DocumentTimeline))",
    )
    const { violations } = await new AxeBuilder({ page })
      .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"])
      .analyze()
    expect(
      violations.map(
        (v) =>
          `${colorScheme} ${v.id}: ${v.nodes.map((n) => `${n.target.join(" ")} ${n.failureSummary ?? ""}`).join(", ")}`,
      ),
    ).toEqual([])
  }
  await page.emulateMedia({ colorScheme: "light" })
}

async function mockAPI(page: Page) {
  const answers: unknown[] = []
  await page.route("**/api/vms", (route) =>
    route.fulfill({
      json: [{ name: "box", status: "running", agentManager: true }],
    }),
  )
  await page.route("**/api/vms/box/sessions", (route) =>
    route.fulfill({
      json: {
        sessions: [
          {
            sessionId: session,
            summary: "Fix the tests",
            cwd: "/home/dev",
            lastModified: 1,
          },
        ],
        live: [session],
        states: { [session]: { running: true, pending: 1 } },
      },
    }),
  )
  await page.route(`**/api/vms/box/sessions/${session}/events`, (route) =>
    route.fulfill({
      contentType: "text/event-stream",
      body: sse([
        ["state", { live: true, running: true, pending: [permission] }],
        [
          "history",
          {
            messages: [
              {
                type: "user",
                uuid: "u1",
                message: { content: "Run the tests" },
              },
            ],
          },
        ],
        [
          "message",
          {
            session,
            message: {
              type: "assistant",
              uuid: "a1",
              message: {
                content: [
                  {
                    type: "text",
                    text: "Running them now.\n\n| Suite | Result |\n| --- | --- |\n| unit | pending |",
                  },
                  {
                    type: "tool_use",
                    id: "t1",
                    name: "Bash",
                    input: { command: "make test" },
                  },
                ],
              },
            },
          },
        ],
      ]),
    }),
  )
  await page.route("**/api/vms/box/models", (route) =>
    route.fulfill({
      json: {
        models: [
          {
            value: "default",
            displayName: "Default (recommended)",
            description: "",
            supportedEffortLevels: ["low", "high"],
          },
          {
            value: "haiku",
            resolvedModel: "claude-haiku-5-5",
            displayName: "Haiku 5.5",
            description: "",
            supportedEffortLevels: ["low", "medium", "high"],
          },
        ],
      },
    }),
  )
  await page.route(`**/api/vms/box/sessions/${session}/effort`, (route) => {
    answers.push({ effort: route.request().postDataJSON() as unknown })
    return route.fulfill({ status: 204 })
  })
  await page.route(`**/api/vms/box/sessions/${session}/mode`, (route) => {
    answers.push({ mode: route.request().postDataJSON() as unknown })
    return route.fulfill({ status: 204 })
  })
  await page.route(
    `**/api/vms/box/sessions/${session}/approvals/r1`,
    (route) => {
      answers.push(route.request().postDataJSON())
      return route.fulfill({ status: 204 })
    },
  )
  return answers
}

test("the VM page opens home and is accessible in both schemes", async ({
  page,
}) => {
  await mockAPI(page)
  await page.goto("/")

  await expect(page).toHaveURL(/\/vms\/box$/)
  await expect(page).toHaveTitle("● box · rye")
  await expect(page.getByRole("link", { name: /box/ })).toBeVisible()
  await expectAccessible(page)
})

test("shows a session and answers its approval", async ({ page }) => {
  const answers = await mockAPI(page)
  await page.goto("/")
  await page.getByRole("link", { name: /box/ }).click()
  await page.getByRole("link", { name: /Fix the tests/ }).click()

  // With a mouse, the message box takes focus as the page opens.
  await expect(page.getByLabel("Message", { exact: true })).toBeFocused()
  await expect(page.getByText("Run the tests")).toBeVisible()
  await expect(page.getByText("Running them now.")).toBeVisible()
  await expect(page.getByRole("table")).toContainText("pending")
  await expect(page.getByRole("status")).toContainText(
    "Waiting for your approval",
  )
  await expect(
    page.getByRole("button", { name: /make test.*Running/ }),
  ).toBeVisible()
  const approval = page.getByRole("group", {
    name: "Claude wants to run make test",
  })
  await expect(approval).toBeVisible()
  await expectAccessible(page)
  await approval.getByRole("button", { name: "Allow", exact: true }).click()
  await expect
    .poll(() => answers)
    .toEqual([{ behavior: "allow", updatedInput: { command: "make test" } }])
})

test("answers Claude's questions one at a time", async ({ page }) => {
  const answers = await mockAPI(page)
  const input = {
    questions: [
      {
        question: "Which database?",
        options: [{ label: "Postgres" }, { label: "SQLite" }],
      },
      {
        question: "Which extras?",
        multiSelect: true,
        options: [{ label: "Backups" }, { label: "Metrics" }],
      },
    ],
  }
  await page.route(`**/api/vms/box/sessions/${session}/events`, (route) =>
    route.fulfill({
      contentType: "text/event-stream",
      body: sse([
        [
          "state",
          {
            live: true,
            running: true,
            pending: [{ request: "r1", tool: "AskUserQuestion", input }],
          },
        ],
        ["history", { messages: [] }],
      ]),
    }),
  )
  await page.goto(`/vms/box/sessions/${session}`)
  const card = page.getByRole("group", { name: "Claude has a question" })
  await expect(card).toBeVisible()
  await expectAccessible(page)
  // The first option takes focus as the card arrives, and number keys pick.
  await expect(card.getByRole("radio", { name: "Postgres" })).toBeFocused()
  await page.keyboard.press("2")
  await expect(card.getByRole("radio", { name: "SQLite" })).toBeChecked()
  await card.getByRole("button", { name: "Next" }).click()
  await card.getByRole("checkbox", { name: "Backups" }).check()
  await card.getByLabel("Other answer").last().fill("Alerts")
  await card.getByRole("button", { name: "Answer" }).click()
  await expect
    .poll(() => answers)
    .toEqual([
      {
        behavior: "allow",
        updatedInput: {
          ...input,
          answers: {
            "Which database?": "SQLite",
            "Which extras?": "Backups, Alerts",
          },
        },
      },
    ])
})

test("loads earlier messages when scrolled to the top", async ({ page }) => {
  await mockAPI(page)
  const user = (i: number) => ({
    type: "user",
    uuid: `m${i}`,
    message: { content: `Message ${i}` },
  })
  const all = Array.from({ length: 150 }, (_, i) => user(i))
  await page.route(`**/api/vms/box/sessions/${session}/events`, (route) =>
    route.fulfill({
      contentType: "text/event-stream",
      body: sse([
        ["state", { live: false, pending: [] }],
        ["history", { messages: all.slice(50), more: true }],
      ]),
    }),
  )
  await page.route(
    `**/api/vms/box/sessions/${session}/messages?before=m50`,
    (route) =>
      route.fulfill({ json: { messages: all.slice(0, 50), more: false } }),
  )
  await page.goto("/")
  await page.getByRole("link", { name: /box/ }).click()
  await page.getByRole("link", { name: /Fix the tests/ }).click()
  await expect(page.getByText("Message 149", { exact: true })).toBeVisible()
  // Scroll up with the wheel, as a reader does, until message 50 shows.
  await page.getByText("Message 149", { exact: true }).hover()
  const fifty = page.getByText("Message 50", { exact: true })
  await expect(async () => {
    await page.mouse.wheel(0, -1500)
    await expect(fifty).toBeInViewport({ timeout: 100 })
  }).toPass()
  await expect(page.getByText("Message 0", { exact: true })).toBeAttached()
  await expect(page.getByText("Message 50", { exact: true })).toBeInViewport()
})

test("returning to a session in the same tab returns to the reader's place", async ({
  page,
}) => {
  await mockAPI(page)
  const user = (i: number) => ({
    type: "user",
    uuid: `m${i}`,
    message: { content: `Message ${i}` },
  })
  await page.route(`**/api/vms/box/sessions/${session}/events`, (route) =>
    route.fulfill({
      contentType: "text/event-stream",
      body: sse([
        ["state", { live: false, pending: [] }],
        [
          "history",
          { messages: Array.from({ length: 60 }, (_, i) => user(i)) },
        ],
      ]),
    }),
  )
  await page.goto(`/vms/box/sessions/${session}`)
  await page.getByText("Message 59", { exact: true }).hover()
  const twenty = page.getByText("Message 20", { exact: true })
  await expect(async () => {
    await page.mouse.wheel(0, -400)
    await expect(twenty).toBeInViewport({ timeout: 100 })
  }).toPass()
  await page.getByRole("link", { name: /box/ }).first().click()
  await expect(page).toHaveURL(/\/vms\/box$/)
  await page.getByRole("link", { name: /Fix the tests/ }).click()
  await expect(twenty).toBeInViewport()
  await expect(
    page.getByText("Message 59", { exact: true }),
  ).not.toBeInViewport()
})

test("a link to a message loads it and lands on it", async ({ page }) => {
  await mockAPI(page)
  const user = (i: number) => ({
    type: "user",
    uuid: `m${i}`,
    message: { content: `Message ${i}` },
  })
  const all = Array.from({ length: 150 }, (_, i) => user(i))
  await page.route(`**/api/vms/box/sessions/${session}/events`, (route) =>
    route.fulfill({
      contentType: "text/event-stream",
      body: sse([
        ["state", { live: false, pending: [] }],
        ["history", { messages: all.slice(50), more: true }],
      ]),
    }),
  )
  await page.route(
    (url) => url.pathname.endsWith(`/sessions/${session}/messages`),
    (route) => {
      const url = new URL(route.request().url())
      expect(url.searchParams.get("from")).toBe("m10")
      return route.fulfill({
        json: { messages: all.slice(0, 50), more: false },
      })
    },
  )
  await page.goto(`/vms/box/sessions/${session}#m10`)
  await expect(page.getByText("Message 10", { exact: true })).toBeInViewport()
  await expect(page).toHaveURL(new RegExp(`/sessions/${session}#m10$`))
})

test("a link lands and its highlight clears while the session streams", async ({
  page,
}) => {
  await mockAPI(page)
  const user = (i: number) => ({
    type: "user",
    uuid: `m${i}`,
    message: { content: `Message ${i}` },
  })
  // The stream ends at once and reconnects after 20 ms. Each reconnect sends history and a reply delta.
  await page.route(`**/api/vms/box/sessions/${session}/events`, (route) =>
    route.fulfill({
      contentType: "text/event-stream",
      body:
        "retry: 20\n\n" +
        sse([
          ["state", { live: true, running: true, pending: [] }],
          [
            "history",
            { messages: Array.from({ length: 60 }, (_, i) => user(i)) },
          ],
          [
            "message",
            {
              session,
              message: {
                type: "stream_event",
                event: {
                  type: "content_block_delta",
                  delta: { type: "text_delta", text: "Working " },
                },
              },
            },
          ],
        ]),
    }),
  )
  await page.goto(`/vms/box/sessions/${session}#m10`)
  const ten = page.getByText("Message 10", { exact: true })
  await expect(ten).toBeInViewport()
  await expect(page.locator(".ring-2")).toHaveCount(1)
  await expect(page.locator(".ring-2")).toHaveCount(0, { timeout: 5000 })
})

test("a VM's page starts a session with its first message, and effort changes in the session", async ({
  page,
}) => {
  const answers = await mockAPI(page)
  let created: unknown
  let sent: unknown
  await page.route(`**/api/vms/box/sessions/${session}/messages`, (route) => {
    sent = route.request().postDataJSON()
    return route.fulfill({ status: 202 })
  })
  await page.route("**/api/vms/box/sessions", (route) => {
    if (route.request().method() !== "POST") return route.fallback()
    created = route.request().postDataJSON()
    return route.fulfill({ status: 201, json: { session } })
  })
  await page.goto("/")
  await page.getByRole("link", { name: /box/ }).click()
  await page.getByLabel("Model").click()
  await page.getByRole("option", { name: "Haiku 5.5" }).click()
  await page.getByLabel("Effort").click()
  await expect(page.getByRole("option")).toHaveCount(4)
  await page.getByRole("option", { name: "Medium effort" }).click()
  await page.getByLabel("Permission mode").click()
  await page.getByRole("option", { name: "Plan", exact: true }).click()
  await page.getByLabel("Message", { exact: true }).fill("Fix the tests")
  await page.getByRole("button", { name: "Send" }).click()
  await expect
    .poll(() => created)
    .toMatchObject({ model: "haiku", effort: "medium", mode: "plan" })
  await expect.poll(() => sent).toEqual({ content: "Fix the tests" })

  await expect(page).toHaveURL(new RegExp(`/sessions/${session}$`))
  await page.getByLabel("Effort").click()
  await page.getByRole("option", { name: "Low effort" }).click()
  await expect.poll(() => answers).toContainEqual({ effort: { effort: "low" } })
})

test("a session's mode switches under the message box", async ({ page }) => {
  const answers = await mockAPI(page)
  await page.goto(`/vms/box/sessions/${session}`)
  await page.getByLabel("Permission mode").click()
  await expect(page.getByRole("option")).toHaveText([
    "Manual",
    "Accept edits",
    "Plan",
    "Auto",
  ])
  await page.getByRole("option", { name: "Plan", exact: true }).click()
  await expect.poll(() => answers).toContainEqual({ mode: { mode: "plan" } })
  await page.getByLabel("Permission mode").click()
  await expect(page.getByRole("option")).toHaveText([
    "Manual",
    "Accept edits",
    "Plan",
    "Auto",
  ])
  await page.getByRole("option", { name: "Auto" }).click()
  await expect.poll(() => answers).toContainEqual({ mode: { mode: "auto" } })
  await page.getByLabel("Permission mode").click()
  await page.getByRole("option", { name: "Manual" }).click()
  await expect.poll(() => answers).toContainEqual({ mode: { mode: "default" } })
  await expect(page.getByLabel("Permission mode")).toContainText("Manual")
})

test("session URLs survive a reload and follow back and forward", async ({
  page,
}) => {
  await mockAPI(page)
  await page.goto(`/vms/box/sessions/${session}`)
  await expect(page.getByText("Running them now.")).toBeVisible()
  await expect(page).toHaveTitle("● Fix the tests · box · rye")
  await page.reload()
  await expect(page.getByText("Running them now.")).toBeVisible()
  await page.getByRole("link", { name: "box", exact: true }).first().click()
  await expect(page).toHaveURL(/\/vms\/box$/)
  await page.goBack()
  await expect(page).toHaveURL(new RegExp(`/sessions/${session}$`))
  await expect(page.getByText("Running them now.")).toBeVisible()
})

test("a session unknown to agent-manager shows as missing", async ({
  page,
}) => {
  await mockAPI(page)
  await page.route("**/api/vms/box/sessions/gone/events", (route) =>
    route.fulfill({
      contentType: "text/event-stream",
      body: sse([
        ["state", { exists: false, live: false, pending: [] }],
        ["history", { messages: [] }],
      ]),
    }),
  )
  await page.goto("/vms/box/sessions/gone")
  await expect(page.getByText(/This session isn't on box/)).toBeVisible()
  await expect(page).toHaveTitle("● Not found · box · rye")
  await expect(page.getByLabel("Message")).toHaveCount(0)
  await expectAccessible(page)
})

test("rows and the tab title show what waits on you", async ({ page }) => {
  await mockAPI(page)
  await page.route("**/api/vms", (route) =>
    route.fulfill({
      json: [
        { name: "box", status: "running", agentManager: true },
        { name: "quiet", status: "running", agentManager: true },
      ],
    }),
  )
  await page.route("**/api/vms/quiet/**", (route) =>
    route.fulfill({ json: { sessions: [], live: [], models: [] } }),
  )
  // The approval waits on box while quiet is open.
  await page.goto("/vms/quiet")
  await expect(page).toHaveTitle("● quiet · rye")
  await page.goto("/vms/box")
  const vms = page.getByRole("list", { name: "VMs" })
  const sessions = page.getByRole("list", { name: "Sessions" })
  await expect(vms.getByRole("img", { name: "Waiting for you" })).toBeVisible()
  await expect(
    sessions.getByRole("img", { name: "Waiting for you" }),
  ).toBeVisible()
  await expect(page).toHaveTitle("● box · rye")
  await page.getByRole("link", { name: "New session" }).click()
  await expect(page).toHaveURL(/\/vms\/box$/)
  await expect(page.getByLabel("Working directory")).toHaveValue("")
})

test("loads every earlier message on request", async ({ page }) => {
  await mockAPI(page)
  const user = (i: number) => ({
    type: "user",
    uuid: `m${i}`,
    message: { content: `Message ${i}` },
  })
  const all = Array.from({ length: 300 }, (_, i) => user(i))
  await page.route(`**/api/vms/box/sessions/${session}/events`, (route) =>
    route.fulfill({
      contentType: "text/event-stream",
      body: sse([
        ["state", { live: false, pending: [] }],
        ["history", { messages: all.slice(200), more: true }],
      ]),
    }),
  )
  let asked = ""
  await page.route(`**/api/vms/box/sessions/${session}/messages?*`, (route) => {
    const url = new URL(route.request().url())
    if (url.searchParams.get("all") === "1") asked = url.search
    const before = Number(url.searchParams.get("before")!.slice(1))
    const start = url.searchParams.get("all") ? 0 : Math.max(0, before - 100)
    return route.fulfill({
      json: { messages: all.slice(start, before), more: start > 0 },
    })
  })
  await page.goto(`/vms/box/sessions/${session}`)
  await expect(page.getByText("Message 299", { exact: true })).toBeVisible()
  await page.getByRole("button", { name: "Load all earlier messages" }).click()
  await expect(page.getByText("Message 0", { exact: true })).toBeAttached()
  expect(asked).toContain("all=1")
  await expect(
    page.getByRole("button", { name: "Load all earlier messages" }),
  ).toHaveCount(0)
})

test("an approval in the open session reloads the list and marks its row", async ({
  page,
}) => {
  await mockAPI(page)
  // The list from agent-manager has the approval once the stream has delivered it.
  let lists = 0
  await page.route("**/api/vms/box/sessions", (route) => {
    lists++
    return route.fulfill({
      json: {
        sessions: [
          {
            sessionId: session,
            summary: "Fix the tests",
            cwd: "/home/dev",
            lastModified: 1,
          },
        ],
        live: [session],
        states: lists > 1 ? { [session]: { running: true, pending: 1 } } : {},
      },
    })
  })
  await page.goto(`/vms/box/sessions/${session}`)
  const sessions = page.getByRole("list", { name: "Sessions" })
  await expect(
    sessions.getByRole("img", { name: "Waiting for you" }),
  ).toBeVisible()
  await expect(
    page
      .getByRole("list", { name: "VMs" })
      .getByRole("img", { name: "Waiting for you" }),
  ).toBeVisible()
  await expect(page).toHaveTitle("● Fix the tests · box · rye")
})

test("a VM without agent-manager says how to install it and isn't polled", async ({
  page,
}) => {
  await mockAPI(page)
  await page.route("**/api/vms", (route) =>
    route.fulfill({
      json: [
        { name: "box", status: "running", agentManager: true },
        { name: "bare", status: "running" },
      ],
    }),
  )
  const asked: string[] = []
  page.on("request", (r) => {
    if (r.url().includes("/api/vms/bare/")) asked.push(r.url())
  })
  await page.goto("/vms/bare")
  await expect(
    page.getByText(
      "bare has no agent-manager. Install it with ssh rye agent-manager install bare.",
    ),
  ).toBeVisible()
  await page.getByLabel("Message", { exact: true }).fill("hi")
  await expect(page.getByRole("button", { name: "Send" })).toBeDisabled()
  await page.waitForTimeout(500)
  expect(asked).toEqual([])
})

test.describe("on a touch screen", () => {
  test.use({
    viewport: { width: 390, height: 844 },
    isMobile: true,
    hasTouch: true,
  })

  test("the VM and session list opens as a sheet, and a page focuses its heading", async ({
    page,
  }) => {
    await mockAPI(page)
    await page.goto("/vms/box")
    await expect(
      page.getByRole("heading", { name: "New session on box" }),
    ).toBeVisible()
    await expect(page.getByRole("list", { name: "Sessions" })).toBeHidden()
    const trigger = page.getByRole("button", { name: "VMs and sessions" })
    await trigger.click()
    const sheet = page.getByRole("dialog", { name: "VMs and sessions" })
    await expect(sheet).toBeVisible()
    await expectAccessible(page)
    await sheet.getByRole("link", { name: /Fix the tests/ }).click()
    await expect(sheet).toBeHidden()
    await expect(page).toHaveURL(new RegExp(`/sessions/${session}$`))
    await expect(page.getByText("Running them now.")).toBeVisible()
    await expect(
      page.getByRole("heading", { name: "Fix the tests" }),
    ).toBeFocused()
  })
})

test("moving from one VM to another starts a fresh form and focuses its message box", async ({
  page,
}) => {
  await mockAPI(page)
  await page.route("**/api/vms", (route) =>
    route.fulfill({
      json: [
        { name: "box", status: "running", agentManager: true },
        { name: "other", status: "stopped", agentManager: true },
      ],
    }),
  )
  await page.goto("/vms/box")
  await page.getByLabel("Working directory").fill("/srv/app")
  await page.getByRole("link", { name: /other/ }).click()
  await expect(
    page.getByRole("heading", { name: "New session on other" }),
  ).toBeVisible()
  await expect(page.getByLabel("Message", { exact: true })).toBeFocused()
  await expect(page.getByLabel("Working directory")).toHaveValue("")
})

test("a failed send keeps the message, and the next send clears the error", async ({
  page,
}) => {
  await mockAPI(page)
  let sends = 0
  await page.route(`**/api/vms/box/sessions/${session}/messages`, (route) => {
    if (route.request().method() !== "POST") return route.fallback()
    sends++
    return sends === 1
      ? route.fulfill({
          status: 502,
          json: { detail: "agent-manager in box is unreachable" },
        })
      : route.fulfill({ status: 202 })
  })
  await page.goto(`/vms/box/sessions/${session}`)
  const box = page.getByLabel("Message", { exact: true })
  await box.fill("Deploy it")
  await box.press("Enter")
  const error = page.getByRole("alert").filter({ hasText: "unreachable" })
  await expect(error).toBeVisible()
  await expect(error.getByRole("button", { name: "Retry" })).toHaveCount(0)
  await expect(box).toHaveValue("Deploy it")
  await expect(
    page.getByRole("log").getByText("Deploy it", { exact: true }),
  ).toHaveCount(0)
  await box.press("Enter")
  await expect(error).toBeHidden()
  await expect(box).toHaveValue("")
  expect(sends).toBe(2)
})

test("an unsent message waits in the box after leaving the session", async ({
  page,
}) => {
  await mockAPI(page)
  await page.goto(`/vms/box/sessions/${session}`)
  await page.getByLabel("Message", { exact: true }).fill("Half a thought")
  await page.getByRole("link", { name: /box/ }).first().click()
  await expect(page).toHaveURL(/\/vms\/box$/)
  await page.getByRole("link", { name: /Fix the tests/ }).click()
  await expect(page.getByLabel("Message", { exact: true })).toHaveValue(
    "Half a thought",
  )
})

test("an edit's approval shows the change and names what Always allow allows", async ({
  page,
}) => {
  const answers = await mockAPI(page)
  const permission = {
    request: "r1",
    toolUseID: "t1",
    tool: "Edit",
    input: {
      file_path: "/srv/app.go",
      old_string: "a := 1",
      new_string: "a := 2",
    },
    suggestions: [
      { type: "addRules", behavior: "allow", rules: [{ toolName: "Edit" }] },
    ],
  }
  await page.route(`**/api/vms/box/sessions/${session}/events`, (route) =>
    route.fulfill({
      contentType: "text/event-stream",
      body: sse([
        ["state", { live: true, running: true, pending: [permission] }],
        [
          "history",
          {
            messages: [
              {
                type: "assistant",
                uuid: "a1",
                message: {
                  content: [
                    {
                      type: "tool_use",
                      id: "t1",
                      name: "Edit",
                      input: permission.input,
                    },
                  ],
                },
              },
            ],
          },
        ],
      ]),
    }),
  )
  await page.goto(`/vms/box/sessions/${session}`)
  const card = page.getByRole("group", { name: /Edit/ })
  await expect(card.getByText("a := 2")).toBeVisible()
  await expect(
    card.getByRole("button", { name: "Always allow Edit" }),
  ).toBeVisible()
  await expect(
    page.getByRole("button", { name: /app\.go.*Waiting for approval/ }),
  ).toBeVisible()
  await card.getByRole("button", { name: "Allow", exact: true }).focus()
  await page.keyboard.press("Escape")
  await expect
    .poll(() => answers)
    .toEqual([{ behavior: "deny", message: "The user denied this." }])
})

test("while Claude works, Stop sits beside Send and Esc in the box stops it", async ({
  page,
}) => {
  await mockAPI(page)
  let interrupts = 0
  await page.route(`**/api/vms/box/sessions/${session}/interrupt`, (route) => {
    interrupts++
    return route.fulfill({ status: 202 })
  })
  await page.goto(`/vms/box/sessions/${session}`)
  const box = page.getByLabel("Message", { exact: true })
  await box.fill("Also check the docs")
  await expect(page.getByRole("button", { name: "Stop" })).toBeVisible()
  await expect(page.getByRole("button", { name: "Send" })).toBeEnabled()
  await box.press("Escape")
  await expect.poll(() => interrupts).toBe(1)
  await expect(box).toHaveValue("Also check the docs")
  await expect(page.getByRole("button", { name: "Stop" })).toBeDisabled()
  await expect(page.getByRole("status").first()).toContainText("Stopping")
})

test("Ctrl+Shift+O opens a new session on the open VM", async ({ page }) => {
  await mockAPI(page)
  await page.goto(`/vms/box/sessions/${session}`)
  await expect(page.getByText("Running them now.")).toBeVisible()
  await page.keyboard.press("Control+Shift+O")
  await expect(page).toHaveURL(/\/vms\/box$/)
})

test("a fetch's approval shows the URL it fetches", async ({ page }) => {
  await mockAPI(page)
  const permission = {
    request: "r1",
    tool: "WebFetch",
    input: { url: "https://example.com/docs", prompt: "Summarize" },
  }
  await page.route(`**/api/vms/box/sessions/${session}/events`, (route) =>
    route.fulfill({
      contentType: "text/event-stream",
      body: sse([
        ["state", { live: true, running: true, pending: [permission] }],
      ]),
    }),
  )
  await page.goto(`/vms/box/sessions/${session}`)
  await expect(
    page
      .getByRole("group", { name: /WebFetch/ })
      .getByText("https://example.com/docs"),
  ).toBeVisible()
})

test("a first message that fails to send waits in the new session's box", async ({
  page,
}) => {
  await mockAPI(page)
  await page.route("**/api/vms/box/sessions", (route) =>
    route.request().method() === "POST"
      ? route.fulfill({ status: 201, json: { session } })
      : route.fallback(),
  )
  await page.route(`**/api/vms/box/sessions/${session}/messages`, (route) =>
    route.fulfill({ status: 502, json: { detail: "box is unreachable" } }),
  )
  await page.goto("/vms/box")
  await page.getByLabel("Message", { exact: true }).fill("Fix the tests")
  await page.getByLabel("Message", { exact: true }).press("Enter")
  await expect(page).toHaveURL(new RegExp(`/sessions/${session}$`))
  await expect(page.getByLabel("Message", { exact: true })).toHaveValue(
    "Fix the tests",
  )
  await expect(page.getByRole("alert")).toContainText("box is unreachable")
  await page.goto("/vms/box")
  await expect(page.getByLabel("Message", { exact: true })).toHaveValue("")
})

test("a running turn has no footer, and a finished turn's footer copies its text", async ({
  page,
  context,
}) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"])
  await mockAPI(page)
  await page.goto(`/vms/box/sessions/${session}`)
  await expect(page.getByText("Running them now.")).toBeVisible()
  await expect(page.getByRole("button", { name: "Copy reply" })).toHaveCount(0)

  await page.route(`**/api/vms/box/sessions/${session}/events`, (route) =>
    route.fulfill({
      contentType: "text/event-stream",
      body: sse([
        ["state", { live: true, running: false, pending: [] }],
        [
          "history",
          {
            messages: [
              {
                type: "user",
                uuid: "u1",
                message: { content: "Run the tests" },
              },
              {
                type: "assistant",
                uuid: "a1",
                message: { content: [{ type: "text", text: "All pass." }] },
              },
            ],
          },
        ],
      ]),
    }),
  )
  await page.reload()
  const copy = page.getByRole("button", { name: "Copy reply" })
  await expect(copy).toBeVisible()
  await copy.click()
  await expect
    .poll(() => page.evaluate("navigator.clipboard.readText()"))
    .toBe("All pass.")
  // The person's earlier message keeps its footer until hover or focus.
  await expect(
    page.getByRole("button", { name: "Copy message" }).locator(".."),
  ).toHaveCSS("opacity", "0")
})

test("a session on a stopped VM says how to start the VM, and opens no stream", async ({
  page,
}) => {
  await mockAPI(page)
  await page.route("**/api/vms", (route) =>
    route.fulfill({
      json: [{ name: "box", status: "stopped", agentManager: true }],
    }),
  )
  let streams = 0
  await page.route(`**/api/vms/box/sessions/${session}/events`, (route) => {
    streams++
    return route.abort()
  })
  await page.goto(`/vms/box/sessions/${session}`)
  await expect(
    page.getByText("box is stopped. Start it with ssh rye start box."),
  ).toBeVisible()
  await expect(page.getByLabel("Message", { exact: true })).toHaveCount(0)
  expect(streams).toBe(0)
})

test("a VM page shows Retry when the VM list fails to load", async ({
  page,
}) => {
  await mockAPI(page)
  await page.route("**/api/vms", (route) =>
    route.fulfill({ status: 502, json: { detail: "service VM is down" } }),
  )
  await page.goto("/vms/box")
  const main = page.getByRole("main")
  await expect(main.getByRole("button", { name: "Retry" })).toBeVisible()
  await expect(main.getByRole("heading", { name: "box" })).toBeVisible()
})

test("a session shows the error agent-manager sends when it refuses the stream", async ({
  page,
}) => {
  await mockAPI(page)
  await page.route(`**/api/vms/box/sessions/${session}/events`, (route) =>
    route.fulfill({
      contentType: "text/event-stream",
      body: sse([
        ["agent-manager-error", { id: 1, error: "no transcript for it" }],
      ]),
    }),
  )
  await page.goto(`/vms/box/sessions/${session}`)
  await expect(page.getByText("no transcript for it")).toBeVisible()
})
