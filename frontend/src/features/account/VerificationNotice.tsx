import { useEffect, useState } from "react";
import { Link, useLocation, useNavigate } from "react-router-dom";
import { useApp } from "../../app/context";
import { api, errorText } from "../../lib/api";
import { Button, Sheet } from "../../components/ui";
import s from "../../styles/App.module.css";

export function VerificationNotice() {
  const { data, notify } = useApp();
  const location = useLocation();
  const navigate = useNavigate();
  const [open, setOpen] = useState(Boolean(location.state?.registered));
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    if (location.state?.registered)
      navigate(location.pathname + location.search, {
        replace: true,
        state: null,
      });
  }, [location.state, location.pathname, location.search, navigate]);
  const resend = async () => {
    setBusy(true);
    try {
      await api("/verify/resend", { method: "POST" });
      notify("Письмо подтверждения поставлено в очередь и скоро придёт.");
    } catch (err) {
      notify(errorText(err));
    } finally {
      setBusy(false);
    }
  };
  if (data.user.is_verified) return null;
  const resendButton = (
    <Button type="button" quiet disabled={busy} onClick={() => void resend()}>
      Отправить письмо ещё раз
    </Button>
  );
  return (
    <>
      <aside className={s.verificationBanner} aria-label="Подтверждение почты">
        <p>
          Можно пользоваться приложением сразу. Email-уведомления начнут
          приходить после подтверждения почты.
        </p>
        <div className={s.row}>
          {resendButton}
          <Link to="/app/account">Настройки почты</Link>
        </div>
      </aside>
      <Sheet
        open={open}
        onClose={() => setOpen(false)}
        title="Аккаунт готов"
        description="Можно пользоваться приложением сразу. Email-уведомления начнут приходить после подтверждения почты."
        wide
      >
        <div className={s.row}>
          <Button onClick={() => setOpen(false)}>Продолжить</Button>
          {resendButton}
        </div>
      </Sheet>
    </>
  );
}
