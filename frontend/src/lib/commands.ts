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
  const { task_xp, pomodoro_xp, achievement_xp, quest_xp, penalty_xp } =
    result.rewards;
  const gain = task_xp + pomodoro_xp + achievement_xp + quest_xp;
  return [gain ? `+${gain} XP` : "", penalty_xp ? `−${penalty_xp} XP` : ""]
    .filter(Boolean)
    .join(" · ");
}
