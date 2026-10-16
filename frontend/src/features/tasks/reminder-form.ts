import type { TaskReminder } from "../../lib/types";

export type ReminderDraft = {
  key: string;
  original?: TaskReminder;
  mode: "offset" | "calendar";
  days: string;
  hours: string;
  minutes: string;
  date: string;
  time: string;
  legacy: boolean;
};
export type FormErrors = Record<string, string>;

export function splitDate(value?: string | null) {
  if (!value) return { date: "", time: "" };
  const d = new Date(value);
  const pad = (n: number) => String(n).padStart(2, "0");
  return {
    date: `${pad(d.getDate())}.${pad(d.getMonth() + 1)}.${d.getFullYear()}`,
    time: `${pad(d.getHours())}:${pad(d.getMinutes())}`,
  };
}
// Strict local round trip rejects impossible dates and DST gaps instead of
// allowing JavaScript to normalize them into a different time.
export function parseDate(
  date: string,
  time: string,
): { value?: Date; error?: string } {
  if (!date && !time) return {};
  if (!date) return { error: "Время указано — осталось выбрать дату" };
  if (!time) return { error: "Дата выбрана — осталось указать время" };
  const parts = /^(\d{2})\.(\d{2})\.(\d{4})$/.exec(date);
  const clock = /^(\d{2}):(\d{2})$/.exec(time);
  if (!parts) return { error: "Укажи дату полностью: ДД.ММ.ГГГГ" };
  if (!clock) return { error: "Укажи время полностью: ЧЧ:ММ" };
  const [, day, month, year] = parts.map(Number),
    [, hour, minute] = clock.map(Number);
  const d = new Date(year, month - 1, day, hour, minute);
  if (
    year < 1000 ||
    year > 9999 ||
    d.getFullYear() !== year ||
    d.getMonth() !== month - 1 ||
    d.getDate() !== day ||
    d.getHours() !== hour ||
    d.getMinutes() !== minute
  )
    return {
      error: "Такой даты или местного времени не существует. Проверь ввод",
    };
  return { value: d };
}
export function newDraft(reminder?: TaskReminder): ReminderDraft {
  const offset = reminder?.offset_minutes ?? 0;
  return {
    key: reminder ? String(reminder.id) : crypto.randomUUID(),
    original: reminder,
    mode: "offset",
    legacy: reminder?.legacy_absolute ?? false,
    days: String(Math.floor(offset / 1440)),
    hours: String(Math.floor((offset % 1440) / 60)),
    minutes: String(offset % 60),
    ...splitDate(reminder?.scheduled_at),
  };
}
export function offsetFor(
  row: ReminderDraft,
  deadline?: Date,
): { offset?: number; error?: string } {
  if (row.mode === "calendar") {
    const parsed = parseDate(row.date, row.time);
    if (parsed.error) return { error: parsed.error };
    if (!parsed.value) return { error: "Выбери дату и время напоминания" };
    if (!deadline) return { error: "Сначала укажи срок задачи" };
    const offset = (deadline.getTime() - parsed.value.getTime()) / 60000;
    if (offset < 0)
      return { error: "Напоминание должно быть не позже срока задачи" };
    if (!Number.isSafeInteger(offset) || offset > 52560000)
      return { error: "Проверь интервал до срока" };
    return { offset };
  }
  const values = [row.days, row.hours, row.minutes];
  if (values.some((v) => !/^\d+$/.test(v)))
    return { error: "Укажи целое неотрицательное число в каждом поле" };
  const [days, hours, minutes] = values.map(Number),
    offset = days * 1440 + hours * 60 + minutes;
  if (!Number.isSafeInteger(offset) || offset > 52560000)
    return { error: "Интервал слишком большой — укажи не более 36500 дней" };
  return { offset };
}
export function reminderPayload(
  rows: ReminderDraft[],
  deadline: Date | undefined,
  premium: boolean,
  now = Date.now(),
) {
  const errors: FormErrors = {},
    moments = new Set<number>();
  let changed = false;
  const reminders = rows.map((row, i) => {
    const field = `reminders.${i}`,
      old = row.original;
    if (row.legacy && old) {
      const moment = Date.parse(old.scheduled_at);
      if (moments.has(moment))
        errors[field] = "На это время уже есть напоминание";
      moments.add(moment);
      return { id: old.id, offset_minutes: null };
    }
    const result = offsetFor(row, deadline);
    if (result.error) errors[field] = result.error;
    if (!deadline)
      errors.deadline =
        "Для напоминаний укажи срок или удали связанные напоминания";
    const offset = result.offset ?? 0;
    const moment =
      old?.status === "sent"
        ? Date.parse(old.scheduled_at)
        : deadline
          ? deadline.getTime() - offset * 60000
          : NaN;
    const moved =
      !old ||
      old.legacy_absolute ||
      old.offset_minutes !== offset ||
      (old.status !== "sent" && Date.parse(old.scheduled_at) !== moment);
    if (moved) changed = true;
    if (deadline && moved && Number.isFinite(moment) && moment <= now)
      errors[field] =
        "Это напоминание уже в прошлом. Измени время или удали его";
    if (moments.has(moment))
      errors[field] = "На это время уже есть напоминание";
    moments.add(moment);
    return { ...(old ? { id: old.id } : {}), offset_minutes: offset };
  });
  if (!premium && rows.length > 2 && changed)
    errors.reminders =
      "Удали лишние напоминания или подключи Premium: бесплатно доступно два на задачу";
  return { errors, reminders };
}
