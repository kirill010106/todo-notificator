import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useSearchParams } from "react-router-dom";
import {
  Plus,
  Search,
  ArrowUpRight,
  Clock3,
  Check,
  Trash2,
  SlidersHorizontal,
  Circle,
  Bell,
} from "lucide-react";
import { api, errorText, json } from "../../lib/api";
import { command, rewardLabel } from "../../lib/commands";
import { dateLabel, localInput } from "../../lib/tasks";
import type {
  Task,
  TaskPage,
  TodayState,
  ActionResponse,
} from "../../lib/types";
import { useApp } from "../../app/context";
import { useFocus, FocusPanel } from "../focus/Focus";
import { Button, Sheet, Heading, State } from "../../components/ui";
import s from "../../styles/App.module.css";

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
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [deadline, setDeadline] = useState("");
  const [reminder, setReminder] = useState("");
  const [category, setCategory] = useState("");
  useEffect(() => {
    if (open) {
      setTitle(task?.title || "");
      setDescription(task?.description || "");
      setDeadline(localInput(task?.deadline));
      setReminder(localInput(task?.reminder_at));
      setCategory(String(task?.category_id || ""));
      setError("");
    }
  }, [task, open]);
  const save = async (event: React.FormEvent) => {
    event.preventDefault();
    if (!title.trim()) return;
    setBusy(true);
    setError("");
    try {
      await command(
        data.user.id,
        task ? `/tasks/${task.id}` : "/tasks",
        {
          title: title.trim(),
          description,
          deadline: deadline ? new Date(deadline).toISOString() : null,
          reminder_at: reminder ? new Date(reminder).toISOString() : null,
          category_id: category ? Number(category) : null,
        },
        task ? "PATCH" : "POST",
      );
      notify(
        task ? "Изменения сохранены." : "Задача добавлена. Первый шаг сделан.",
      );
      close();
      await refresh();
    } catch (e) {
      setError(errorText(e));
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
      <form className={s.editor} onSubmit={save}>
        <label className={s.field}>
          Название
          <input
            autoFocus
            required
            maxLength={255}
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            placeholder="Что хочется сделать?"
          />
        </label>
        <label className={s.field}>
          Описание <span className={s.muted}>необязательно</span>
          <textarea
            rows={5}
            maxLength={2000}
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="Детали, мысли, первый шаг…"
          />
        </label>
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
        <label className={s.field}>
          Срок
          <input
            type="datetime-local"
            value={deadline}
            onChange={(e) => setDeadline(e.target.value)}
          />
        </label>
        <label className={s.field}>
          Напоминание по email
          <input
            type="datetime-local"
            value={reminder}
            onChange={(e) => setReminder(e.target.value)}
          />
        </label>
        {!data.user.is_verified && reminder && (
          <p className={s.notice}>
            Для писем подтверди свой email в разделе аккаунта.
          </p>
        )}
        <p className={s.muted}>
          Напоминания: {data.user.active_reminders_count} /{" "}
          {data.user.reminders_limit === -1 ? "∞" : data.user.reminders_limit}.
          Время указано в часовом поясе устройства.
        </p>
        {error && (
          <p className={s.error} role="alert">
            {error}
          </p>
        )}
        <Button type="submit" disabled={busy || !title.trim()}>
          {busy
            ? "Сохраняем…"
            : task
              ? "Сохранить изменения"
              : "Создать задачу"}
          <ArrowUpRight size={17} />
        </Button>
      </form>
    </Sheet>
  );
}

