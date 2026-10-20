import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import * as Menu from "@radix-ui/react-dropdown-menu";
import { useNavigate, useSearchParams, Link } from "react-router-dom";
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
  Play,
  MoreHorizontal,
} from "lucide-react";
import { api, errorText, json } from "../../lib/api";
import { command, rewardLabel } from "../../lib/commands";
import { safeTimeZone } from "../../lib/timezone";
import { dateLabel } from "../../lib/tasks";
import type {
  Task,
  TaskPage,
  TodayState,
  ActionResponse,
} from "../../lib/types";
import { useApp } from "../../app/context";
import { useFocus } from "../focus/Focus";
import { Button, Sheet, Heading, State } from "../../components/ui";
import s from "../../styles/App.module.css";

export { TaskEditor } from "./TaskEditor";
import { TaskEditor } from "./TaskEditor";

export function TasksPage({ today = false }: { today?: boolean }) {
  const { data, notify, refresh, award } = useApp();
  const focus = useFocus();
  const [params, setParams] = useSearchParams();
  const navigate = useNavigate();
  const picking = params.get("pick") === "focus";
  const pick = (task: Task) => {
    focus.chooseTask(task);
    navigate("/app/focus");
  };
  const [editor, setEditor] = useState(false);
  const [editing, setEditing] = useState<Task | null>(null);
  const [selected, setSelected] = useState<number[]>([]);
  const [selecting, setSelecting] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [deleteIds, setDeleteIds] = useState<number[]>([]);
  const [clock, setClock] = useState(new Date());
  const [search, setSearch] = useState(params.get("search") || "");
  const [filtersOpen, setFiltersOpen] = useState(false);
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
    setSelecting(false);
  }, [params.toString(), today]);
  useEffect(() => {
    setSearch(params.get("search") || "");
  }, [params]);
  const status = picking ? "pending" : params.get("status") || "pending";
  const category = params.get("category") || "";
  const sort = params.get("sort") || "created_at";
  const filterCount = Number(Boolean(category)) + Number(sort !== "created_at");
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
          {!today && !picking && selecting && (
            <input
              type="checkbox"
              className={s.selectCheck}
              aria-label={`Выбрать: ${task.title}`}
              checked={selected.includes(task.id)}
              disabled={busy}
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
            onClick={() => (picking ? pick(task) : toggle(task))}
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
              if (picking) {
                pick(task);
                return;
              }
              setEditing(task);
              setEditor(true);
            }}
          >
            <strong>{task.title}</strong>
            <span>
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
                {task.reminders?.some((r) => r.status === "scheduled") && (
                  <Bell size={12} />
                )}
                {task.status === "burnt" && (
                  <span className={s.danger}>Сожжена</span>
                )}
              </span>
            </span>
          </button>
          {picking ? (
            <Button quiet
              disabled={focus.busy || !!focus.session || task.status !== "pending"}
              onClick={() => pick(task)}
              aria-label={`Выбрать для фокуса: ${task.title}`}
            >
              Выбрать
            </Button>
          ) : (
            <Menu.Root>
              <Menu.Trigger className={s.iconButton} disabled={busy}
                aria-label={`Действия: ${task.title}`}>
                <MoreHorizontal size={20} />
              </Menu.Trigger>
              <Menu.Portal>
                <Menu.Content className={s.dropdown} align="end" sideOffset={8}>
                  <Menu.Item className={s.taskMenuItem}
                    disabled={focus.busy || !!focus.session || task.status !== "pending"}
                    onSelect={() => focus.requestStart(task)}>
                    <Play size={16} /> Начать фокус
                  </Menu.Item>
                  <Menu.Item className={s.taskMenuItem} disabled={busy}
                    onSelect={() => setDeleteIds([task.id])}>
                    <Trash2 size={16} /> Удалить
                  </Menu.Item>
                </Menu.Content>
              </Menu.Portal>
            </Menu.Root>
          )}
        </article>
      ))}
    </div>
  );
  return (
    <div className={s.tasksPage}>
      <Heading
        eyebrow={
          today
            ? clock.toLocaleDateString("ru-RU", {
                timeZone: safeTimeZone(data.user.timezone),
                weekday: "long",
                day: "numeric",
                month: "long",
              })
            : undefined
        }
        title={
          picking
            ? "Выбери задачу для фокуса"
            : today
              ? "Сегодня"
              : "Мои задачи"
        }
        action={
          <Button className={s.taskCreateButton} onClick={create}>
            <Plus size={18} /> Новая задача
          </Button>
        }
      />
      {picking && (
        <p className={s.notice}>
          Выбор задачи ещё не запускает таймер.{" "}
          <Link to="/app/focus">Вернуться к фокусу</Link>
        </p>
      )}
      <div className={s.tasksWorkspace}>
        <div className={s.mainColumn}>

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
              <Button
                quiet
                aria-expanded={filtersOpen}
                aria-controls="task-filters"
                onClick={() => setFiltersOpen((open) => !open)}
              >
                <SlidersHorizontal size={16} />
                Фильтры{filterCount > 0 ? ` (${filterCount})` : ""}
              </Button>
              {filterCount > 0 && (
                <Button quiet onClick={() => {
                  const next = new URLSearchParams(params);
                  next.delete("category");
                  next.delete("sort");
                  next.delete("offset");
                  setParams(next);
                }}>
                  Сбросить фильтры
                </Button>
              )}
              <div id="task-filters" className={s.filterPanel} hidden={!filtersOpen}>
              <label>
                Категория
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
              </label>
              <label>
                Сортировка
              <select
                aria-label="Сортировка"
                value={sort}
                onChange={(e) => setFilter("sort", e.target.value)}
              >
                <option value="created_at">Сначала новые</option>
                <option value="deadline">По сроку</option>
              </select>
              </label>
              </div>
            </div>
          )}
          {!today && !picking && (
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
            </div>
          )}
          {error && (
            <p className={s.error} role="alert">
              {error}
            </p>
          )}
          {!today && !picking && (
            <div className={s.selectionToolbar}>
              <Button quiet disabled={busy} aria-pressed={selecting}
                onClick={() => {
                  setSelecting((value) => !value);
                  setSelected([]);
                }}>
                {selecting ? "Отменить выбор" : "Выбрать"}
              </Button>
            </div>
          )}
          {selecting && selected.length > 0 && (
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
                    <h2>Просроченные</h2>
                    <span className={s.count}>{overdueTotal}</span>
                  </div>
                  {renderTasks(groups.overdue)}
                  {groupPagination("overdue")}
                </section>
              )}
              <section>
                <div className={s.sectionTitle}>
                  <h2>На сегодня</h2>
                  <span className={s.count}>{todayTotal}</span>
                </div>
                {groups.today.length ? (
                  renderTasks(groups.today)
                ) : (
                  <State title="На сегодня задач нет">
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

        </div>
      </div>
      <button className={s.fab} aria-label="Создать задачу" onClick={create}>
        <Plus size={26} />
      </button>
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
    </div>
  );
}
