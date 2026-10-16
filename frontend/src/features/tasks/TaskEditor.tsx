import { useEffect, useRef, useState } from "react";
import { CalendarDays, Plus, Trash2 } from "lucide-react";
import { Link } from "react-router-dom";
import { useApp } from "../../app/context";
import { Button, Sheet } from "../../components/ui";
import { ApiError, errorText } from "../../lib/api";
import { command } from "../../lib/commands";
import type { Task } from "../../lib/types";
import { dateLabel } from "../../lib/tasks";
import {
  newDraft,
  offsetFor,
  parseDate,
  reminderPayload,
  splitDate,
  type FormErrors,
  type ReminderDraft,
} from "./reminder-form";
import s from "../../styles/App.module.css";
import styles from "./TaskEditor.module.css";

function DateTimeFields({
  label,
  date,
  time,
  onChange,
  error,
  id,
  onBlur,
  disabled = false,
}: {
  label: string;
  date: string;
  time: string;
  onChange: (date: string, time: string) => void;
  error?: string;
  id: string;
  onBlur: () => void;
  disabled?: boolean;
}) {
  const picker = useRef<HTMLInputElement>(null);
  const parsed = parseDate(date, time || "00:00");
  const pickerValue = parsed.value
    ? `${parsed.value.getFullYear()}-${String(parsed.value.getMonth() + 1).padStart(2, "0")}-${String(parsed.value.getDate()).padStart(2, "0")}`
    : "";
  return (
    <fieldset className={styles.dateField} disabled={disabled}>
      <legend>{label}</legend>
      <div className={styles.dateRow}>
        <input
          id={id}
          data-field={id}
          aria-label={`${label}: дата`}
          placeholder="ДД.ММ.ГГГГ"
          inputMode="numeric"
          value={date}
          onChange={(e) => onChange(e.target.value, time)}
          onBlur={onBlur}
          aria-invalid={!!error}
          aria-describedby={error ? `${id}-error` : undefined}
        />
        <button
          type="button"
          className={s.iconButton}
          aria-label={`Календарь: ${label}`}
          onClick={() => picker.current?.showPicker()}
        >
          <CalendarDays size={18} />
        </button>
        <input
          ref={picker}
          type="date"
          tabIndex={-1}
          aria-label={`Выбор даты: ${label}`}
          className={styles.nativePicker}
          value={pickerValue}
          onChange={(e) => {
            const [year, month, day] = e.target.value.split("-");
            onChange(year ? `${day}.${month}.${year}` : "", time);
          }}
        />
        <input
          data-field={id}
          aria-label={`${label}: время`}
          placeholder="ЧЧ:ММ"
          inputMode="numeric"
          value={time}
          onChange={(e) => onChange(date, e.target.value)}
          onBlur={onBlur}
          aria-invalid={!!error}
          aria-describedby={error ? `${id}-error` : undefined}
        />
      </div>
      {error && (
        <p id={`${id}-error`} className={s.error}>
          {error}
        </p>
      )}
    </fieldset>
  );
}

