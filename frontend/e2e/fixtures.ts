import { type Page } from "@playwright/test";

const now = new Date();
now.setHours(18, 0, 0, 0);
export const tasks = [
  {
    id: 1,
    title: "Продумать новую идею",
    description: "Собрать первые мысли и выбрать направление",
    status: "pending",
    category_id: 1,
    deadline: now.toISOString(),
  },
  {
    id: 2,
    title: "Подготовить материалы для встречи",
    description: "",
    status: "pending",
    deadline: now.toISOString(),
  },
  {
    id: 3,
    title: "Выделить время для себя",
    description: "Небольшая прогулка без телефона",
    status: "pending",
    category_id: 2,
    deadline: now.toISOString(),
  },
];
const user = {
  id: 1,
  email: "kirill@example.com",
  is_verified: true,
  is_premium: false,
  categories_limit: 3,
  reminders_per_task_limit: 2,
  active_reminders_count: 0,
  categories_count: 2,
};
const gamification = {
  level: 3,
  points: 310,
  current_level_xp: 60,
  next_level_xp: 200,
  progress_percent: 30,
  rank_title: "В своём ритме",
  streak: 7,
  best_streak: 12,
  unlocked_count: 2,
  total_achievements: 6,
  daily_quests: [],
};
export async function fixture(
  page: Page,
  session = false,
  elapsedSeconds = 60,
  completedCount = 0,
  questsReady = false,
  authenticated = true,
) {
  let active = session
    ? {
        id: 9,
        task_id: 1,
        started_at: new Date(Date.now() - elapsedSeconds * 1000).toISOString(),
        duration_minutes: 25,
        breaks_used: 0,
        status: "active",
      }
    : null;
  let phase = active ? "focus" : "idle";
  let breakUntil = 0;
  let count = completedCount;
  const profile = {
    ...gamification,
    daily_chest_claimed: false,
  };
  const wallet = {
    version: 2,
    coins: 50,
    customRewards: [] as { id: string; title: string; price: number }[],
    boosters: {} as Record<string, number>,
    ownedItems: ["default"],
    activeTheme: "default",
    activeSound: "default",
    activeBooster: false,
    imported: true,
  };
  const catalog = [
    {
      id: "ocean",
      title: "Океан",
      kind: "theme",
      price: 40,
      description: "Спокойная синяя палитра",
    },
    { id: "bell", title: "Колокольчик", kind: "sound", price: 30, description: "Лёгкий сигнал завершения" },
  ];
  const currentTasks = tasks.map((task) => ({ ...task }));
  const categories = [
    { id: 1, name: "Работа" },
    { id: 2, name: "Личное" },
  ];
  const focus = () => ({
    cancel_until: active
      ? new Date(Date.parse(active.started_at) + 30000).toISOString()
      : null,
    server_now: new Date().toISOString(),
    phase,
    session: active,
    phase_ends_at:
      phase === "extra_break" || phase === "recovery_break"
        ? new Date(breakUntil).toISOString()
        : active
          ? new Date(Date.parse(active.started_at) + 1500000).toISOString()
          : null,
    remaining_work_seconds: active
      ? Math.max(
          0,
          Math.min(
            1500,
            Math.ceil(
              (Date.parse(active.started_at) + 1500000 - Date.now()) / 1000,
            ),
          ),
        )
      : 0,
    completed_in_cycle: count,
    available_actions: [],
  });
  await page.addInitScript((authenticated) => {
    if (authenticated) {
      localStorage.setItem("access_token", "test-token");
      localStorage.setItem("refresh_token", "test-refresh");
    }
    if (!localStorage.getItem("todo:theme"))
      localStorage.setItem("todo:theme", "light");
  }, authenticated);
  await page.route("**/api/v1/**", async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname.replace("/api/v1", "");
    const method = route.request().method();
    const result = {
      pomodoro_counted: false,
      task_completed: false,
      changed_task_ids: [] as number[],
      skipped_task_ids: [] as number[],
    };
    const rewards = {
      coins: 0,
      task_xp: 0,
      pomodoro_xp: 0,
      achievement_xp: 0,
      quest_xp: 0,
      penalty_xp: 0,
    };
    if (path.endsWith("/cancel")) {
      active = null;
      phase = "idle";
    }
    if (path === "/pomodoros/9/stop") {
      const request = route.request().postDataJSON();
      const counted =
        !!active && Date.now() - Date.parse(active.started_at) >= 24 * 60000;
      active = null;
      phase =
        request.action === "completed" && counted ? "recovery_break" : "idle";
      if (phase === "recovery_break") {
        count = (count + 1) % 4;
        breakUntil = Date.now() + (count === 0 ? 900000 : 300000);
        result.pomodoro_counted = true;
        rewards.pomodoro_xp = 20;
        rewards.coins = 10;
      }
    }
    if (path.endsWith("/pause") && active) {
      active.breaks_used = 1;
      phase = "extra_break";
      breakUntil = Date.now() + 300000;
    }
    if (path === "/pomodoros/start") {
      active = {
        id: 10,
        task_id: route.request().postDataJSON()?.task_id,
        started_at: new Date().toISOString(),
        duration_minutes: 25,
        breaks_used: 0,
        status: "active",
      };
      phase = "focus";
    }
    if (path === "/me/focus/resume") {
      if (!active)
        active = {
          id: 10,
          task_id: 1,
          started_at: new Date().toISOString(),
          duration_minutes: 25,
          breaks_used: 0,
          status: "active",
        };
      phase = "focus";
    }
    if (path === "/me/focus/rest/finish") {
      phase = "idle";
      breakUntil = 0;
    }
    if (path === "/tasks" && method === "POST") {
      const request = route.request().postDataJSON();
      currentTasks.push({ ...request, id: 4, status: "pending" });
    }
    if (path.startsWith("/tasks/") && method === "PATCH") {
      const task = currentTasks.find(
        (task) => task.id === Number(path.split("/").at(-1)),
      );
      if (task) Object.assign(task, route.request().postDataJSON());
    }
    let body: unknown = {
      status: "OK",
      id: 4,
      operation_id: route.request().headers()["idempotency-key"] || "read",
      focus: focus(),
      result,
      rewards,
      stats: { points: 350 },
      unlocked_achievements: [],
    };
    if (path === "/me/shop") body = { wallet, catalog, ledger: [] };
    else if (path === "/me/shop/actions") {
      const req = route.request().postDataJSON();
      if (req.action === "create_personal")
        wallet.customRewards.push({
          id: "reward-1",
          title: req.title,
          price: req.price,
        });
      if (req.action === "buy") {
        const item = catalog.find((i) => i.id === req.item_id)!;
        if (wallet.coins < item.price) {
          await route.fulfill({
            status: 409,
            json: { code: "INSUFFICIENT_COINS", message: "Not enough coins" },
          });
          return;
        }
        wallet.coins -= item.price;
        wallet.ownedItems.push(item.id);
      }
      if (req.action === "redeem_personal") {
        const reward = wallet.customRewards.find((item) => item.id === req.item_id)!;
        if (wallet.coins < reward.price) {
          await route.fulfill({ status: 409, json: { code: "INSUFFICIENT_COINS", message: "Не хватает монет" } });
          return;
        }
        wallet.coins -= reward.price;
      }
      if (req.action === "equip") wallet.activeTheme = req.item_id;
      body = { wallet };
    } else if (path === "/me/bootstrap")
      body = {
        user: { ...user, timezone: "Europe/Moscow" },
        categories,
        tasks: currentTasks,
        pagination: { total: currentTasks.length, limit: 20, offset: 0 },
        gamification: profile,
        stats: { total_pomodoros: 18, total_completed_tasks: 7 },
        active_pomodoro: active,
        focus: focus(),
      };
    else if (path === "/me/focus") body = { focus: focus() };
    else if (path === "/me/today")
      body = {
        server_now: new Date().toISOString(),
        timezone: "Europe/Moscow",
        today: {
          tasks: currentTasks,
          pagination: { limit: 20, offset: 0, total: currentTasks.length },
        },
        overdue: { tasks: [], pagination: { limit: 20, offset: 0, total: 0 } },
      };
    else if (path === "/tasks" && method === "GET")
      body = {
        tasks: currentTasks,
        pagination: { total: currentTasks.length, limit: 100, offset: 0 },
      };
    else if (path === "/pomodoros/active") {
      if (!active) {
        await route.fulfill({
          status: 404,
          json: { status: "Error", error: "session not found" },
        });
        return;
      }
      body = { active_session: active };
    } else if (path === "/me/stats" && method === "PATCH") {
      await route.fulfill({
        status: 405,
        json: { code: "METHOD_NOT_ALLOWED" },
      });
      return;
    } else if (path === "/me/achievements")
      body = {
        achievements: [
          {
            code: "first_step",
            title: "Первый шаг",
            description: "Завершить первую задачу",
            icon: "✦",
            unlocked: true,
            points: 0,
            current: 1, target: 1, archived: false,
          },
          {code: "task_spree", title: "Десятка", description: "Завершить 10 задач", icon: "⚡", unlocked: false, points: 0, current: 7, target: 10, archived: false},
          {code: "night_owl", title: "Полуночник", description: "Историческая награда", icon: "🦉", unlocked: true, points: 0, current: 0, target: 0, archived: true},
        ],
      };
    else if (path === "/me/logs") body = { logs: [] };
    await route.fulfill({ json: body });
  });
}
