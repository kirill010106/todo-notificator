import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { Award, Timer, Gift, Check, LockKeyhole } from "lucide-react";
import { api, errorText } from "../../lib/api";
import type { Achievement, Activity } from "../../lib/types";
import { useApp } from "../../app/context";
import { Button, Heading, State } from "../../components/ui";
import { dateLabel } from "../../lib/tasks";
import s from "../../styles/App.module.css";

export function ProgressPage() {
  const { data } = useApp();
  const profile = data.gamification;
  const [filter, setFilter] = useState("all");
  const [offset, setOffset] = useState(0);
  const achievements = useQuery({
    queryKey: ["achievements"],
    queryFn: ({ signal }) =>
      api<{ achievements: Achievement[] }>("/me/achievements", { signal }),
  });
  const logs = useQuery({
    queryKey: ["logs", offset],
    queryFn: ({ signal }) =>
      api<{ logs: Activity[] }>(
        `/me/logs?limit=${data.user.is_premium ? 20 : 5}&offset=${offset}`,
        { signal },
      ),
  });
  const visibleAchievements = achievements.data?.achievements.filter((item) =>
    filter === "archived" ? item.archived : !item.archived &&
      (filter === "all" || (filter === "unlocked" ? item.unlocked : !item.unlocked)),
  ) || [];
  return (
    <>
      <Heading
        eyebrow="КАЖДЫЙ ШАГ ИМЕЕТ ЗНАЧЕНИЕ"
        title="Твой прогресс"
        action={
          <Link className={s.secondary} to="/app/rewards">
            <Gift size={17} />
            Награды
          </Link>
        }
      >
        Замечай то, что уже получилось.
      </Heading>
      <div className={s.statsGrid}>
        {[
          [Award, "Уровень", profile?.level],
          [Check, "Завершённые задачи", data.stats?.total_completed_tasks],
          [Timer, "Сессии фокуса", data.stats?.total_pomodoros],
        ].map(([Icon, label, value], i) => {
          const Symbol = Icon as typeof Award;
          return (
            <div className={s.card} key={i}>
              <Symbol size={21} className={s.green} />
              <span className={s.muted}>{String(label)}</span>
              <strong className={s.statNumber}>
                {value === undefined ? "—" : String(value)}
              </strong>
            </div>
          );
        })}
      </div>
      {profile && (
        <section className={s.card}>
          <div className={s.rowBetween}>
            <h2>{profile.rank_title}</h2>
            <strong>{profile.points} XP</strong>
          </div>
          <div className={s.progressTrack}>
            <span
              style={{
                width: `${Math.max(0, Math.min(100, profile.progress_percent))}%`,
              }}
            />
          </div>
          <p className={s.muted}>
            За первое завершение задачи — 10 XP, за засчитанную сессию — 20 XP.
          </p>
          <p className={s.muted}>
            {profile.level >= 100 ? "Максимальный уровень" : `${profile.current_level_xp} / ${profile.next_level_xp} XP до следующего уровня`}
          </p>
        </section>
      )}
      <section>
        <div className={s.sectionTitle}>
          <h2>Коллекция достижений</h2>
          <span className={s.count}>{profile?.unlocked_count ?? "—"}</span>
        </div>
        <div className={s.tabs}>
          {[
            ["all", "Все"],
            ["unlocked", "Полученные"],
            ["locked", "Впереди"],
            ["archived", "Архив"],
          ].map(([key, title]) => (
            <button
              className={filter === key ? s.tabActive : ""}
              onClick={() => setFilter(key)}
              key={key}
            >
              {title}
            </button>
          ))}
        </div>
        {achievements.isPending ? (
          <State title="Загружаем достижения…" loading />
        ) : achievements.isError ? (
          <State
            title="Достижения недоступны"
            retry={() => void achievements.refetch()}
          >
            {errorText(achievements.error)}
          </State>
        ) : !visibleAchievements.length ? (
          <State title="В этой группе пока нет достижений" />
        ) : (
          <div className={s.cardGrid}>
            {visibleAchievements.map((item) => (
                <article
                  className={`${s.achievement} ${!item.unlocked ? s.locked : ""}`}
                  key={item.code}
                >
                  <span className={s.achievementIcon}>
                    {item.unlocked ? item.icon : <LockKeyhole />}
                  </span>
                  <div>
                    <h3>{item.title}</h3>
                    <p>{item.description}</p>
                    <small>
                      {item.unlocked ? (item.archived ? "Получено · Архив" : "Получено") : `${item.current} / ${item.target} ${item.code.startsWith("pomodoro_") ? "сессий" : "задач"}`}
                    </small>
                  </div>
                </article>
              ))}
          </div>
        )}
      </section>
      <section>
        <div className={s.sectionTitle}>
          <h2>История активности</h2>
        </div>
        {logs.isPending ? (
          <State title="Загружаем историю…" loading />
        ) : logs.isError ? (
          <State
            title="История временно недоступна"
            retry={() => void logs.refetch()}
          >
            {errorText(logs.error)}
          </State>
        ) : logs.data.logs.length ? (
          <div className={s.card}>
            {logs.data.logs.map((log, i) => (
              <div className={s.log} key={log.id || i}>
                <span className={s.dot} />
                <div>
                  <strong>{actions[log.action] || log.action}</strong>
                  <small>{dateLabel(log.timestamp || log.created_at)}</small>
                </div>
              </div>
            ))}
          </div>
        ) : (
          <State title="История начинается с первого шага" />
        )}
        {data.user.is_premium ? (
          <div className={s.pagination}>
            <Button
              quiet
              disabled={!offset}
              onClick={() => setOffset(Math.max(0, offset - 20))}
            >
              Назад
            </Button>
            <Button
              quiet
              disabled={!logs.data || logs.data.logs.length < 20}
              onClick={() => setOffset(offset + 20)}
            >
              Далее
            </Button>
          </div>
        ) : (
          <p className={s.muted}>
            В бесплатном тарифе доступны 5 последних событий.
          </p>
        )}
      </section>
    </>
  );
}
const actions: Record<string, string> = {
  TASK_CREATED: "Добавлена задача",
  TASK_UPDATED: "Задача обновлена",
  TASK_DELETED: "Задача удалена",
  TASK_COMPLETED: "Задача завершена",
  POMODORO_STARTED: "Началась сессия фокуса",
  POMODORO_STOPPED: "Сессия фокуса завершена",
  POMODORO_PAUSED: "Дополнительный перерыв",
  LOGIN: "Вход в аккаунт",
  REGISTER: "Аккаунт создан",
  CATEGORY_CREATED: "Создана категория",
  CATEGORY_UPDATED: "Категория обновлена",
  CATEGORY_DELETED: "Категория удалена",
  LOGOUT: "Выход из аккаунта",
};
