import { api } from "./api";
import type { Task, TaskPage } from "./types";

export async function allPending(signal?: AbortSignal): Promise<Task[]> {
  const tasks = new Map<number, Task>();
  let offset = 0;
  for (;;) {
    const page = await api<TaskPage>(
      `/tasks?status=pending&limit=100&offset=${offset}&sort_by=created_at&order=asc`,
      { signal },
    );
    const batch = page.tasks || [];
    batch.forEach((task) => tasks.set(task.id, task));
    offset += batch.length;
    if (offset >= page.pagination.total) break;
    if (!batch.length) throw new Error("Incomplete pagination");
  }
  return [...tasks.values()];
}
export function todayGroups(tasks: Task[], now = new Date()) {
  const start = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  const end = new Date(now.getFullYear(), now.getMonth(), now.getDate() + 1);
  const active = tasks.filter(
    (t) =>
      t.status === "pending" &&
      t.deadline &&
      Number.isFinite(Date.parse(t.deadline)),
  );
  const sort = (a: Task, b: Task) =>
    Date.parse(a.deadline!) - Date.parse(b.deadline!);
  return {
    overdue: active.filter((t) => new Date(t.deadline!) < start).sort(sort),
    today: active
      .filter(
        (t) => new Date(t.deadline!) >= start && new Date(t.deadline!) < end,
      )
      .sort(sort),
  };
}
export const dateLabel = (value?: string | null) =>
  value
    ? new Date(value).toLocaleString("ru-RU", {
        day: "numeric",
        month: "short",
        hour: "2-digit",
        minute: "2-digit",
      })
    : "Без срока";
export function localInput(value?: string | null) {
  if (!value) return "";
  const date = new Date(value);
  return new Date(date.getTime() - date.getTimezoneOffset() * 60000)
    .toISOString()
    .slice(0, 16);
}
