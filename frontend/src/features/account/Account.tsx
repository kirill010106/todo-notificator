import { useState } from "react";
import { useNavigate, Link } from "react-router-dom";
import {
  Mail,
  ShieldCheck,
  Sun,
  Moon,
  Monitor,
  CreditCard,
  LogOut,
  Volume2,
} from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import { api, endSession, errorText, json, refreshAccess } from "../../lib/api";
import { useApp } from "../../app/context";
import { Button, Heading, Sheet } from "../../components/ui";
import s from "../../styles/App.module.css";

export function setTheme(theme: string) {
  localStorage.setItem("todo:theme", theme);
  document.documentElement.dataset.theme =
    theme === "system"
      ? matchMedia("(prefers-color-scheme: dark)").matches
        ? "dark"
        : "light"
      : theme;
}
export function AccountPage() {
  const { data, refresh, notify } = useApp();
  const client = useQueryClient();
  const navigate = useNavigate();
  const [theme, chooseTheme] = useState(
    localStorage.getItem("todo:theme") || "system",
  );
  const [sound, setSound] = useState(
    localStorage.getItem("sound_enabled") !== "0",
  );
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [logout, setLogout] = useState(false);
  const [timezone, chooseTimezone] = useState(data.user.timezone || "UTC");
  const run = async (fn: () => Promise<void>) => {
    setBusy(true);
    setError("");
    try {
      await fn();
    } catch (e) {
      setError(errorText(e));
    } finally {
      setBusy(false);
    }
  };
  return (
    <>
      <Heading eyebrow="ТВОЁ ПРОСТРАНСТВО" title="Аккаунт и настройки">
        Всё, чтобы чувствовать себя на своём месте.
      </Heading>
      {error && (
        <p className={s.error} role="alert">
          {error}
        </p>
      )}
      <div className={s.settingsGrid}>
        <section className={s.card}>
          <Mail className={s.green} />
          <h2>{data.user.email}</h2>
          <p className={s.muted}>
            {data.user.is_verified
              ? "Email подтверждён. Напоминания готовы к отправке."
              : "Подтверди email, чтобы получать напоминания."}
          </p>
          {!data.user.is_verified && (
            <Button
              quiet
              disabled={busy}
              onClick={() =>
                void run(async () => {
                  await api("/verify/resend", { method: "POST" });
                  notify("Письмо отправлено. Проверь входящие и папку «Спам».");
                })
              }
            >
              Отправить письмо ещё раз
            </Button>
          )}
        </section>
        <section className={`${s.card} ${s.premiumCard}`}>
          <ShieldCheck />
          <span className={s.eyebrow}>БОЛЬШЕ ПРОСТРАНСТВА ДЛЯ ПЛАНОВ</span>
          <h2>
            {data.user.is_premium ? "Твой Premium активен" : "Premium навсегда"}
          </h2>
          <p>
            {data.user.is_premium
              ? "Все возможности доступны без ограничений тарифа."
              : "Без лимитов на категории и активные напоминания. Полная история активности."}
          </p>
          <strong className={s.statNumber}>
            {data.user.is_premium ? "Спасибо, что с нами" : "49 ₽"}
          </strong>
          {!data.user.is_premium && (
            <>
              <p className={s.muted}>
                Разовая оплата. Сумма подтверждается на странице оплаты.
              </p>
              <Button
                disabled={busy}
                onClick={() =>
                  void run(async () => {
                    const result = await api<{ confirmation_url?: string }>(
                      "/payments/create",
                      { method: "POST" },
                    );
                    if (!result.confirmation_url)
                      throw new Error("Missing payment URL");
                    const url = new URL(result.confirmation_url);
                    if (url.protocol !== "https:")
                      throw new Error("Invalid payment URL");
                    location.assign(url.href);
                  })
                }
              >
                <CreditCard size={17} />
                Перейти к оплате
              </Button>
            </>
          )}
          <Button
            quiet
            disabled={busy}
            onClick={() =>
              void run(async () => {
                const result = await api<{
                  is_premium?: boolean;
                  payment_status?: string;
                }>("/payments/sync", { method: "POST" });
                if (result.is_premium) await refreshAccess();
                await refresh();
                notify(
                  result.is_premium
                    ? "Premium активирован."
                    : result.payment_status === "pending"
                      ? "Платёж ещё обрабатывается. Проверь статус позже."
                      : "Подтверждённый платёж пока не найден.",
                );
              })
            }
          >
            Проверить статус оплаты
          </Button>
          <small>
            <a href="/offer.html">Условия оферты</a> ·{" "}
            <a href="/policy.html">Конфиденциальность</a>
          </small>
        </section>
        <section className={s.card}>
          <Sun className={s.green} />
          <h2>Внешний вид</h2>
          <p className={s.muted}>Подстрой пространство под свой ритм.</p>
          <div className={s.row}>
            {[
              ["light", Sun, "Светлая"],
              ["dark", Moon, "Тёмная"],
              ["system", Monitor, "Системная"],
            ].map(([key, Icon, label]) => {
              const Symbol = Icon as typeof Sun;
              return (
                <Button
                  quiet
                  key={String(key)}
                  aria-pressed={theme === key}
                  onClick={() => {
                    chooseTheme(String(key));
                    setTheme(String(key));
                  }}
                >
                  <Symbol size={16} />
                  {String(label)}
                </Button>
              );
            })}
          </div>
        </section>
        <section className={s.card}>
          <Volume2 className={s.green} />
          <h2>Звуки</h2>
          <p className={s.muted}>Небольшой сигнал, когда дело сделано.</p>
          <label className={s.row}>
            <input
              type="checkbox"
              checked={sound}
              onChange={(e) => {
                setSound(e.target.checked);
                localStorage.setItem(
                  "sound_enabled",
                  e.target.checked ? "1" : "0",
                );
              }}
            />
            Звук завершения
          </label>
        </section>
      </div>
      <section className={s.card}>
        <h2>Часовой пояс</h2>
        <p className={s.muted}>
          Определяет день для задач, серий и ежедневных наград.
        </p>
        <label className={s.field}>
          Часовой пояс IANA
          <input
            list="timezones"
            value={timezone}
            onChange={(event) => chooseTimezone(event.target.value)}
          />
          <datalist id="timezones">
            <option value="UTC" />
            <option value="Europe/Moscow" />
            <option value={Intl.DateTimeFormat().resolvedOptions().timeZone} />
          </datalist>
        </label>
        <Button
          quiet
          disabled={busy || timezone === data.user.timezone}
          onClick={() =>
            void run(async () => {
              await api("/me/profile", {
                method: "PATCH",
                body: json({ timezone }),
              });
              await refresh();
              notify("Часовой пояс сохранён.");
            })
          }
        >
          Сохранить часовой пояс
        </Button>
      </section>
      <div className={s.rowBetween}>
        <Link to="/app/categories" className={s.textLink}>
          Управлять категориями →
        </Link>
        <Button quiet onClick={() => setLogout(true)}>
          <LogOut size={16} />
          Выйти из аккаунта
        </Button>
      </div>
      <Sheet
        open={logout}
        onClose={() => {
          if (!busy) setLogout(false);
        }}
        title="Выйти из аккаунта?"
        description="Серверные данные сохранятся. Активный таймер продолжит идти."
        wide
      >
        <div className={s.stack}>
          {error && (
            <p className={s.error} role="alert">
              {error}
            </p>
          )}
          <Button
            disabled={busy}
            onClick={() =>
              void run(async () => {
                await api("/logout", {
                  method: "POST",
                  body: json({
                    refresh_token: localStorage.getItem("refresh_token"),
                  }),
                });
                endSession();
                client.clear();
                navigate("/login", { replace: true });
              })
            }
          >
            Выйти
          </Button>
        </div>
      </Sheet>
    </>
  );
}
