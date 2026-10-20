import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api, ApiError, endSession, errorText, json } from "../../lib/api";
import { useQueryClient } from "@tanstack/react-query";
import { Brand, Button } from "../../components/ui";
import s from "../../styles/App.module.css";

export function PasswordRecovery({ reset = false }: { reset?: boolean }) {
  const client = useQueryClient();
  const [token] = useState(
    () => new URLSearchParams(location.search).get("token") || "",
  );
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [repeat, setRepeat] = useState("");
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState(false);
  const [error, setError] = useState("");
  const [invalid, setInvalid] = useState(reset && !token);
  useEffect(() => {
    if (!reset) return;
    const previous = document.querySelector<HTMLMetaElement>(
      'meta[name="referrer"]',
    );
    const meta = document.createElement("meta");
    meta.name = "referrer";
    meta.content = "no-referrer";
    previous?.remove();
    document.head.append(meta);
    // Keep the secret in memory instead of the address bar and browser history.
    history.replaceState(null, "", location.pathname);
    return () => {
      meta.remove();
      if (previous) document.head.append(previous);
    };
  }, [reset]);
  return (
    <main className={s.verification}>
      <Brand />
      <form
        className={s.authForm}
        onSubmit={async (e) => {
          e.preventDefault();
          setError("");
          if (reset && password !== repeat) {
            setError("Пароли не совпадают");
            return;
          }
          if (
            reset &&
            (new TextEncoder().encode(password).length > 72 ||
              Array.from(password).length < 8)
          ) {
            setError(
              "Пароль должен содержать не менее 8 символов и не более 72 байт",
            );
            return;
          }
          setBusy(true);
          try {
            await api(
              reset ? "/password/reset" : "/password/forgot",
              {
                method: "POST",
                body: json(
                  reset ? { token, new_password: password } : { email },
                ),
              },
              false,
            );
            if (reset) {
              endSession();
              client.clear();
              setPassword("");
              setRepeat("");
            }
            setDone(true);
          } catch (err) {
            if (
              err instanceof ApiError &&
              err.data.code === "INVALID_RESET_TOKEN"
            )
              setInvalid(true);
            else setError(errorText(err));
          } finally {
            setBusy(false);
          }
        }}
      >
        <h1>
          {done
            ? reset
              ? "Пароль изменён"
              : "Проверьте почту"
            : reset
              ? "Новый пароль"
              : "Восстановление пароля"}
        </h1>
        {done ? (
          <p>
            {reset
              ? "Все прежние сессии завершены. Войдите с новым паролем."
              : "Если аккаунт с такой почтой существует, мы отправим ссылку для восстановления"}
          </p>
        ) : invalid ? (
          <p role="alert">
            Ссылка недействительна или срок её действия истёк. Запросите новое
            письмо.
          </p>
        ) : (
          <>
            {reset ? (
              <>
                <label className={s.field}>
                  Новый пароль
                  <input
                    required
                    type="password"
                    autoComplete="new-password"
                    minLength={8}
                    maxLength={72}
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                  />
                </label>
                <label className={s.field}>
                  Повторите новый пароль
                  <input
                    required
                    type="password"
                    autoComplete="new-password"
                    value={repeat}
                    onChange={(e) => setRepeat(e.target.value)}
                  />
                </label>
              </>
            ) : (
              <label className={s.field}>
                Email
                <input
                  required
                  type="email"
                  autoComplete="email"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                />
              </label>
            )}
            {error && (
              <p role="alert" className={s.error}>
                {error}
              </p>
            )}
            <Button disabled={busy}>
              {busy
                ? "Подождите…"
                : reset
                  ? "Сохранить пароль"
                  : "Отправить ссылку"}
            </Button>
          </>
        )}
        {invalid && <Link to="/forgot-password">Запросить новое письмо</Link>}
        <Link to="/login">Перейти ко входу</Link>
      </form>
    </main>
  );
}
