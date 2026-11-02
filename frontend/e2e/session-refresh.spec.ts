import { expect, test, type Page } from "@playwright/test";

const token = (uid: number, n: number) =>
  `x.${Buffer.from(JSON.stringify({ uid, n })).toString("base64")}.x`;
const refresh = (page: Page) =>
  page.evaluate(async () => {
    const api = await import("/src/lib/api.ts");
    try {
      await api.refreshAccess();
      return true;
    } catch {
      return false;
    }
  });

for (const firstTab of [0, 1]) {
  test(`serializes refresh across two tabs, leader ${firstTab}`, async ({
    context,
  }) => {
    await context.route("**/__session_test", (route) =>
      route.fulfill({
        contentType: "text/html",
        body: "<!doctype html><title>Session test</title>",
      }),
    );
    const pages = [await context.newPage(), await context.newPage()];
    await Promise.all(pages.map((page) => page.goto("/__session_test")));
    await pages[0].evaluate(
      ({ access, refresh }) => {
        localStorage.setItem("access_token", access);
        localStorage.setItem("refresh_token", refresh);
      },
      { access: token(7, 1), refresh: "old" },
    );
    let calls = 0;
    let release!: () => void;
    const ready = new Promise<void>((resolve) => {
      release = resolve;
    });
    await context.route("**/api/v1/refresh", async (route) => {
      calls++;
      expect(route.request().postDataJSON()).toEqual({ refresh_token: "old" });
      await ready;
      await route.fulfill({
        json: { access_token: token(7, 2), refresh_token: "rotated" },
      });
    });
    const leader = refresh(pages[firstTab]);
    await expect.poll(() => calls).toBe(1);
    const follower = refresh(pages[1 - firstTab]);
    await expect
      .poll(() =>
        pages[0].evaluate(
          async () => (await navigator.locks.query()).pending?.length,
        ),
      )
      .toBe(1);
    release();
    expect(await Promise.all([leader, follower])).toEqual([true, true]);
    expect(calls).toBe(1);
    expect(
      await pages[0].evaluate(() => localStorage.getItem("refresh_token")),
    ).toBe("rotated");
  });
}

for (const action of ["logout", "switch account"]) {
  test(`does not restore or overwrite credentials after ${action} with two waiting tabs`, async ({
    context,
  }) => {
    await context.route("**/__session_test", (route) =>
      route.fulfill({ contentType: "text/html", body: "<!doctype html>" }),
    );
    const a = await context.newPage(),
      b = await context.newPage();
    await Promise.all([a.goto("/__session_test"), b.goto("/__session_test")]);
    await a.evaluate(
      (access) => {
        localStorage.setItem("access_token", access);
        localStorage.setItem("refresh_token", "old");
      },
      token(7, 1),
    );
    let calls = 0,
      release!: () => void;
    const ready = new Promise<void>((resolve) => {
      release = resolve;
    });
    await context.route("**/api/v1/refresh", async (route) => {
      calls++;
      await ready;
      await route.fulfill({
        json: { access_token: token(7, 2), refresh_token: "stale" },
      });
    });
    const leader = refresh(a);
    await expect.poll(() => calls).toBe(1);
    const follower = refresh(b);
    await expect
      .poll(() =>
        a.evaluate(async () => (await navigator.locks.query()).pending?.length),
      )
      .toBe(1);
    await a.evaluate(
      async ({ change, access }) => {
        const api = await import("/src/lib/api.ts");
        api.endSession();
        if (change)
          api.saveTokens({ access_token: access, refresh_token: "account-b" });
      },
      { change: action === "switch account", access: token(8, 1) },
    );
    release();
    expect(await Promise.all([leader, follower])).toEqual([false, false]);
    expect(calls).toBe(1);
    expect(await a.evaluate(() => localStorage.getItem("refresh_token"))).toBe(
      action === "logout" ? null : "account-b",
    );
  });
}
