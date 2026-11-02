import { api, ApiError, json } from "./api";
import type { ActionResponse } from "./types";

// Persist the key before sending. Retrying an uncertain response, including
// after reload, reuses the same operation instead of repeating its side effects.
export async function command<T = ActionResponse>(
  userId: number,
  path: string,
  body?: unknown,
  method = "POST",
): Promise<T> {
  const storageKey = `todo:command:${userId}:${method}:${path}:${json(body ?? null)}`;
  let key = sessionStorage.getItem(storageKey);
  if (!key) {
    key = crypto.randomUUID();
    sessionStorage.setItem(storageKey, key);
  }
  try {
    const response = await api<T>(path, {
      method,
      headers: { "Idempotency-Key": key },
      ...(body !== undefined ? { body: json(body) } : {}),
    });
    sessionStorage.removeItem(storageKey);
    return response;
  } catch (error) {
    if (
      error instanceof ApiError &&
      error.status >= 400 &&
      error.status < 500 &&
      error.status !== 429
    )
      sessionStorage.removeItem(storageKey);
    throw error;
  }
}
export function rewardLabel(result: ActionResponse) {
  if (!result.rewards) return "";
  const { task_xp = 0, pomodoro_xp = 0, coins = 0 } = result.rewards;
  const gain = task_xp + pomodoro_xp;
  const total = [gain ? `+${gain} XP` : "", coins ? `+${coins} монет` : ""].filter(Boolean).join(", ");
  return task_xp && pomodoro_xp
    ? `${total} (сессия: +${pomodoro_xp} XP; задача: +${task_xp} XP)`
    : total;
}

// Notify only from a command result, never from bootstrap/refetch snapshots.
export function actionNotice(userId: number, result: ActionResponse, message: string) {
  const key = `todo:notice:${userId}:${result.operation_id}`;
  if (result.operation_id && localStorage.getItem(key)) return "";
  const parts = [message, rewardLabel(result)];
  if (result.level_up) parts.push(`Новый уровень: ${result.stats?.level}.`);
  for (const item of result.unlocked_achievements || [])
    parts.push(`Достижение «${item.title}»: ${item.description}.`);
  const text = parts.filter(Boolean).join(" ");
  if (text && result.operation_id) localStorage.setItem(key, "1");
  return text;
}
