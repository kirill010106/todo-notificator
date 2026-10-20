import { test, expect, type Page } from "@playwright/test";
import { fixture } from "./fixtures";

async function accountFixture(
  page: Page,
  initiallyVerified = false,
  authenticated = true,
) {
  await fixture(page, false, 60, 0, false, authenticated);
  let user = {
    id: 1,
    email: "kirill@example.com",
    display_name: "",
    is_verified: initiallyVerified,
    is_premium: false,
    timezone: "Europe/Moscow",
    categories_count: 2,
    active_reminders_count: 0,
    categories_limit: 3,
    reminders_per_task_limit: 2,
  };
  await page.route("**/api/v1/me/bootstrap", async (route) => {
    await route.fulfill({
      json: {
        status: "OK",
        user,
        categories: [],
        tasks: [],
        pagination: { total: 0, limit: 20, offset: 0 },
        focus: {
          phase: "idle",
          server_now: new Date().toISOString(),
          completed_count: 0,
        },
      },
    });
  });
  await page.route("**/api/v1/me/profile", async (route) => {
    user = { ...user, ...route.request().postDataJSON() };
    await route.fulfill({ json: { status: "OK" } });
  });
  await page.route("**/api/v1/me/email", async (route) => {
    const body = route.request().postDataJSON();
    expect(body.current_password).toBe("password123");
    user = { ...user, email: body.email, is_verified: false };
    await route.fulfill({
      json: {
        status: "OK",
        access_token: "changed-access",
        refresh_token: "changed-refresh",
      },
    });
  });
  await page.route("**/api/v1/me/password", async (route) => {
    expect(route.request().postDataJSON()).toEqual({
      current_password: "password123",
      new_password: "newpassword123",
    });
    await route.fulfill({
      json: {
        status: "OK",
        access_token: "password-access",
        refresh_token: "password-refresh",
      },
    });
  });
  await page.route("**/api/v1/verify?token=confirm", async (route) => {
    user = { ...user, is_verified: true };
    await route.fulfill({ json: { status: "OK" } });
  });
  await page.route("**/api/v1/refresh", (route) =>
    route.fulfill({
      json: { access_token: "refreshed", refresh_token: "refresh" },
    }),
  );
  return { getUser: () => user };
}

test("registration signs in, shows a dismissible notice and persists the session", async ({
  page,
}) => {
  await accountFixture(page, false, false);
  await page.route("**/api/v1/register", async (route) => {
    expect(route.request().postDataJSON()).toEqual({
      email: "kirill@example.com",
      password: "password123",
      display_name: "Кирилл",
    });
    await route.fulfill({
      json: {
        status: "OK",
        user_id: 1,
        access_token: "registered-access",
        refresh_token: "registered-refresh",
      },
    });
  });
  await page.goto("/register");
  await page.getByLabel("Отображаемое имя (необязательно)").fill("Кирилл");
  await page.getByLabel("Email", { exact: true }).fill("kirill@example.com");
  await page.getByLabel("Пароль", { exact: true }).fill("password123");
  await page.getByRole("checkbox").check();
  await page.getByRole("button", { name: "Создать аккаунт" }).click();
  await expect(page).toHaveURL(/\/app\/today$/);
  await expect(page.getByRole("dialog")).toContainText("Аккаунт готов");
  await page.getByRole("button", { name: "Продолжить", exact: true }).click();
  await expect(
    page.getByRole("complementary", { name: "Подтверждение почты" }),
  ).toBeVisible();
  await expect
    .poll(() => page.evaluate(() => localStorage.getItem("refresh_token")))
    .toBe("registered-refresh");
  await page.reload();
  await expect(page.getByRole("dialog")).not.toBeVisible();
  await expect(page).toHaveURL(/\/app\/today$/);
});

