import { useState } from "react";
import { Gift, Plus, Sparkles, Trash2 } from "lucide-react";
import { useApp } from "../../app/context";
import { importLegacyRewards, shop, useShop } from "../../lib/shop";
import { readLegacyRewards } from "../../lib/rewards";
import { Button, Heading, Sheet, State } from "../../components/ui";
import { errorText } from "../../lib/api";
import { claimDailyReward } from "../../lib/xp-rewards";
import { rewardLabel } from "../../lib/commands";
import s from "../../styles/App.module.css";
export function RewardsPage() {
  const { data, notify, refresh } = useApp();
  const query = useShop(data.user.id);
  const [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const [open, setOpen] = useState(false),
    [title, setTitle] = useState(""),
    [price, setPrice] = useState("50");
  const [purchase, setPurchase] = useState<{
    action: string;
    item_id: string;
    title: string;
    price: number;
  } | null>(null);
  const wallet = query.data?.wallet;
  const legacy = readLegacyRewards(data.user.id);
  const run = async (body: Record<string, unknown>) => {
    if (busy) return false;
    setBusy(true);
    setError("");
    try {
      await shop.action(data.user.id, body);
      await refresh();
      notify("Изменения сохранены.");
      return true;
    } catch (e) {
      setError(errorText(e));
      return false;
    } finally {
      setBusy(false);
    }
  };
  const dailyReady =
    !!data.gamification?.daily_quests?.length &&
    data.gamification.daily_quests.every((q) => q.completed);
  return (
    <>
      <Heading
        eyebrow="ЗАМЕЧАЙ МАЛЕНЬКИЕ ПОБЕДЫ"
        title="Магазин и награды"
        action={
          <Button onClick={() => setOpen(true)}>
            <Plus size={17} />
            Своя награда
          </Button>
        }
      >
        Выбирай, что порадует тебя за проделанную работу.
      </Heading>
      {query.isPending ? (
        <State title="Загружаем магазин…" loading />
      ) : query.isError ? (
        <State
          title="Не удалось открыть магазин"
          retry={() => void query.refetch()}
        >
          {errorText(query.error)}
        </State>
      ) : (
        <>
          <div className={s.wallet}>
            <Sparkles />
            <div>
              <small>ТВОЙ БАЛАНС</small>
              <strong>
                {wallet?.coins} <span>монет</span>
              </strong>
            </div>
            <p>
              +5 за первое завершение задачи
              <br />
              +10 за засчитанную сессию фокуса
            </p>
          </div>
          <p className={s.notice}>
            Баланс и покупки хранятся на сервере и доступны на твоих
            устройствах. Начальный баланс — 50 монет. Старые локальные значения
            сохранены в браузере для справки.
          </p>
          {!wallet?.imported && legacy.available && (
            <div className={s.notice}>
              <p>На этом устройстве найдены старые личные награды или оформление. Монеты и бустеры не переносятся.</p>
              <Button
                quiet
                disabled={busy}
                onClick={async () => {
                  if (busy) return;
                  setBusy(true);
                  setError("");
                  try {
                    await importLegacyRewards(data.user.id, legacy.legacy);
                    await query.refetch();
                    notify("Старые награды и настройки перенесены.");
                  } catch (e) {
                    setError(errorText(e));
                  } finally {
                    setBusy(false);
                  }
                }}
              >
                Перенести данные с этого устройства
              </Button>
            </div>
          )}
          {error && (
            <p className={s.error} role="alert">
              {error}
            </p>
          )}
          {wallet?.activeBooster && (
            <p className={s.notice}>
              Двойной фокус активен: следующая засчитанная сессия принесёт 40 XP
              вместо 20. Ранняя отмена не расходует бустер.
            </p>
          )}
          <div className={s.sectionTitle}>
            <h2>Оформление, звук и бустеры</h2>
          </div>
          <div className={s.cardGrid}>
            {query.data?.catalog.map((item) => {
              const owned = wallet?.ownedItems.includes(item.id);
              const equipped =
                wallet?.activeTheme === item.id ||
                wallet?.activeSound === item.id;
              return (
                <article key={item.id} className={s.card}>
                  <Sparkles className={s.green} />
                  <h3>{item.title}</h3>
                  <p className={s.muted}>{item.description}</p>
                  {item.kind === "booster" && (
                    <p>В инвентаре: {wallet?.boosters[item.id] || 0}</p>
                  )}
                  <div className={s.stack}>
                    {owned ? (
                      <Button
                        quiet
                        disabled={busy || equipped}
                        onClick={() =>
                          void run({ action: "equip", item_id: item.id })
                        }
                      >
                        {equipped ? "Используется" : "Применить"}
                      </Button>
                    ) : (
                      <Button
                        disabled={busy}
                        onClick={() => {
                          setError("");
                          setPurchase({
                            action: "buy",
                            item_id: item.id,
                            title: item.title,
                            price: item.price,
                          });
                        }}
                      >
                        Купить · {item.price} монет
                      </Button>
                    )}
                    {item.kind === "booster" && (
                      <Button
                        quiet
                        disabled={
                          busy ||
                          !!wallet?.activeBooster ||
                          !wallet?.boosters[item.id]
                        }
                        onClick={() => void run({ action: "activate_booster" })}
                      >
                        {wallet?.activeBooster
                          ? "Бустер уже активен"
                          : "Активировать двойной XP"}
                      </Button>
                    )}
                  </div>
                </article>
              );
            })}
          </div>
          <div className={s.row}>
            <Button
              quiet
              disabled={busy || wallet?.activeTheme === "default"}
              onClick={() => void run({ action: "equip", item_id: "default" })}
            >
              Зелёная палитра
            </Button>
            <Button
              quiet
              disabled={busy || wallet?.activeSound === "default"}
              onClick={() =>
                void run({ action: "equip", item_id: "default_sound" })
              }
            >
              Обычный звук
            </Button>
          </div>
          <p className={s.muted}>
            Бустер удваивает только 20 XP за фокус. Награды за задачу,
            достижения и задания не умножаются. Сессия засчитывается после 24
            минут. Одновременно активен один бустер.
          </p>
          <div className={s.sectionTitle}>
            <h2>Личные награды</h2>
          </div>
          <div className={s.cardGrid}>
            {wallet?.customRewards.map((reward) => (
              <article className={s.card} key={reward.id}>
                <Gift className={s.green} />
                <h3>{reward.title}</h3>
                <p>{reward.price} монет</p>
                <div className={s.row}>
                  <Button
                    disabled={busy}
                    onClick={() => {
                      setError("");
                      setPurchase({
                        action: "redeem_personal",
                        item_id: reward.id,
                        title: reward.title,
                        price: reward.price,
                      });
                    }}
                  >
                    Получить награду
                  </Button>
                  <button
                    className={s.iconButton}
                    disabled={busy}
                    aria-label={`Удалить награду ${reward.title}`}
                    onClick={() =>
                      void run({
                        action: "delete_personal",
                        item_id: reward.id,
                      })
                    }
                  >
                    <Trash2 size={16} />
                  </button>
                </div>
              </article>
            ))}
            <button
              className={s.rewardPlaceholder}
              onClick={() => setOpen(true)}
            >
              <Plus />
              <h3>Что тебя порадует?</h3>
              <p>Книга, чашка кофе или вечер без планов.</p>
            </button>
          </div>
          <section className={s.card}>
            <h2>Сундук дня</h2>
            <p>
              Выполни ежедневные задания и забери 50 XP. Случайные сундуки пока
              недоступны.
            </p>
            <Button
              disabled={
                busy || !dailyReady || data.gamification?.daily_chest_claimed
              }
              onClick={async () => {
                setBusy(true);
                setError("");
                try {
                  const result = await claimDailyReward(data.user.id);
                  await refresh();
                  notify(`Награда подтверждена: ${rewardLabel(result)}.`);
                } catch (e) {
                  setError(errorText(e));
                } finally {
                  setBusy(false);
                }
              }}
            >
              {data.gamification?.daily_chest_claimed
                ? "Сегодня уже получено"
                : "Забрать 50 XP"}
            </Button>
          </section>
          <div className={s.sectionTitle}>
            <h2>История монет</h2>
          </div>
          <section className={s.card}>
            {query.data?.ledger.length ? (
              query.data.ledger.map((entry) => (
                <div className={s.rowBetween} key={entry.id}>
                  <span>
                    {entry.description}
                    <small className={s.muted}>
                      {" "}
                      · {new Date(entry.created_at).toLocaleString("ru-RU")}
                    </small>
                  </span>
                  <strong>
                    {entry.coins > 0 ? "+" : ""}
                    {entry.coins}
                  </strong>
                </div>
              ))
            ) : (
              <p className={s.muted}>
                Здесь появятся подтверждённые покупки и начисления.
              </p>
            )}
          </section>
        </>
      )}
      <Sheet
        open={!!purchase}
        onClose={() => {
          if (!busy) setPurchase(null);
        }}
        title="Подтвердить покупку"
        description={purchase?.title}
        wide
      >
        <p>
          Стоимость: {purchase?.price} монет. На балансе: {wallet?.coins ?? "—"}
          .
        </p>
        {error && (
          <p className={s.error} role="alert">
            {error}
          </p>
        )}
        <Button
          disabled={busy}
          onClick={async () => {
            if (
              purchase &&
              (await run({
                action: purchase.action,
                item_id: purchase.item_id,
              }))
            )
              setPurchase(null);
          }}
        >
          Подтвердить · {purchase?.price} монет
        </Button>
        <Button quiet disabled={busy} onClick={() => setPurchase(null)}>
          Отмена
        </Button>
      </Sheet>
      <Sheet
        open={open}
        onClose={() => {
          if (!busy) setOpen(false);
        }}
        title="Твоя маленькая награда"
        wide
      >
        <form
          className={s.stack}
          onSubmit={async (e) => {
            e.preventDefault();
            if (
              await run({
                action: "create_personal",
                title: title.trim(),
                price: Number(price),
              })
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
              maxLength={120}
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
              max="1000000"
              step="1"
              value={price}
              onChange={(e) => setPrice(e.target.value)}
            />
          </label>
          {error && (
            <p className={s.error} role="alert">
              {error}
            </p>
          )}
          <Button disabled={busy}>Добавить награду</Button>
        </form>
      </Sheet>
    </>
  );
}