export function TasksPage({ today = false }: { today?: boolean }) {
  const { data, notify, refresh, award } = useApp();
  const focus = useFocus();
  const [params, setParams] = useSearchParams();
  const [editor, setEditor] = useState(false);
  const [editing, setEditing] = useState<Task | null>(null);
  const [selected, setSelected] = useState<number[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [deleteIds, setDeleteIds] = useState<number[]>([]);
  const [clock, setClock] = useState(new Date());
  const [search, setSearch] = useState(params.get("search") || "");
  useEffect(() => {
    const timer = setInterval(() => setClock(new Date()), 30_000);
    return () => clearInterval(timer);
  }, []);
  useEffect(() => {
    if (params.get("new") === "1") {
      setEditing(null);
      setEditor(true);
      const next = new URLSearchParams(params);
      next.delete("new");
      setParams(next, { replace: true });
    }
  }, [params, setParams]);
  useEffect(() => {
    setSelected([]);
  }, [params.toString(), today]);
  useEffect(() => {
    setSearch(params.get("search") || "");
  }, [params]);
  const status = params.get("status") || "pending";
  const category = params.get("category") || "";
  const sort = params.get("sort") || "created_at";
  const offset = Math.max(0, Number(params.get("offset")) || 0);
  const queryParams = new URLSearchParams({
    status,
    limit: "20",
    offset: String(offset),
    sort_by: sort,
    order: sort === "deadline" ? "asc" : "desc",
  });
  if (params.get("search")) queryParams.set("search", params.get("search")!);
  if (category) queryParams.set("category_id", category);
  const query = useQuery({
    queryKey: today
      ? ["tasks", "today", data.user.timezone, params.toString()]
      : ["tasks", queryParams.toString()],
    queryFn: async ({ signal }): Promise<TaskPage & { day?: TodayState }> => {
      if (!today) return api<TaskPage>(`/tasks?${queryParams}`, { signal });
      const day = await api<TodayState>(
        `/me/today?limit=20&today_offset=${Math.max(0, Number(params.get("today_offset")) || 0)}&overdue_offset=${Math.max(0, Number(params.get("overdue_offset")) || 0)}`,
        { signal },
      );
      return {
        day,
        tasks: [...day.today.tasks, ...day.overdue.tasks],
        pagination: {
          limit: 20,
          offset: 0,
          total: day.today.pagination.total + day.overdue.pagination.total,
        },
      };
    },
    refetchInterval: today ? 30_000 : false,
  });
  const setFilter = (name: string, value: string) => {
    const next = new URLSearchParams(params);
    value ? next.set(name, value) : next.delete(name);
    next.delete("offset");
    setParams(next);
  };
  const run = async (
    action: () => Promise<unknown>,
    message: string,
    onSuccess?: () => void,
  ) => {
    setBusy(true);
    setError("");
    try {
      const result = (await action()) as ActionResponse | undefined;
      onSuccess?.();
      setSelected([]);
      setDeleteIds([]);
      const label = result?.rewards ? rewardLabel(result) : "";
      notify(label ? `${message} ${label}` : message);
      await refresh();
    } catch (e) {
      setError(errorText(e));
    } finally {
      setBusy(false);
    }
  };
  const toggle = (task: Task) =>
    void run(
      () =>
        command(
          data.user.id,
          `/tasks/${task.id}`,
          { status: task.status === "done" ? "pending" : "done" },
          "PATCH",
        ),
      task.status === "done"
        ? "Задача снова открыта."
        : "Ещё одно дело завершено.",
      () => {
        if (task.status !== "done") award(`task:${task.id}`, 5);
      },
    );
  const create = () => {
    setEditing(null);
    setEditor(true);
  };
  const groups = {
    today: query.data?.day?.today.tasks || [],
    overdue: query.data?.day?.overdue.tasks || [],
  };
  const todayTotal = query.data?.day?.today.pagination.total ?? 0;
  const overdueTotal = query.data?.day?.overdue.pagination.total ?? 0;
  const groupPagination = (kind: "today" | "overdue") => {
    const page = query.data?.day?.[kind].pagination;
    if (!page || page.total <= page.limit) return null;
    return (
      <div className={s.pagination}>
        <Button
          quiet
          disabled={!page.offset}
          onClick={() =>
            setFilter(
              `${kind}_offset`,
              String(Math.max(0, page.offset - page.limit)),
            )
          }
        >
          Назад
        </Button>
        <span>
          {Math.floor(page.offset / page.limit) + 1} /{" "}
          {Math.ceil(page.total / page.limit)}
        </span>
        <Button
          quiet
          disabled={page.offset + page.limit >= page.total}
          onClick={() =>
            setFilter(`${kind}_offset`, String(page.offset + page.limit))
          }
        >
          Далее
        </Button>
      </div>
    );
  };
  const tasks = query.data?.tasks || [];
  const renderTasks = (list: Task[]) => (
    <div className={s.taskList}>
      {list.map((task) => (
        <article
          className={`${s.taskRow} ${task.status === "done" ? s.taskDone : ""}`}
          key={task.id}
        >
          {!today && (
            <input
              type="checkbox"
              className={s.selectCheck}
              aria-label={`Выбрать: ${task.title}`}
              checked={selected.includes(task.id)}
              onChange={(e) =>
                setSelected((ids) =>
                  e.target.checked
                    ? [...ids, task.id]
                    : ids.filter((id) => id !== task.id),
                )
              }
            />
          )}
          <button
            className={s.complete}
            disabled={busy}
            onClick={() => toggle(task)}
            aria-label={`${task.status === "done" ? "Открыть" : "Завершить"}: ${task.title}`}
          >
            {task.status === "done" ? (
              <Check size={17} />
            ) : (
              <Circle size={21} />
            )}
          </button>
          <button
            className={s.taskBody}
            onClick={() => {
              setEditing(task);
              setEditor(true);
            }}
          >
            <strong>{task.title}</strong>
            <span>
              {task.description && (
                <span className={s.taskDescription}>{task.description}</span>
              )}
              <span className={s.taskMeta}>
                {task.category_id && (
                  <span className={s.categoryDot}>
                    {data.categories.find((cat) => cat.id === task.category_id)
                      ?.name || "Категория"}
                  </span>
                )}
                <span>
                  <Clock3 size={12} /> {dateLabel(task.deadline)}
                </span>
                {task.reminder_at && <Bell size={12} />}
                {task.status === "burnt" && (
                  <span className={s.danger}>Сожжена</span>
                )}
              </span>
            </span>
          </button>
          <button
            className={s.iconButton}
            disabled={
              focus.busy || !!focus.session || task.status !== "pending"
            }
            onClick={() => void focus.start(task.id)}
            aria-label={`Начать фокус: ${task.title}`}
            title="Начать фокус"
          >
            <ArrowUpRight size={19} />
          </button>
          <button
            className={s.iconButton}
            disabled={busy}
            onClick={() => setDeleteIds([task.id])}
            aria-label={`Удалить: ${task.title}`}
          >
            <Trash2 size={16} />
          </button>
        </article>
      ))}
    </div>
  );
  return (
    <>
      <Heading
        eyebrow={
          today
            ? clock.toLocaleDateString("ru-RU", {
                timeZone: data.user.timezone || "UTC",
                weekday: "long",
                day: "numeric",
                month: "long",
              })
            : "ВСЁ ПОД КОНТРОЛЕМ"
        }
        title={today ? "Хороший день, чтобы начать." : "Мои задачи"}
        action={
          <Button onClick={create}>
            <Plus size={18} /> Новая задача
          </Button>
        }
      >
        {today
          ? "Не нужно успеть всё. Начни с того, что важно сегодня."
          : "Большие планы складываются из маленьких шагов."}
      </Heading>
      <div className={s.workspace}>
        <div className={s.mainColumn}>
          {today && (
            <div className={s.overview}>
              <div>
                <span className={s.eyebrow}>НА СЕГОДНЯ</span>
                <strong>
                  {query.isSuccess ? todayTotal : "—"}
                  <small> задач</small>
                </strong>
              </div>
              <div>
                <span className={s.eyebrow}>ТРЕБУЮТ ВНИМАНИЯ</span>
                <strong>
                  {query.isSuccess ? overdueTotal : "—"}
                  <small> просрочено</small>
                </strong>
              </div>
              <div>
                <span className={s.eyebrow}>ТВОЯ СЕРИЯ</span>
                <strong>
                  {data.gamification?.streak ?? "—"}
                  <small> дней</small>
                </strong>
              </div>
            </div>
          )}
          {!today && (
            <div className={s.filters}>
              <form
                className={s.search}
                onSubmit={(e) => {
                  e.preventDefault();
                  setFilter("search", search);
                }}
              >
                <Search size={17} />
                <input
                  aria-label="Поиск задач"
                  placeholder="Найти задачу…"
                  value={search}
                  onChange={(e) => setSearch(e.target.value)}
                />
                <button aria-label="Искать" type="submit">
                  <ArrowUpRight size={17} />
                </button>
              </form>
              <select
                aria-label="Категория задач"
                value={category}
                onChange={(e) => setFilter("category", e.target.value)}
              >
                <option value="">Все категории</option>
                {data.categories.map((cat) => (
                  <option key={cat.id} value={cat.id}>
                    {cat.name}
                  </option>
                ))}
              </select>
              <select
                aria-label="Сортировка"
                value={sort}
                onChange={(e) => setFilter("sort", e.target.value)}
              >
                <option value="created_at">Сначала новые</option>
                <option value="deadline">По сроку</option>
              </select>
            </div>
          )}
          {!today && (
            <div className={s.tabs}>
              {[
                ["pending", "В работе"],
                ["done", "Завершённые"],
                ["burnt", "Сожжённые"],
                ["all", "Все"],
              ].map(([value, title]) => (
                <button
                  key={value}
                  className={status === value ? s.tabActive : ""}
                  onClick={() => setFilter("status", value)}
                >
                  {title}
                </button>
              ))}
              <SlidersHorizontal size={16} />
            </div>
          )}
          {error && (
            <p className={s.error} role="alert">
              {error}
            </p>
          )}
          {selected.length > 0 && (
            <div className={s.bulk}>
              <span>Выбрано: {selected.length}</span>
              <Button
                quiet
                disabled={busy}
                onClick={() =>
                  void run(
                    () =>
                      command(data.user.id, "/tasks/bulk-complete", {
                        task_ids: selected,
                      }),
                    "Задачи завершены.",
                  )
                }
              >
                Завершить
              </Button>
              <Button
                quiet
                disabled={busy}
                onClick={() => setDeleteIds(selected)}
              >
                Удалить
              </Button>
            </div>
          )}
          {query.isPending ? (
            <State title="Собираем твои задачи…" loading />
          ) : query.isError ? (
            <State
              title="Не удалось загрузить задачи"
              retry={() => void query.refetch()}
            >
              {errorText(query.error)}
            </State>
          ) : today ? (
            <>
              {overdueTotal > 0 && (
                <section>
                  <div className={s.sectionTitle}>
                    <h2>Требуют внимания</h2>
                    <span className={s.count}>{overdueTotal}</span>
                  </div>
                  {renderTasks(groups.overdue)}
                  {groupPagination("overdue")}
                </section>
              )}
              <section>
                <div className={s.sectionTitle}>
                  <h2>План на сегодня</h2>
                  <span className={s.count}>{todayTotal}</span>
                  <span className={s.muted}>В своём темпе</span>
                </div>
                {groups.today.length ? (
                  renderTasks(groups.today)
                ) : (
                  <State title="Сегодня есть место для нового">
                    Здесь появятся задачи со сроком на сегодня. Все остальные —
                    в разделе «Задачи».
                  </State>
                )}
                {groupPagination("today")}
              </section>
            </>
          ) : tasks.length ? (
            renderTasks(tasks)
          ) : (
            <State title="Здесь пока тихо">
              Добавь задачу или попробуй другие фильтры.
            </State>
          )}
          <button className={s.addRow} onClick={create}>
            <Plus size={18} /> Добавить задачу{" "}
            <span>Освободи место для мыслей</span>
          </button>
          {!today && query.data && query.data.pagination.total > 20 && (
            <div className={s.pagination}>
              <Button
                quiet
                disabled={offset === 0}
                onClick={() => {
                  const next = new URLSearchParams(params);
                  next.set("offset", String(Math.max(0, offset - 20)));
                  setParams(next);
                }}
              >
                Назад
              </Button>
              <span>
                {Math.floor(offset / 20) + 1} /{" "}
                {Math.ceil(query.data.pagination.total / 20)}
              </span>
              <Button
                quiet
                disabled={offset + 20 >= query.data.pagination.total}
                onClick={() => {
                  const next = new URLSearchParams(params);
                  next.set("offset", String(offset + 20));
                  setParams(next);
                }}
              >
                Далее
              </Button>
            </div>
          )}
          {today && (
            <div className={s.quietNote}>
              <LeafMark />
              <div>
                <strong>Прогресс — это движение, а не скорость.</strong>
                <p>
                  Один маленький шаг сегодня — уже больше, чем идеальный план на
                  завтра.
                </p>
              </div>
            </div>
          )}
        </div>
        <aside className={s.rightColumn}>
          <FocusPanel />
          <div className={s.dailyCard}>
            <span className={s.eyebrow}>МАЛЕНЬКИЕ ПОБЕДЫ</span>
            <h3>Твой ежедневный ритм</h3>
            {data.gamification?.daily_quests?.length ? (
              data.gamification.daily_quests.map((quest) => (
                <div className={s.quest} key={quest.code}>
                  <span>
                    {quest.completed ? (
                      <Check size={16} />
                    ) : (
                      <Circle size={16} />
                    )}
                  </span>
                  <div>
                    {quest.title}
                    <small>
                      {quest.current} / {quest.target}
                    </small>
                  </div>
                  <span>+{quest.reward_xp} XP</span>
                </div>
              ))
            ) : (
              <p className={s.muted}>
                Задания появятся, когда сервер обновит профиль.
              </p>
            )}
          </div>
        </aside>
      </div>
      <TaskEditor open={editor} task={editing} close={() => setEditor(false)} />
      <Sheet
        open={deleteIds.length > 0}
        onClose={() => {
          if (!busy) setDeleteIds([]);
        }}
        title="Удалить задачи?"
        description="Это действие нельзя отменить."
        wide
      >
        <div className={s.stack}>
          <p>Будет удалено: {deleteIds.length}.</p>
          {error && (
            <p className={s.error} role="alert">
              {error}
            </p>
          )}
          <Button
            disabled={busy}
            onClick={() =>
              void run(
                () =>
                  deleteIds.length === 1
                    ? api(`/tasks/${deleteIds[0]}`, { method: "DELETE" })
                    : api("/tasks/bulk-delete", {
                        method: "POST",
                        body: json({ task_ids: deleteIds }),
                      }),
                "Задачи удалены.",
              )
            }
          >
            Удалить
          </Button>
          <Button quiet disabled={busy} onClick={() => setDeleteIds([])}>
            Оставить задачи
          </Button>
        </div>
      </Sheet>
    </>
  );
}
function LeafMark() {
  return <span className={s.noteMark}>✳</span>;
}
