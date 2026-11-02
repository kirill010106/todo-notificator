import { beforeEach, describe, expect, it } from "vitest";
import { actionNotice, rewardLabel } from "./commands";
import type { ActionResponse } from "./types";

const result = (overrides: Partial<ActionResponse> = {}) => ({
  operation_id: "finish-1",
  rewards: { task_xp: 10, pomodoro_xp: 20, coins: 15, achievement_xp: 0, quest_xp: 0, penalty_xp: 0 },
  stats: { level: 2 },
  level_up: true,
  unlocked_achievements: [{ code: "first_step", title: "Первый шаг", description: "Завершить первую задачу" }],
  ...overrides,
} as ActionResponse);

beforeEach(() => localStorage.clear());

describe("server reward notifications", () => {
  it("shows actual totals and task/session breakdown", () => {
    expect(rewardLabel(result())).toBe("+30 XP, +15 монет (сессия: +20 XP; задача: +10 XP)");
    const text = actionNotice(1, result(), "Сессия завершена.");
    expect(text).toContain("Новый уровень: 2");
    expect(text).toContain("Достижение «Первый шаг»: Завершить первую задачу");
  });
  it("persists deduplication across repeated results and keeps accounts separate", () => {
    expect(actionNotice(1, result(), "Готово.")).not.toBe("");
    expect(actionNotice(1, JSON.parse(JSON.stringify(result())), "Готово.")).toBe("");
    expect(actionNotice(2, result(), "Готово.")).not.toBe("");
  });
  it("never invents rewards for an uncounted session or an old bonus", () => {
    expect(rewardLabel(result({ rewards: { task_xp: 0, pomodoro_xp: 0, coins: 0, achievement_xp: 25, quest_xp: 50, penalty_xp: 30 } }))).toBe("");
  });
});
