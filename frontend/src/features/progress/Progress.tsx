import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { Award, Flame, Timer, Gift, Check, LockKeyhole } from "lucide-react";
import { api, errorText } from "../../lib/api";
import { claimDailyReward } from "../../lib/xp-rewards";
import { rewardLabel } from "../../lib/commands";
import type { Achievement, Activity } from "../../lib/types";
import { useApp } from "../../app/context";
import { Button, Heading, State } from "../../components/ui";
import { dateLabel } from "../../lib/tasks";
import s from "../../styles/App.module.css";

export function ProgressPage() {
  const { data, refresh, notify } = useApp();
  const [claiming, setClaiming] = useState("");
  const [claimError, setClaimError] = useState("");
  const claim = async (code: string) => {
    setClaiming(code);
    setClaimError("");
    try {
      const result = await claimDailyReward(data.user.id, code);
      notify(`Награда получена: ${rewardLabel(result)}`);
      await refresh();
    } catch (error) {
      setClaimError(errorText(error));
    } finally {
      setClaiming("");
    }
  };
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
          [Flame, "Серия дней", profile?.streak],
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
            {profile.current_level_xp} / {profile.next_level_xp} XP до
            следующего уровня · Лучшая серия: {profile.best_streak} дней
          </p>
        </section>
      )}
      <section className={s.card}>
        <h2>Ежедневный ритм</h2>
        <p className={s.muted}>
          Новый день заданий — по часовому поясу {data.user.timezone || "UTC"}.
        </p>
        {claimError && (
          <p className={s.error} role="alert">
            {claimError}
          </p>
        )}
        {profile?.daily_quests?.length ? (
          profile.daily_quests.map((quest) => (
            <div className={s.quest} key={quest.code}>
              {quest.completed ? <Check size={18} /> : <Timer size={18} />}
              <div>
                {quest.title}
                <small>
                  {quest.current} / {quest.target}
                </small>
              </div>
              <Button
                quiet
                disabled={!!claiming || !quest.completed || quest.claimed}
                onClick={() => void claim(quest.code)}
              >
                {quest.claimed ? "Получено" : `Забрать ${quest.reward_xp} XP`}
              </Button>
            </div>
          ))
        ) : (
          <p className={s.muted}>Задания пока не загружены сервером.</p>
        )}
      </section>
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
        ) : (
          <div className={s.cardGrid}>
            {achievements.data.achievements
              .filter(
                (item) =>
                  filter === "all" ||
                  (filter === "unlocked" ? item.unlocked : !item.unlocked),
              )
              .map((item) => (
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
                      {item.unlocked ? "Получено" : `${item.points} XP`}
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