export function TaskEditor({
  task,
  open,
  close,
}: {
  task: Task | null;
  open: boolean;
  close: () => void;
}) {
  const { data, notify, refresh } = useApp();
  const form = useRef<HTMLFormElement>(null);
  const lastDeadline = useRef<Date | undefined>(undefined);
  const remindersTouched = useRef(false);
  const [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const [title, setTitle] = useState(""),
    [description, setDescription] = useState(""),
    [category, setCategory] = useState("");
  const [date, setDate] = useState(""),
    [time, setTime] = useState("");
  const [rows, setRows] = useState<ReminderDraft[]>([]),
    [errors, setErrors] = useState<FormErrors>({}),
    [touched, setTouched] = useState<Record<string, boolean>>({});
  useEffect(() => {
    if (open) {
      remindersTouched.current = false;
      setTitle(task?.title || "");
      setDescription(task?.description || "");
      setCategory(String(task?.category_id || ""));
      lastDeadline.current = task?.deadline
        ? new Date(task.deadline)
        : undefined;
      const parts = splitDate(task?.deadline);
      setDate(parts.date);
      setTime(parts.time);
      setRows((task?.reminders || []).map(newDraft));
      setError("");
      setErrors({});
      setTouched({});
    }
  }, [task, open]);
  const premium = data.user.is_premium;
  const zone = Intl.DateTimeFormat().resolvedOptions().timeZone;
  const validate = () => {
    const parsed = parseDate(date, time),
      next: FormErrors = {};
    const original = splitDate(task?.deadline);
    if (task?.deadline && date === original.date && time === original.time)
      parsed.value = new Date(task.deadline);
    if (!title.trim()) next.title = "Добавь название задачи";
    if (title.trim().length > 255)
      next.title = "Название должно быть не длиннее 255 символов";
    if (description.length > 2000)
      next.description = "Описание должно быть не длиннее 2000 символов";
    if (parsed.error) next.deadline = parsed.error;
    const result = reminderPayload(rows, parsed.value, premium);
    Object.assign(next, result.errors);
    if (parsed.error) next.deadline = parsed.error;
    return {
      errors: next,
      deadline: parsed.value,
      reminders: result.reminders,
    };
  };
  const blur = (field: string) => {
    setTouched((v) => ({ ...v, [field]: true }));
    setErrors((previous) => ({ ...previous, ...validate().errors }));
  };
  const shown = (field: string) => (touched[field] ? errors[field] : undefined);
  const changeDeadline = (d: string, t: string) => {
    remindersTouched.current = true;
    const old = lastDeadline.current,
      next = parseDate(d, t).value;
    if (old && next)
      setRows((current) =>
        current.map((row) => {
          if (
            row.mode !== "calendar" ||
            row.legacy ||
            row.original?.status === "sent"
          )
            return row;
          const { offset } = offsetFor(row, old);
          return offset === undefined
            ? row
            : {
              ...row,
              ...splitDate(
                new Date(next.getTime() - offset * 60000).toISOString(),
              ),
            };
        }),
      );
    if (next) lastDeadline.current = next;
    setDate(d);
    setTime(t);
    setErrors({});
  };
  const patch = (index: number, value: Partial<ReminderDraft>) => {
    remindersTouched.current = true;
    setRows((current) =>
      current.map((r, i) => (i === index ? { ...r, ...value } : r)),
    );
    setErrors({});
  };
  const save = async (e: React.FormEvent) => {
    e.preventDefault();
    if (busy) return;
    const result = validate();
    setErrors(result.errors);
    setTouched(
      Object.fromEntries(Object.keys(result.errors).map((k) => [k, true])),
    );
    if (Object.keys(result.errors).length) {
      form.current
        ?.querySelector<HTMLElement>(
          `[data-field="${Object.keys(result.errors)[0]}"]`,
        )
        ?.focus();
      return;
    }
    setBusy(true);
    setError("");
    try {
      const payload = {
        title: title.trim(),
        description,
        category_id: category ? Number(category) : null,
        deadline: result.deadline?.toISOString() ?? null,
        ...(!task || remindersTouched.current
          ? { reminders: result.reminders }
          : {}),
      };
      await command(
        data.user.id,
        task ? `/tasks/${task.id}` : "/tasks",
        payload,
        task ? "PATCH" : "POST",
      );
      notify(
        task ? "Изменения сохранены." : "Задача добавлена. Первый шаг сделан.",
      );
      close();
      await refresh();
    } catch (e) {
      setError(errorText(e));
      if (
        e instanceof ApiError &&
        e.data.field_errors &&
        typeof e.data.field_errors === "object"
      ) {
        const fields = Object.fromEntries(
          Object.entries(e.data.field_errors).filter(
            (entry): entry is [string, string] => typeof entry[1] === "string",
          ),
        );
        setErrors(fields);
        setTouched(
          Object.fromEntries(Object.keys(fields).map((k) => [k, true])),
        );
        form.current
          ?.querySelector<HTMLElement>(
            `[data-field="${Object.keys(fields)[0]}"]`,
          )
          ?.focus();
      }
    } finally {
      setBusy(false);
    }
  };
  return (
    <Sheet
      open={open}
      onClose={() => {
        if (!busy) close();
      }}
      title={task ? "Детали задачи" : "Новая задача"}
      description="Освободи голову — запиши то, что нужно сделать."
    >
      <form ref={form} className={s.editor} noValidate onSubmit={save}>
        <label className={s.field}>
          Название
          <input
            autoFocus
            data-field="title"
            value={title}
            onChange={(e) => {
              setTitle(e.target.value);
              setErrors({});
            }}
            onBlur={() => blur("title")}
            aria-invalid={!!shown("title")}
            aria-describedby={shown("title") ? "title-error" : undefined}
          />
        </label>
        {shown("title") && (
          <p id="title-error" className={s.error}>
            {shown("title")}
          </p>
        )}
        <label className={s.field}>
          Описание <span className={s.muted}>необязательно</span>
          <textarea
            data-field="description"
            rows={4}
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            onBlur={() => blur("description")}
            aria-invalid={!!shown("description")}
            aria-describedby={
              shown("description") ? "description-error" : undefined
            }
          />
        </label>
        {shown("description") && (
          <p id="description-error" className={s.error}>
            {shown("description")}
          </p>
        )}
        <label className={s.field}>
          Категория
          <select
            value={category}
            onChange={(e) => setCategory(e.target.value)}
          >
            <option value="">Без категории</option>
            {data.categories.map((item) => (
              <option key={item.id} value={item.id}>
                {item.name}
              </option>
            ))}
          </select>
        </label>
        <DateTimeFields
          id="deadline"
          label="Срок задачи"
          date={date}
          time={time}
          onChange={changeDeadline}
          error={shown("deadline")}
          onBlur={() => blur("deadline")}
        />
        {(date || time) && (
          <button
            type="button"
            className={styles.textButton}
            onClick={() => changeDeadline("", "")}
          >
            Убрать срок
          </button>
        )}
        <p className={s.muted}>
          Часовой пояс: {zone}. Один день интервала — 24 часа.
        </p>
        <section className={styles.reminders} aria-label="Напоминания по email">
          <h3>Напоминания по email</h3>
          <p className={s.muted}>
            {premium
              ? `Напоминания: ${rows.length}, без тарифного ограничения`
              : `Напоминания: ${rows.length} из 2`}
          </p>
          {rows.map((row, i) => {
            const field = `reminders.${i}`,
              deadline = parseDate(date, time).value,
              { offset } = offsetFor(row, deadline);
            const fixed = row.legacy || row.original?.status === "sent";
            const preview = fixed
              ? row.original?.scheduled_at
              : deadline && offset !== undefined
                ? new Date(deadline.getTime() - offset * 60000).toISOString()
                : undefined;
            return (
              <fieldset
                key={row.key}
                className={styles.reminder}
                data-field={field}
                tabIndex={-1}
                aria-invalid={!!shown(field)}
                aria-describedby={
                  shown(field) ? `reminder-${row.key}-error` : undefined
                }
              >
                <legend>Напоминание {i + 1}</legend>
                <div className={styles.rowHeader}>
                  <span>
                    {row.original?.status === "sent"
                      ? "Отправлено"
                      : row.original?.status === "failed"
                        ? "Не удалось отправить после пяти попыток"
                        : "По email"}
                  </span>
                  <button
                    type="button"
                    className={s.iconButton}
                    aria-label={`Удалить напоминание ${i + 1}`}
                    onClick={() => {
                      remindersTouched.current = true;
                      setRows((current) =>
                        current.filter((_, index) => index !== i),
                      );
                      setErrors({});
                    }}
                  >
                    <Trash2 size={17} />
                  </button>
                </div>
                {row.legacy ? (
                  <>
                    <p>Старое напоминание на отдельную дату</p>
                    {row.original?.status !== "sent" && (
                      <button
                        className={styles.textButton}
                        type="button"
                        onClick={() =>
                          patch(i, { legacy: false, mode: "offset" })
                        }
                      >
                        Привязать к сроку
                      </button>
                    )}
                  </>
                ) : row.original?.status !== "sent" ? (
                  <>
                    <label className={s.field}>
                      Как напомнить
                      <select
                        value={row.mode}
                        onChange={(e) => {
                          const mode = e.target.value as ReminderDraft["mode"];
                          const parsed = parseDate(date, time).value,
                            existing = offsetFor(row, parsed).offset;
                          patch(i, {
                            mode,
                            ...(mode === "calendar" &&
                              parsed &&
                              existing !== undefined
                              ? splitDate(
                                new Date(
                                  parsed.getTime() - existing * 60000,
                                ).toISOString(),
                              )
                              : {}),
                            ...(mode === "offset" && existing !== undefined
                              ? {
                                days: String(Math.floor(existing / 1440)),
                                hours: String(
                                  Math.floor((existing % 1440) / 60),
                                ),
                                minutes: String(existing % 60),
                              }
                              : {}),
                          });
                        }}
                      >
                        <option value="offset">За … до срока</option>
                        <option value="calendar">Выбрать дату и время</option>
                      </select>
                    </label>
                    {row.mode === "offset" ? (
                      <div className={styles.offsetRow}>
                        {(
                          [
                            ["days", "Дни"],
                            ["hours", "Часы"],
                            ["minutes", "Минуты"],
                          ] as const
                        ).map(([key, label]) => (
                          <label className={s.field} key={key}>
                            {label}
                            <input
                              aria-label={`${label}: напоминание ${i + 1}`}
                              inputMode="numeric"
                              value={row[key]}
                              onChange={(e) =>
                                patch(i, { [key]: e.target.value })
                              }
                              onBlur={() => blur(field)}
                              aria-invalid={!!shown(field)}
                              aria-describedby={
                                shown(field)
                                  ? `reminder-${row.key}-error`
                                  : undefined
                              }
                            />
                          </label>
                        ))}
                      </div>
                    ) : (
                      <DateTimeFields
                        id={`reminder-date-${row.key}`}
                        label={`Напоминание ${i + 1}`}
                        date={row.date}
                        time={row.time}
                        error={shown(field)}
                        onChange={(d, t) => patch(i, { date: d, time: t })}
                        onBlur={() => blur(field)}
                      />
                    )}
                  </>
                ) : null}
                {preview && (
                  <p className={s.muted}>
                    {row.original?.status === "sent"
                      ? "Было запланировано"
                      : "Письмо придёт"}
                    : {dateLabel(preview)}
                  </p>
                )}
                {row.original?.status === "sent" && (
                  <p className={s.muted}>
                    {row.original.sent_at
                      ? `Отправлено: ${dateLabel(row.original.sent_at)}`
                      : "Точное время старой отправки неизвестно"}
                    . Повторно отправляться не будет.
                  </p>
                )}
                {shown(field) && (
                  <p id={`reminder-${row.key}-error`} className={s.error}>
                    {shown(field)}
                  </p>
                )}
              </fieldset>
            );
          })}
          {shown("reminders") && (
            <p className={s.error} role="alert">
              {shown("reminders")}
            </p>
          )}
          {premium || rows.length < 2 ? (
            <Button
              quiet
              type="button"
              onClick={() => {
                remindersTouched.current = true;
                setRows((current) => [...current, newDraft()]);
              }}
            >
              <Plus size={17} />
              Добавить напоминание
            </Button>
          ) : (
            <p className={s.notice}>
              До двух напоминаний на задачу бесплатно.{" "}
              <Link to="/app/account">Больше — с Premium</Link>
            </p>
          )}
          {!data.user.is_verified && rows.length > 0 && (
            <p className={s.notice}>
              Для отправки писем подтверди email в аккаунте.
            </p>
          )}
        </section>
        {error && (
          <p className={s.error} role="alert">
            {error}
          </p>
        )}
        <Button type="submit" disabled={busy}>
          {busy
            ? "Сохраняем…"
            : task
              ? "Сохранить изменения"
              : "Создать задачу"}
        </Button>
      </form>
    </Sheet>
  );
}
