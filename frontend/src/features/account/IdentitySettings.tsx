import { useState, type FormEvent, type ReactNode } from "react";
import { useApp } from "../../app/context";
import { api, ApiError, errorText, json, saveTokens } from "../../lib/api";
import { Button } from "../../components/ui";
import s from "../../styles/App.module.css";

function SettingsForm({
  title,
  children,
  save,
}: {
  title: string;
  children: ReactNode;
  save: () => Promise<void>;
}) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      await save();
    } catch (err) {
      setError(errorText(err));
    } finally {
      setBusy(false);
    }
  };
  return (
    <form className={s.card} onSubmit={(e) => void submit(e)}>
      <h2>{title}</h2>
      <fieldset disabled={busy} className={s.identityFields}>
        {children}
        {error && (
          <p className={s.error} role="alert">
            {error}
          </p>
        )}
        <Button disabled={busy}>{busy ? "Сохраняем…" : "Сохранить"}</Button>
      </fieldset>
    </form>
  );
}

export function IdentitySettings() {
  const { data, refresh, notify } = useApp();
  const [name, setName] = useState(data.user.display_name || "");
  const [email, setEmail] = useState(data.user.email);
  const [emailPassword, setEmailPassword] = useState("");
  const [current, setCurrent] = useState("");
  const [password, setPassword] = useState("");
  const [repeat, setRepeat] = useState("");
  const change = async (path: string, body: unknown) => {
    const result = await api<{ access_token?: string; refresh_token?: string }>(
      path,
      { method: "POST", body: json(body) },
    );
    if (result.access_token)
      saveTokens({ ...result, access_token: result.access_token });
    await refresh();
  };
  return (
    <div className={s.settingsGrid}>
      <SettingsForm
        title="Отображаемое имя"
        save={async () => {
          await api("/me/profile", {
            method: "PATCH",
            body: json({ display_name: name }),
          });
          await refresh();
          notify("Имя сохранено.");
        }}
      >
        <p className={s.muted}>
          Необязательно. Если оставить пустым, показывается часть почты до @.
        </p>
        <label className={s.field}>
          Имя
          <input
            autoComplete="nickname"
            maxLength={80}
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
        </label>
      </SettingsForm>
      <SettingsForm
        title="Сменить почту"
        save={async () => {
          await change("/me/email", { email, current_password: emailPassword });
          setEmailPassword("");
          localStorage.setItem("user_email", email.trim());
          notify("Почта сохранена.");
        }}
      >
        <p className={s.muted}>
          Новый адрес сразу станет логином. Email-уведомления будут
          приостановлены до его подтверждения. Остальные устройства выйдут из
          аккаунта.
        </p>
        <label className={s.field}>
          Новая почта
          <input
            required
            type="email"
            autoComplete="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
          />
        </label>
        <label className={s.field}>
          Текущий пароль для смены почты
          <input
            required
            type="password"
            autoComplete="current-password"
            value={emailPassword}
            onChange={(e) => setEmailPassword(e.target.value)}
          />
        </label>
      </SettingsForm>
      <SettingsForm
        title="Сменить пароль"
        save={async () => {
          if (password !== repeat)
            throw new ApiError(400, "Пароли не совпадают");
          if (
            new TextEncoder().encode(password).length > 72 ||
            Array.from(password).length < 8
          )
            throw new ApiError(
              400,
              "Пароль должен содержать не менее 8 символов и не более 72 байт",
            );
          await change("/me/password", {
            current_password: current,
            new_password: password,
          });
          setCurrent("");
          setPassword("");
          setRepeat("");
          notify("Пароль изменён. Остальные сессии завершены.");
        }}
      >
        <label className={s.field}>
          Текущий пароль
          <input
            required
            type="password"
            autoComplete="current-password"
            value={current}
            onChange={(e) => setCurrent(e.target.value)}
          />
        </label>
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
      </SettingsForm>
    </div>
  );
}
