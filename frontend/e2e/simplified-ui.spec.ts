import { test, expect } from "@playwright/test";
import { fixture, tasks } from "./fixtures";

test("simplified workspace at desktop and mobile sizes", async ({ page }) => {
  await fixture(page, true);
  for (const width of [390, 1440]) {
    await page.setViewportSize({ width, height: 1000 });
    for (const theme of ["light", "dark"]) {
      await page.goto("/app/today");
      await expect(page.getByText(tasks[0].title, { exact: true })).toBeVisible();
      await page.evaluate(async (value) => {
        document.documentElement.dataset.theme = value;
        await document.fonts.ready;
      }, theme);
      expect(await page.evaluate(() => document.fonts.check('16px "Manrope Variable"', "Задачи"))).toBe(true);
      await expect(page.locator("html")).toHaveJSProperty("scrollWidth", width);
      await expect(page.getByRole("button", { name: "Новая задача", exact: true })).toBeVisible({ visible: width > 767 });
      await expect(page.getByRole("button", { name: "Создать задачу", exact: true })).toBeVisible({ visible: width <= 767 });
      await page.screenshot({ path: `test-results/simplified-${width}-${theme}.png`, fullPage: true, animations: "disabled" });
    }
  }
  await page.goto("/app/tasks");
  await expect(page.getByText(tasks[0].title, { exact: true })).toBeVisible();
  await page.getByText(tasks[0].title, { exact: true }).evaluate((element) => {
    element.textContent = "Очень длинное название задачи для проверки переноса текста без обрезания и горизонтальной прокрутки ".repeat(3);
  });
  await page.evaluate(() => { document.documentElement.style.fontSize = "200%"; });
  await expect(page.locator("html")).toHaveJSProperty("scrollWidth", 1440);
  await page.screenshot({ path: "test-results/simplified-large-text.png", fullPage: true, animations: "disabled" });
  await page.evaluate(() => { document.documentElement.style.fontSize = ""; });
  await page.setViewportSize({ width: 390, height: 1000 });
  await page.getByRole("button", { name: "Создать задачу", exact: true }).click();
  await expect(page.getByLabel("Название", { exact: true })).toBeVisible();
  await expect(page.locator("html")).toHaveJSProperty("scrollWidth", 390);
  await page.screenshot({ path: "test-results/simplified-editor.png", fullPage: true, animations: "disabled" });
});

test("filters, selection and task actions remain available", async ({ page }) => {
  await fixture(page);
  await page.goto("/app/tasks");
  await expect(page.getByLabel("Сортировка", { exact: true })).toBeHidden();
  await page.getByRole("button", { name: "Фильтры", exact: true }).click();
  await page.getByLabel("Сортировка", { exact: true }).selectOption("deadline");
  await expect(page.getByRole("button", { name: "Фильтры (1)", exact: true })).toBeVisible();
  await expect(page).toHaveURL(/sort=deadline/);
  await page.getByRole("button", { name: "Сбросить фильтры" }).click();
  await expect(page).not.toHaveURL(/sort=/);
  await expect(page.getByLabel(`Выбрать: ${tasks[0].title}`, { exact: true })).toBeHidden();
  await page.getByRole("button", { name: "Выбрать", exact: true }).click();
  await page.getByLabel(`Выбрать: ${tasks[0].title}`, { exact: true }).check();
  const bulk = page.waitForRequest((request) => request.url().endsWith("/tasks/bulk-complete"));
  await page.getByRole("button", { name: "Завершить", exact: true }).click();
  expect((await bulk).postDataJSON()).toEqual({ task_ids: [1] });
  await page.getByRole("button", { name: "Отменить выбор" }).click();
  await page.getByRole("button", { name: `Действия: ${tasks[1].title}` }).click();
  await page.getByRole("menuitem", { name: "Удалить", exact: true }).click();
  await expect(page.getByRole("dialog")).toBeVisible();
  await page.keyboard.press("Escape");
  await page.getByRole("button", { name: `Действия: ${tasks[1].title}` }).click();
  await page.getByRole("menuitem", { name: "Начать фокус", exact: true }).click();
  await expect(page.getByRole("dialog")).toContainText("Начать 25 минут фокуса?");
  await page.keyboard.press("Escape");
  await page.goto("/app/tasks?pick=focus");
  await page.getByRole("button", { name: `Выбрать для фокуса: ${tasks[1].title}` }).click();
  await expect(page).toHaveURL(/\/app\/focus/);
});
