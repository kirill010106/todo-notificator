import { useEffect, useState } from "react";
import { Gift, Plus, Trash2, Sparkles, Palette, Volume2 } from "lucide-react";
import { rewards, useApp } from "../../app/context";
import type { Wallet } from "../../lib/rewards";
import { Button, Heading, Sheet } from "../../components/ui";
import { claimDailyReward } from "../../lib/xp-rewards";
import { rewardLabel } from "../../lib/commands";
import { errorText } from "../../lib/api";
import s from "../../styles/App.module.css";

export function RewardsPage() {
  const { data, notify, refresh } = useApp();
  const [claiming, setClaiming] = useState(false);
  const [wallet, setWallet] = useState<Wallet | null>(null);
  const [error, setError] = useState("");
  const [open, setOpen] = useState(false);
  const [title, setTitle] = useState("");
  const [price, setPrice] = useState("50");
  const claim = async () => {
    setClaiming(true);
    setError("");
    try {
      const result = await claimDailyReward(data.user.id);
      await refresh();
      notify(`Награда подтверждена: ${rewardLabel(result)}.`);
    } catch (error) {
      setError(
        error instanceof Error && !(error instanceof TypeError)
          ? error.message
          : errorText(error),
      );
    } finally {
      setClaiming(false);
    }
  };
  const dailyReady =
    !!data.gamification?.daily_quests?.length &&
    data.gamification.daily_quests.every((quest) => quest.completed);
  const rewardBlocked = claiming;
  useEffect(() => {
    const load = () => {
      try {
        setWallet(rewards.load(data.user.id));
      } catch {
        setError(
          "Не удалось прочитать хранилище браузера. Разреши сохранение данных для этого сайта.",
        );
      }
    };
    load();
    window.addEventListener("rewards-changed", load);
    window.addEventListener("storage", load);
    return () => {
      window.removeEventListener("rewards-changed", load);
      window.removeEventListener("storage", load);
    };
  }, [data.user.id]);
  const update = (change: (wallet: Wallet) => Wallet) => {
    try {
      setWallet(rewards.update(data.user.id, change));
      setError("");
      return true;
    } catch (error) {
      setError(
        error instanceof Error && error.message === "Недостаточно монет."
          ? error.message
          : "Браузер не смог сохранить изменения.",
      );
      return false;
    }
  };
  return (
    <>
      <Heading
        eyebrow="ЗАМЕЧАЙ МАЛЕНЬКИЕ ПОБЕДЫ"
        title="Награды для себя"
        action={
          <Button onClick={() => setOpen(true)}>
            <Plus size={17} />
            Своя награда
          </Button>
        }
      >
        Приятные поводы двигаться дальше.
      </Heading>
      <div className={s.wallet}>
        <span className={s.achievementIcon}>
          <Sparkles />
        </span>
        <div>
          <small>ТВОЙ БАЛАНС</small>
          <strong>
            {wallet?.coins ?? "—"} <span>монет</span>
          </strong>
        </div>
        <p>
          +5 за завершённую задачу
          <br />
          +10 за завершённую сессию
        </p>
      </div>
      <p className={s.notice}>
        Монеты и покупки сохраняются в этом браузере для твоего аккаунта. На
        других устройствах они пока недоступны.
      </p>
      {error && (
        <p className={s.error} role="alert">
          {error}
        </p>
      )}
      <div className={s.cardGrid}>
        <section className={s.card}>
          <Gift className={s.green} />
          <h2>Сундук дня</h2>
          <p className={s.muted}>
            Выполни все ежедневные задания и забери 50 XP.
          </p>
          <Button
            disabled={
              rewardBlocked ||
              !dailyReady ||
              data.gamification?.daily_chest_claimed
            }
            onClick={() => void claim()}
          >
            {data.gamification?.daily_chest_claimed
              ? "Сегодня уже получено"
              : "Забрать 50 XP"}
          </Button>
        </section>
        <section className={s.card}>
          <Sparkles className={s.green} />
          <h2>Золотой сундук</h2>
          <p className={s.muted}>
            Будет доступен после подключения серверного кошелька. Локальные
            монеты пока используются только для личных наград.
          </p>
          <Button disabled>Пока недоступно</Button>
        </section>
      </div>
      <div className={s.cardGrid}>
        {wallet?.customRewards.map((reward) => (
          <article className={s.card} key={reward.id}>
            <Gift size={26} className={s.green} />
            <h2>{reward.title}</h2>
            <p className={s.muted}>{reward.price} монет</p>
            <div className={s.row}>
              <Button
                disabled={wallet.coins < reward.price}
                onClick={() => {
                  if (
                    update((w) => {
                      if (w.coins < reward.price)
                        throw new Error("Недостаточно монет.");
                      return { ...w, coins: w.coins - reward.price };
                    })
                  )
                    notify(`Награда «${reward.title}» твоя. Наслаждайся!`);
                }}
              >
                Получить награду
              </Button>
              <button
                className={s.iconButton}
                aria-label={`Удалить награду ${reward.title}`}
                onClick={() =>
                  update((w) => ({
                    ...w,
                    customRewards: w.customRewards.filter(
                      (item) => item.id !== reward.id,
                    ),
                  }))
                }
              >
                <Trash2 size={16} />
              </button>
            </div>
          </article>
        ))}
        <button className={s.rewardPlaceholder} onClick={() => setOpen(true)}>
          <Plus size={27} />
          <h3>Что тебя порадует?</h3>
          <p>
            Кофе в любимом месте, новая книга
            <br />
            или вечер без планов.
          </p>
        </button>
      </div>
      <div className={s.sectionTitle}>
        <h2>Оформление и звук</h2>
      </div>
      <div className={s.cardGrid}>
        <section className={s.card}>
          <Palette />
          <h3>Твоя палитра</h3>
          <div className={s.row}>
            {[
              ["default", "Зелёная"],
              ["ocean", "Океан"],
              ["violet", "Лаванда"],
            ].map(([key, label]) => (
              <Button
                quiet
                key={key}
                aria-pressed={wallet?.activeTheme === key}
                onClick={() => {
                  if (update((w) => ({ ...w, activeTheme: key }))) {
                    document.documentElement.dataset.accent = key;
                    localStorage.setItem("todo:accent", key);
                  }
                }}
              >
                {label}
              </Button>
            ))}
          </div>
        </section>
        <section className={s.card}>
          <Volume2 />
          <h3>Звук завершения</h3>
          <p className={s.muted}>
            Мягкий аккорд. Включить или выключить звук можно в настройках
            аккаунта.
          </p>
        </section>
      </div>
      <div className={s.sectionTitle}>
        <h2>Бустеры</h2>
      </div>
      <div className={s.card}>
        <h3>Заморозка серии, усилитель опыта и щит</h3>
        <p className={s.muted}>
          Эти эффекты пока не поддерживаются сервером. Покупки недоступны; ранее
          приобретённые предметы сохранены.
        </p>
        <p className={s.muted}>
          В инвентаре:{" "}
          {Object.values(wallet?.boosters || {}).reduce(
            (sum, value) => sum + (Number(value) || 0),
            0,
          )}
        </p>
        <Button quiet disabled>
          Скоро
        </Button>
      </div>
      <Sheet
        open={open}
        onClose={() => setOpen(false)}
        title="Твоя маленькая награда"
        wide
      >
        <form
          className={s.stack}
          onSubmit={(e) => {
            e.preventDefault();
            if (
              !title.trim() ||
              !Number.isInteger(Number(price)) ||
              Number(price) < 10
            )
              return;
            if (
              update((w) => ({
                ...w,
                customRewards: [
                  ...w.customRewards,
                  {
                    id: crypto.randomUUID(),
                    title: title.trim(),
                    price: Number(price),
                  },
                ],
              }))
            ) {
              setOpen(false);
              setTitle("");
            }
          }}
        >
          <label className={s.field}>
            Что тебя порадует?
            <input
              autoFocus
              required
              maxLength={150}
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              placeholder="Например, новая книга"
            />
          </label>
          <label className={s.field}>
            Стоимость в монетах
            <input
              required
              type="number"
              min="10"
              step="1"
              max="1000000"
              value={price}
              onChange={(e) => setPrice(e.target.value)}
            />
          </label>
          {error && <p className={s.error}>{error}</p>}
          <Button>Добавить награду</Button>
        </form>
      </Sheet>
    </>
  );
}
