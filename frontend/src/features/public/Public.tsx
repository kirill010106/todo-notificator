import { useEffect, useRef, useState } from "react";
import { Link, Navigate, useNavigate } from "react-router-dom";
import {
  ArrowUpRight,
  Check,
  Circle,
  Leaf,
  Timer,
  Bell,
  ShieldCheck,
} from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import {
  api,
  errorText,
  hasSession,
  json,
  saveTokens,
  refreshAccess,
} from "../../lib/api";
import { Brand, Button, State } from "../../components/ui";
import s from "../../styles/App.module.css";

export function Landing() {
  return (
    <div className={s.landing}>
      <nav className={s.publicNav}>
        <Brand />
        <div className={s.row}>
          <Link to="/login" className={s.textLink}>
            Войти
          </Link>
          <Link
            to={hasSession() ? "/app/today" : "/register"}
            className={s.button}
          >
            {hasSession() ? "К моим задачам" : "Начать бесплатно"}
            <ArrowUpRight size={16} />
          </Link>
        </div>
      </nav>
      <main>
        <section className={s.hero}>
          <div>
            <span className={s.eyebrow}>
              <span className={s.dot} /> МЕСТО ДЛЯ ВАЖНОГО
            </span>
            <h1>
              Меньше суеты.
              <br />
              Больше <em>смысла.</em>
            </h1>
            <p>
              Собери задачи, найди свой фокус и замечай прогресс. Спокойное
              пространство для твоих больших и маленьких планов.
            </p>
            <Link className={s.button} to="/register">
              Освободить голову <ArrowUpRight size={18} />
            </Link>
            <small>Бесплатно для начала. В своём темпе.</small>
          </div>
          <div
            className={s.heroPreview}
            aria-label="Пример интерфейса приложения"
          >
            <div className={s.rowBetween}>
              <span className={s.eyebrow}>ТВОЙ НОВЫЙ РИТМ</span>
              <Leaf size={19} />
            </div>
            <h2>Сегодня — хороший день.</h2>
            <p className={s.muted}>Три маленьких шага к большему.</p>
            {[
              ["Продумать новую идею", true],
              ["Выделить время для главного", false],
              ["Сделать паузу и выдохнуть", false],
            ].map(([title, done]) => (
              <div className={s.previewTask} key={String(title)}>
                {done ? <Check size={19} /> : <Circle size={19} />}
                <span>{title}</span>
              </div>
            ))}
            <div className={s.previewFocus}>
              <Timer size={22} />
              <div>
                <strong>25:00</strong>
                <small>Время для одной важной вещи</small>
              </div>
              <span>Фокус</span>
            </div>
            <div className={s.previewFoot}>
              ✳ Маленькие шаги. Настоящие перемены.
            </div>
          </div>
        </section>
        <section className={s.featureGrid}>
          {[
            [
              Leaf,
              "Задачи без лишнего шума",
              "Понятные списки, категории и сроки. Всё нужное — на своём месте.",
            ],
            [
              Timer,
              "Внимание к одному делу",
              "Сессии Pomodoro помогают начать и сохранить ритм.",
            ],
            [
              Bell,
              "Важное не потеряется",
              "Напоминания по email в нужный момент.",
            ],
            [
              ShieldCheck,
              "Прогресс, который видно",
              "Серии, достижения и маленькие награды за твои усилия.",
            ],
          ].map(([Icon, title, text]) => {
            const Symbol = Icon as typeof Leaf;
            return (
              <article key={String(title)}>
                <Symbol size={24} />
                <h3>{String(title)}</h3>
                <p>{String(text)}</p>
              </article>
            );
          })}
        </section>
        <section className={s.landingCta}>
          <span className={s.eyebrow}>ПРОСТО НАЧНИ</span>
          <h2>
            Не идеальный план.
            <br />
            Твой следующий шаг.
          </h2>
          <Link className={s.button} to="/register">
            Создать аккаунт <ArrowUpRight size={17} />
          </Link>
        </section>
      </main>
      <footer className={s.publicFooter}>
        <Brand />
        <span>© 2026 ToDo Notificator</span>
        <a href="/policy.html">Конфиденциальность</a>
        <a href="/offer.html">Оферта</a>
      </footer>
    </div>
  );
}
export function AuthPage({ register = false }: { register?: boolean }) {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [accepted, setAccepted] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [created, setCreated] = useState(false);
  const navigate = useNavigate();
  const client = useQueryClient();
  if (hasSession()) return <Navigate to="/app/today" replace />;
  return (
    <div className={s.authPage}>
      <aside className={s.authAside}>
        <Brand />
        <div>
          <span className={s.eyebrow}>В СВОЁМ ТЕМПЕ</span>
          <h1>
            Для дел.
            <br />
            Для планов.
            <br />
            <em>Для себя.</em>
          </h1>
          <p>
            Немного порядка в делах —<br />
            чуть больше свободы в жизни.
          </p>
        </div>
        <small>ToDo Notificator · Твоё пространство фокуса</small>
      </aside>
      <main className={s.authMain}>
        <div className={s.mobileBrand}>
          <Brand />
        </div>
        {created ? (
          <div className={s.authForm}>
            <ShieldCheck size={40} className={s.green} />
            <h1>Первый шаг сделан.</h1>
            <p>
              Аккаунт создан. Мы отправили письмо для подтверждения email. После
              подтверждения войди в приложение.
            </p>
            <Link
              to="/login"
              className={s.button}
              onClick={() => setCreated(false)}
            >
              Перейти ко входу
            </Link>
          </div>
        ) : (
          <form
            className={s.authForm}
            onSubmit={async (e) => {
              e.preventDefault();
              setBusy(true);
              setError("");
              try {
                if (register) {
                  await api(
                    "/register",
                    { method: "POST", body: json({ email, password }) },
                    false,
                  );
                  setCreated(true);
                } else {
                  const result = await api<{
                    access_token: string;
                    refresh_token: string;
                  }>(
                    "/login",
                    { method: "POST", body: json({ email, password }) },
                    false,
                  );
                  client.clear();
                  saveTokens(result);
                  localStorage.setItem("user_email", email);
                  navigate("/app/today", { replace: true });
                }
              } catch (err) {
                setError(errorText(err));
              } finally {
                setBusy(false);
              }
            }}
          >
            <span className={s.eyebrow}>
              {register ? "НАЧАЛО ЧЕГО-ТО ХОРОШЕГО" : "РАДЫ ВИДЕТЬ ТЕБЯ СНОВА"}
            </span>
            <h1>{register ? "Место для твоих планов." : "С возвращением."}</h1>
            <p>
              {register
                ? "Создай аккаунт и освободи голову для важного."
                : "Твои задачи и немного спокойствия уже здесь."}
            </p>
            <label className={s.field}>
              Email
              <input
                required
                type="email"
                autoComplete="email"
                placeholder="you@example.com"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
              />
            </label>
            <label className={s.field}>
              Пароль
              <input
                required
                type="password"
                minLength={8}
                maxLength={72}
                autoComplete={register ? "new-password" : "current-password"}
                placeholder="Не менее 8 символов"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
            </label>
            {register && (
              <label className={s.consent}>
                <input
                  required
                  type="checkbox"
                  checked={accepted}
                  onChange={(e) => setAccepted(e.target.checked)}
                />
                <span>
                  Принимаю{" "}
                  <a href="/offer.html" target="_blank" rel="noreferrer">
                    оферту
                  </a>{" "}
                  и{" "}
                  <a href="/policy.html" target="_blank" rel="noreferrer">
                    политику конфиденциальности
                  </a>
                  .
                </span>
              </label>
            )}
            {error && (
              <p role="alert" className={s.error}>
                {error}
              </p>
            )}
            <Button disabled={busy || (register && !accepted)}>
              {busy
                ? "Подожди немного…"
                : register
                  ? "Создать аккаунт"
                  : "Войти"}
              <ArrowUpRight size={17} />
            </Button>
            <p className={s.authSwitch}>
              {register ? "Уже есть аккаунт?" : "Первый раз здесь?"}{" "}
              <Link to={register ? "/login" : "/register"}>
                {register ? "Войти" : "Создать аккаунт"}
              </Link>
            </p>
          </form>
        )}
      </main>
    </div>
  );
}
export function Verification({ token }: { token: string }) {
  const client = useQueryClient();
  const [state, setState] = useState<"loading" | "success" | "error">(
    "loading",
  );
  const [message, setMessage] = useState("");
  const started = useRef(false);
  const verify = async () => {
    setState("loading");
    try {
      await api(`/verify?token=${encodeURIComponent(token)}`, {}, false);
      setState("success");
      history.replaceState(null, "", location.pathname);
      if (hasSession()) {
        try {
          await refreshAccess();
          await client.invalidateQueries({ queryKey: ["bootstrap"] });
        } catch {
          setMessage(
            "Email подтверждён. Если напоминания недоступны, войди заново.",
          );
        }
      }
    } catch (e) {
      setMessage(errorText(e));
      setState("error");
    }
  };
  useEffect(() => {
    if (!started.current) {
      started.current = true;
      void verify();
    }
  }, []);
  return (
    <div className={s.verification}>
      <Brand />
      <State
        loading={state === "loading"}
        title={
          state === "loading"
            ? "Подтверждаем email…"
            : state === "success"
              ? "Email подтверждён"
              : "Не удалось подтвердить email"
        }
        retry={state === "error" ? () => void verify() : undefined}
      >
        {state === "success"
          ? "Теперь важные напоминания будут приходить на почту."
          : message}
      </State>
      {state !== "loading" && (
        <Link
          className={s.button}
          to={hasSession() ? "/app/account" : "/login"}
        >
          Продолжить <ArrowUpRight size={16} />
        </Link>
      )}
    </div>
  );
}
