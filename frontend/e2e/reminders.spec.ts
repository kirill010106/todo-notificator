import { test, expect } from "@playwright/test";
import { fixture } from "./fixtures";

test("partial dates are explained without a save request", async ({ page }) => {
  await fixture(page);
  let writes = 0;
  page.on("request", (r) => {
    if (r.url().endsWith("/tasks") && r.method() === "POST") writes++;
  });
  await page.goto("/app/tasks?new=1");
  await page.getByLabel("Название", { exact: true }).fill("Черновик");
  await page.getByLabel("Срок задачи: дата", { exact: true }).fill("18.10.20");
  await page
    .getByRole("button", { name: "Создать задачу", exact: true })
    .click();
  await expect(
    page.getByText("Дата выбрана — осталось указать время"),
  ).toBeVisible();
  await page.getByLabel("Срок задачи: время", { exact: true }).fill("18:00");
  await page
    .getByRole("button", { name: "Создать задачу", exact: true })
    .click();
  await expect(
    page.getByText("Укажи дату полностью: ДД.ММ.ГГГГ"),
  ).toBeVisible();
  expect(writes).toBe(0);
  await expect(page.getByLabel("Название", { exact: true })).toHaveValue(
    "Черновик",
  );
});

for (const width of [360, 768, 1440])
  test(`arbitrary reminders at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 1000 });
    await fixture(page);
    await page.goto("/app/tasks?new=1");
    await page.getByLabel("Название", { exact: true }).fill("Два напоминания");
    await page
      .getByLabel("Срок задачи: дата", { exact: true })
      .fill("18.10.2099");
    await page.getByLabel("Срок задачи: время", { exact: true }).fill("18:00");
    await page
      .getByRole("button", { name: "Добавить напоминание", exact: true })
      .click();
    await page.getByLabel("Дни: напоминание 1").fill("1");
    await page.getByLabel("Часы: напоминание 1").fill("3");
    await page.getByLabel("Минуты: напоминание 1").fill("20");
    await page.getByLabel("Как напомнить").selectOption("calendar");
    await expect(
      page.getByLabel("Напоминание 1: дата", { exact: true }),
    ).toHaveValue("17.10.2099");
    await expect(
      page.getByLabel("Напоминание 1: время", { exact: true }),
    ).toHaveValue("14:40");
    await page.getByLabel("Срок задачи: время", { exact: true }).fill("19:00");
    await expect(
      page.getByLabel("Напоминание 1: время", { exact: true }),
    ).toHaveValue("15:40");
    await page
      .getByRole("button", { name: "Добавить напоминание", exact: true })
      .click();
    await expect(
      page.getByRole("link", { name: "Больше — с Premium" }),
    ).toBeVisible();
    await expect(
      page.getByRole("button", { name: "Добавить напоминание", exact: true }),
    ).toHaveCount(0);
    await expect(page.locator("html")).toHaveJSProperty("scrollWidth", width);
    await page.screenshot({
      path: `test-results/reminders-${width}.png`,
      fullPage: true,
    });
    const sent = page.waitForRequest(
      (r) => r.url().endsWith("/tasks") && r.method() === "POST",
    );
    await page
      .getByRole("button", { name: "Создать задачу", exact: true })
      .click();
    expect((await sent).postDataJSON().reminders).toEqual([
      { offset_minutes: 1640 },
      { offset_minutes: 0 },
    ]);
    await expect(page.getByRole("dialog")).toHaveCount(0);
  });

test.describe("DST validation", () => {
  test.use({ timezoneId: "America/New_York" });
  test("rejects a local time in the spring gap", async ({ page }) => {
    await fixture(page);
    let writes = 0;
    page.on("request", (r) => {
      if (r.url().endsWith("/tasks") && r.method() === "POST") writes++;
    });
    await page.goto("/app/tasks?new=1");
    await page.getByLabel("Название", { exact: true }).fill("Переход часов");
    await page
      .getByLabel("Срок задачи: дата", { exact: true })
      .fill("08.03.2026");
    await page.getByLabel("Срок задачи: время", { exact: true }).fill("02:30");
    await page
      .getByRole("button", { name: "Создать задачу", exact: true })
      .click();
    await expect(
      page.getByText(
        "Такой даты или местного времени не существует. Проверь ввод",
      ),
    ).toBeVisible();
    expect(writes).toBe(0);
  });
});