test("profile name, email and password can be changed independently", async ({
  page,
}) => {
  const account = await accountFixture(page, true);
  await page.goto("/app/account");
  const name = page
    .getByRole("heading", { name: "Отображаемое имя", exact: true })
    .locator("..");
  await name.getByLabel("Имя", { exact: true }).fill("Новое имя");
  await name.getByRole("button", { name: "Сохранить", exact: true }).click();
  await expect(page.getByRole("status")).toContainText("Имя сохранено.");
  await page.reload();
  await expect(name.getByLabel("Имя", { exact: true })).toHaveValue("Новое имя");
  expect(account.getUser().timezone).toBe("Europe/Moscow");
  const email = page
    .getByRole("heading", { name: "Сменить почту" })
    .locator("..");
  await email.getByLabel("Новая почта").fill("new@example.com");
  await email.getByLabel("Текущий пароль для смены почты").fill("password123");
  await email.getByRole("button", { name: "Сохранить", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "new@example.com", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("complementary", { name: "Подтверждение почты" }),
  ).toBeVisible();
  const password = page
    .getByRole("heading", { name: "Сменить пароль" })
    .locator("..");
  await password
    .getByLabel("Текущий пароль", { exact: true })
    .fill("password123");
  await password
    .getByLabel("Новый пароль", { exact: true })
    .fill("newpassword123");
  await password.getByLabel("Повторите новый пароль").fill("different123");
  await password
    .getByRole("button", { name: "Сохранить", exact: true })
    .click();
  await expect(password.getByRole("alert")).toContainText(
    "Пароли не совпадают",
  );
  await password.getByLabel("Повторите новый пароль").fill("newpassword123");
  await password
    .getByRole("button", { name: "Сохранить", exact: true })
    .click();
  await expect
    .poll(() => page.evaluate(() => localStorage.getItem("access_token")))
    .toBe("password-access");
  await expect(
    password.getByLabel("Новый пароль", { exact: true }),
  ).toHaveValue("");
  await page.setViewportSize({ width: 360, height: 900 });
  await expect(page.locator("html")).toHaveJSProperty("scrollWidth", 360);
  await page.evaluate(() => {
    (document.activeElement as HTMLElement)?.blur();
    window.scrollTo(0, 0);
  });
  await page.screenshot({
    path: "test-results/account-mobile.png",
    fullPage: true,
  });
  await page.goto("/?token=confirm");
  await page.getByRole("link", { name: "Продолжить" }).click();
  await expect(
    page.getByRole("complementary", { name: "Подтверждение почты" }),
  ).not.toBeVisible();
});

test("forgotten password request and reset lead back to login", async ({
  page,
}) => {
  await page.route("**/api/v1/password/forgot", async (route) => {
    expect(route.request().postDataJSON()).toEqual({
      email: "user@example.com",
    });
    await route.fulfill({ status: 202, json: { status: "OK" } });
  });
  await page.goto("/login");
  await page.getByRole("link", { name: "Забыли пароль?" }).click();
  await expect(
    page.getByRole("heading", { name: "Восстановление пароля", exact: true }),
  ).toBeVisible();
  await page.getByLabel("Email").fill("user@example.com");
  await page.getByRole("button", { name: "Отправить ссылку" }).click();
  await expect(
    page.getByText(
      "Если аккаунт с такой почтой существует, мы отправим ссылку для восстановления",
    ),
  ).toBeVisible();
  await page.route("**/api/v1/password/reset", async (route) => {
    expect(route.request().postDataJSON()).toEqual({
      token: "secret",
      new_password: "newpassword123",
    });
    await route.fulfill({ json: { status: "OK" } });
  });
  const response = await page.goto("/reset-password?token=secret");
  expect(response?.headers()["referrer-policy"]).toBe("no-referrer");
  await expect(page).toHaveURL(/\/reset-password$/);
  await page.getByLabel("Новый пароль", { exact: true }).fill("newpassword123");
  await page.getByLabel("Повторите новый пароль").fill("newpassword123");
  await page.getByRole("button", { name: "Сохранить пароль" }).click();
  await expect(
    page.getByRole("heading", { name: "Пароль изменён" }),
  ).toBeVisible();
  await page.getByRole("link", { name: "Перейти ко входу" }).click();
  await expect(page).toHaveURL(/\/login$/);
});

test("expired recovery links offer a new email", async ({ page }) => {
  await page.route("**/api/v1/password/reset", (route) =>
    route.fulfill({
      status: 400,
      json: {
        status: "Error",
        code: "INVALID_RESET_TOKEN",
        message: "Ссылка недействительна",
      },
    }),
  );
  await page.goto("/reset-password?token=expired");
  await page.getByLabel("Новый пароль", { exact: true }).fill("newpassword123");
  await page.getByLabel("Повторите новый пароль").fill("newpassword123");
  await page.getByRole("button", { name: "Сохранить пароль" }).click();
  await expect(
    page.getByRole("link", { name: "Запросить новое письмо" }),
  ).toBeVisible();
  await page.getByRole("link", { name: "Запросить новое письмо" }).click();
  await expect(page.getByLabel("Email")).toBeVisible();
});
