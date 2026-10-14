import { test, expect } from "@playwright/test";
import { fixture, tasks } from "./fixtures";
test.beforeEach(({ page }) => {
  page.on("pageerror", (error) => {
    throw error;
  });
});
for (const width of [360, 768, 1440]) {
  test(`today, editor and focus at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 1000 });
    await fixture(page);
    await page.goto("/app/today");
    await expect(page.getByText(tasks[0].title, { exact: true })).toBeVisible();
    await expect(page.locator("html")).toHaveJSProperty("scrollWidth", width);
    await page.screenshot({
      path: `test-results/today-${width}.png`,
      fullPage: true,
      animations: "disabled",
    });
    await page.getByText(tasks[0].title, { exact: true }).click();
    await expect(page.getByLabel("Название")).toHaveValue(tasks[0].title);
    await page.screenshot({
      path: `test-results/editor-${width}.png`,
      fullPage: true,
      animations: "disabled",
    });
    await page.keyboard.press("Escape");
    await expect(page.getByRole("dialog")).not.toBeVisible();
    await page.goto("/app/focus");
    await expect(
      page.getByRole("heading", { name: "Время для фокуса" }),
    ).toBeVisible();
    await page.screenshot({
      path: `test-results/focus-${width}.png`,
      fullPage: true,
      animations: "disabled",
    });
  });
  test(`dark theme at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 1000 });
    await fixture(page);
    await page.goto("/app/account");
    await page.getByRole("button", { name: "Тёмная", exact: true }).click();
    await page.goto("/app/today");
    await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
    await expect(page.locator("html")).toHaveJSProperty("scrollWidth", width);
    await page.screenshot({
      path: `test-results/dark-${width}.png`,
      fullPage: true,
      animations: "disabled",
    });
  });
}
test("failed finish preserves the active session", async ({ page }) => {
  await fixture(page, true);
  await page.route("**/pomodoros/9/stop", (route) =>
    route.fulfill({
      status: 500,
      json: { error: "Сервер временно недоступен" },
    }),
  );
  await page.goto("/app/focus");
  await page.getByRole("button", { name: "Закончить", exact: true }).click();
  await page.getByRole("button", { name: "Да, задача выполнена" }).click();
  await expect(page.getByRole("dialog").getByRole("alert")).toContainText(
    "Сервер временно недоступен",
  );
  await page.keyboard.press("Escape");
  await page.reload();
  await expect(
    page.getByRole("button", { name: "Закончить", exact: true }),
  ).toBeVisible();
});
test("verification works without authentication", async ({ page }) => {
  await page.route("**/api/v1/verify?token=abc", (route) =>
    route.fulfill({ json: { status: "OK" } }),
  );
  await page.goto("/?token=abc");
  await expect(
    page.getByRole("heading", { name: "Email подтверждён" }),
  ).toBeVisible();
  await expect(page).toHaveURL(/:\d+\/$/);
});
test("creation sends an idempotency key and nullable task fields", async ({
  page,
}) => {
  await fixture(page);
  await page.goto("/app/tasks?new=1");
  await page.getByLabel("Название").fill("Новая задача");
  const sent = page.waitForRequest(
    (request) =>
      request.url().endsWith("/tasks") && request.method() === "POST",
  );
  await page
    .getByRole("button", { name: "Создать задачу", exact: true })
    .click();
  const request = await sent;
  expect(request.postDataJSON()).toMatchObject({
    title: "Новая задача",
    deadline: null,
    reminder_at: null,
    category_id: null,
  });
  expect(request.headers()["idempotency-key"]).toBeTruthy();
  await expect(page.getByRole("dialog")).not.toBeVisible();
});
test("editing clears an existing deadline with null", async ({ page }) => {
  await fixture(page);
  await page.goto("/app/tasks");
  await page.getByText(tasks[0].title, { exact: true }).click();
  await page.getByLabel("Срок").fill("");
  const sent = page.waitForRequest(
    (request) =>
      request.url().endsWith("/tasks/1") && request.method() === "PATCH",
  );
  await page.getByRole("button", { name: "Сохранить изменения" }).click();
  expect((await sent).postDataJSON().deadline).toBeNull();
});
test("search, sorting and bulk completion use the API", async ({ page }) => {
  await fixture(page);
  await page.goto("/app/tasks");
  await page.getByLabel("Поиск задач").fill("встреча");
  const searched = page.waitForRequest(
    (request) =>
      new URL(request.url()).searchParams.get("search") === "встреча",
  );
  await page.getByRole("button", { name: "Искать", exact: true }).click();
  await searched;
  await page.getByLabel("Сортировка").selectOption("deadline");
  await page.getByLabel(`Выбрать: ${tasks[0].title}`, { exact: true }).check();
  const bulk = page.waitForRequest((request) =>
    request.url().endsWith("/tasks/bulk-complete"),
  );
  await page.getByRole("button", { name: "Завершить", exact: true }).click();
  expect((await bulk).postDataJSON()).toEqual({ task_ids: [1] });
});
test("category creation uses the existing contract", async ({ page }) => {
  await fixture(page);
  await page.goto("/app/categories");
  await page.getByRole("button", { name: "Добавить категорию" }).click();
  await page.getByLabel("Название").fill("Учёба");
  const sent = page.waitForRequest(
    (request) =>
      request.url().endsWith("/categories") && request.method() === "POST",
  );
  await page.getByRole("button", { name: "Сохранить", exact: true }).click();
  expect((await sent).postDataJSON()).toEqual({ name: "Учёба" });
});
test("progress and local personal rewards remain available", async ({
  page,
}) => {
  await fixture(page);
  await page.goto("/app/progress");
  await expect(page.getByRole("heading", { name: "Первый шаг" })).toBeVisible();
  await page.goto("/app/rewards");
  await page.getByRole("button", { name: "Своя награда" }).click();
  await page.getByLabel("Что тебя порадует?").fill("Книга");
  await page.getByRole("button", { name: "Добавить награду" }).click();
  await page.reload();
  await expect(page.getByRole("heading", { name: "Книга" })).toBeVisible();
});
test("golden chest is disabled and never patches statistics", async ({
  page,
}) => {
  await fixture(page);
  let writes = 0;
  page.on("request", (request) => {
    if (request.url().endsWith("/me/stats") && request.method() === "PATCH")
      writes++;
  });
  await page.goto("/app/rewards");
  await expect(
    page.getByRole("button", { name: "Пока недоступно" }),
  ).toBeDisabled();
  expect(writes).toBe(0);
});
test("daily chest uses the server claim endpoint", async ({ page }) => {
  await fixture(page, false, 60, 0, true);
  await page.goto("/app/rewards");
  const sent = page.waitForRequest((request) =>
    request.url().endsWith("/me/rewards/daily-chest/claim"),
  );
  await page.getByRole("button", { name: "Забрать 50 XP" }).click();
  expect((await sent).postData()).toBeNull();
  await expect(
    page.getByRole("button", { name: "Сегодня уже получено" }),
  ).toBeDisabled();
});
test("Today uses server totals and requests the next page on demand", async ({
  page,
}) => {
  await fixture(page);
  await page.route("**/api/v1/me/today?**", async (route) => {
    const offset = Number(
      new URL(route.request().url()).searchParams.get("today_offset"),
    );
    const batch = offset
      ? [{ ...tasks[0], id: 21, title: "Задача со второй страницы" }]
      : Array.from({ length: 20 }, (_, i) => ({
          ...tasks[0],
          id: i + 1,
          title: `Задача ${i + 1}`,
        }));
    await route.fulfill({
      json: {
        server_now: new Date().toISOString(),
        timezone: "Europe/Moscow",
        today: { tasks: batch, pagination: { total: 21, offset, limit: 20 } },
        overdue: { tasks: [], pagination: { total: 0, limit: 20, offset: 0 } },
      },
    });
  });
  await page.goto("/app/today");
  await expect(
    page.getByText("Задача со второй страницы", { exact: true }),
  ).not.toBeVisible();
  await page.getByRole("button", { name: "Далее", exact: true }).click();
  await expect(
    page.getByText("Задача со второй страницы", { exact: true }),
  ).toBeVisible();
});
test("Today surfaces a server failure", async ({ page }) => {
  await fixture(page);
  await page.route("**/api/v1/me/today?**", (route) =>
    route.fulfill({ status: 503, json: { error: "Попробуйте позже" } }),
  );
  await page.goto("/app/today");
  await expect(
    page.getByRole("heading", { name: "Не удалось загрузить задачи" }),
  ).toBeVisible();
});
test("idle focus after creating a task has no missing-session error", async ({
  page,
}) => {
  await fixture(page);
  await page.goto("/app/tasks?new=1");
  await page.getByLabel("Название").fill("Без таймера");
  const refreshed = page.waitForResponse((response) =>
    response.url().endsWith("/me/focus"),
  );
  await page
    .getByRole("button", { name: "Создать задачу", exact: true })
    .click();
  expect((await refreshed).status()).toBe(200);
  await expect(
    page.getByText("session not found", { exact: false }),
  ).not.toBeVisible();
});
test("extra break survives reload and resumes the same session", async ({
  page,
}) => {
  await fixture(page, true);
  let starts = 0;
  page.on("request", (request) => {
    if (request.url().endsWith("/pomodoros/start")) starts++;
  });
  await page.goto("/app/focus");
  await page
    .getByRole("button", { name: "Перерыв · 5 минут", exact: true })
    .click();
  await expect(
    page.getByRole("button", { name: "Продолжить работу", exact: true }),
  ).toBeVisible();
  await page.reload();
  await page
    .getByRole("button", { name: "Продолжить работу", exact: true })
    .click();
  await expect(
    page.getByRole("button", { name: "Закончить", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Перерыв · 5 минут", exact: true }),
  ).not.toBeVisible();
  expect(starts).toBe(0);
});
for (const count of [0, 3])
  test(`automatic completion restores server recovery after cycle ${count + 1}`, async ({
    page,
  }) => {
    await fixture(page, true, 1498, count);
    const completed = page.waitForRequest((request) =>
      request.url().endsWith("/pomodoros/9/stop"),
    );
    await page.goto("/app/focus");
    expect((await completed).postDataJSON()).toEqual({
      action: "completed",
      finish_task: false,
    });
    await expect(
      page.getByRole("button", { name: "Продолжить работу", exact: true }),
    ).toBeVisible();
    const restored = page.waitForResponse((response) =>
      response.url().endsWith("/me/bootstrap"),
    );
    await page.reload();
    const snapshot = (await (await restored).json()).focus;
    expect(snapshot.phase).toBe("recovery_break");
    expect(
      Math.round(
        (Date.parse(snapshot.phase_ends_at) - Date.parse(snapshot.server_now)) /
          1000,
      ),
    ).toBeGreaterThan(count === 3 ? 895 : 295);
    await page
      .getByRole("button", { name: "Закончить отдых", exact: true })
      .click();
    await expect(
      page.getByRole("button", { name: "Начать фокус", exact: true }),
    ).toBeVisible();
  });
test("early finish is a guided task outcome", async ({ page }) => {
  await fixture(page, true);
  await page.goto("/app/focus");
  await page.getByRole("button", { name: "Закончить", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Задача уже выполнена?" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Нет, хочу прерваться" }).click();
  await expect(
    page.getByRole("heading", { name: "Нужна передышка?" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Взять перерыв · 5 минут" }).click();
  await expect(page.getByRole("dialog")).not.toBeVisible();
  await expect(
    page.getByRole("button", { name: "Продолжить работу", exact: true }),
  ).toBeVisible();
});
test("dialog traps focus and restores it to its opener", async ({ page }) => {
  await fixture(page);
  await page.goto("/app/tasks");
  const task = page.getByText(tasks[0].title, { exact: true });
  await task.click();
  await expect(page.getByLabel("Название")).toBeFocused();
  await page.keyboard.press("Shift+Tab");
  await expect(
    page.getByRole("button", { name: "Закрыть", exact: true }),
  ).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(task.locator("..")).toBeFocused();
});
test("logout clears authentication", async ({ page }) => {
  await fixture(page);
  await page.goto("/app/account");
  await page.getByRole("button", { name: "Выйти из аккаунта" }).click();
  await page.getByRole("button", { name: "Выйти", exact: true }).click();
  await expect(page).toHaveURL(/\/login$/);
  expect(
    await page.evaluate(() => localStorage.getItem("access_token")),
  ).toBeNull();
});
test("mobile landing has no horizontal overflow", async ({ page }) => {
  await page.setViewportSize({ width: 360, height: 800 });
  await page.goto("/");
  await expect(
    page.getByRole("heading", { name: /Меньше суеты/ }),
  ).toBeVisible();
  await expect(page.locator("html")).toHaveJSProperty("scrollWidth", 360);
});
