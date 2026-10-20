import {
  useEffect,
  useState,
  Component,
  type ReactNode,
  type ErrorInfo,
} from "react";
import {
  BrowserRouter,
  Navigate,
  NavLink,
  Outlet,
  Route,
  Routes,
  Link,
  useNavigate,
} from "react-router-dom";
import {
  QueryClient,
  QueryClientProvider,
  useQueryClient,
} from "@tanstack/react-query";

import {
  CalendarDays,
  ListTodo,
  Timer,
  ChartNoAxesCombined,
  Folder,
  Settings2,
  ArrowUpRight,
  MoreHorizontal,
  Gift,
} from "lucide-react";
import {
  api,
  ApiError,
  errorText,
  hasSession,
  refreshAccess,
  sessionStorageChanged,
} from "../lib/api";
import { Brand, State } from "../components/ui";
import { AppProvider, useApp, useBootstrap } from "./context";
import { FocusProvider, FocusPage, useFocus } from "../features/focus/Focus";
import { TasksPage } from "../features/tasks/Tasks";
import { CategoriesPage } from "../features/categories/Categories";
import { ProgressPage } from "../features/progress/Progress";
import { RewardsPage } from "../features/rewards/Rewards";
import { AccountPage, setTheme } from "../features/account/Account";
import { PasswordRecovery } from "../features/public/PasswordRecovery";
import { VerificationNotice } from "../features/account/VerificationNotice";
import { Landing, AuthPage, Verification } from "../features/public/Public";
import { useShop } from "../lib/shop";
import s from "../styles/App.module.css";

const client = new QueryClient({
  defaultOptions: {
    queries: {
      retry: (count, error) =>
        !(
          error instanceof ApiError &&
          [400, 401, 403, 404].includes(error.status)
        ) && count < 1,
      staleTime: 15_000,
      refetchOnWindowFocus: true,
    },
  },
});
const links = [
  ["today", CalendarDays, "Сегодня"],
  ["tasks", ListTodo, "Задачи"],
  ["focus", Timer, "Фокус"],
  ["more", MoreHorizontal, "Ещё"],
] as const;
function Protected() {
  const navigate = useNavigate();
  const cache = useQueryClient();
  const query = useBootstrap();
  useEffect(() => {
    const ended = () => {
      void cache.cancelQueries();
      cache.clear();
      navigate("/login", { replace: true });
    };
    const storage = (event: StorageEvent) => {
      if (event.key === "access_token" || event.key === "refresh_token") {
        if (!sessionStorageChanged()) return;
        void cache.cancelQueries();
        cache.clear();
        navigate(hasSession() ? "/app/today" : "/login", { replace: true });
      }
    };
    window.addEventListener("session-ended", ended);
    window.addEventListener("storage", storage);
    return () => {
      window.removeEventListener("session-ended", ended);
      window.removeEventListener("storage", storage);
    };
  }, [navigate, cache]);
  if (!hasSession()) return <Navigate to="/login" replace />;
  if (query.isPending)
    return (
      <div className={s.boot}>
        <Brand />
        <State title="Готовим твоё пространство…" loading />
      </div>
    );
  if (query.isError)
    return (
      <div className={s.boot}>
        <Brand />
        <State
          title="Не удалось открыть приложение"
          retry={() => void query.refetch()}
        >
          {errorText(query.error)}
        </State>
        <Link to="/">На главную</Link>
      </div>
    );
  return (
    <AppProvider data={query.data}>
      <FocusProvider>
        <Shell />
      </FocusProvider>
    </AppProvider>
  );
}
function Shell() {
  const { data, notify, refresh } = useApp();
  const focus = useFocus();
  const wallet = useShop(data.user.id);
  const [paymentHandled, setPaymentHandled] = useState(false);
  useEffect(() => {
    document.documentElement.dataset.accent =
      wallet.data?.wallet.activeTheme || "default";
    localStorage.setItem(
      "todo:active-sound",
      wallet.data?.wallet.activeSound || "default",
    );
  }, [wallet.data?.wallet.activeTheme, wallet.data?.wallet.activeSound]);
  useEffect(() => {
    if (!new URLSearchParams(location.search).has("payment") || paymentHandled)
      return;
    setPaymentHandled(true);
    void (async () => {
      try {
        const result = await api<{ is_premium?: boolean }>("/payments/sync", {
          method: "POST",
        });
        if (result.is_premium) await refreshAccess();
        await refresh();
        notify(
          result.is_premium
            ? "Premium активирован. Спасибо за поддержку!"
            : "Платёж пока не подтверждён. Проверь статус в аккаунте.",
        );
      } catch (error) {
        notify(errorText(error));
      }
    })();
  }, [paymentHandled, notify, refresh]);
  return (
    <div className={s.app}>
      <a className={s.skip} href="#main">
        К содержимому
      </a>
      <aside className={s.sidebar}>
        <Brand />
        <nav className={s.sideNav}>
          {links.map(([path, Icon, title]) => (
            <NavLink
              key={path}
              to={`/app/${path}`}
              className={({ isActive }) => (isActive ? s.navActive : "")}
            >
              <Icon size={19} />
              {title}
            </NavLink>
          ))}
        </nav>
        <div className={s.sidebarBottom}>
          {(focus.session || focus.phase === "rest") && (
            <Link to="/app/focus" className={s.sidebarTimer}>
              <Timer size={18} />
              <span>
                <strong>
                  {String(Math.floor(focus.seconds / 60)).padStart(2, "0")}:
                  {String(focus.seconds % 60).padStart(2, "0")}
                </strong>
                <small>
                  {focus.phase === "focus" ? "Сессия фокуса" : "Перерыв"}
                </small>
              </span>
              <ArrowUpRight size={16} />
            </Link>
          )}
        </div>
      </aside>
      <div className={s.appBody}>
        <header className={s.shellHeader}>
          <Brand />
        </header>
        <main id="main" className={s.content}>
          <VerificationNotice />
          <Outlet />
        </main>
        <footer className={s.appFooter}>
          <a href="/policy.html">
            Конфиденциальность <ArrowUpRight size={12} />
          </a>
        </footer>
      </div>
      {(focus.session || focus.phase === "rest") && (
        <Link to="/app/focus" className={s.mobileTimer}>
          <Timer size={17} />
          <strong>
            {String(Math.floor(focus.seconds / 60)).padStart(2, "0")}:
            {String(focus.seconds % 60).padStart(2, "0")}
          </strong>
          <span>Вернуться к фокусу</span>
          <ArrowUpRight size={16} />
        </Link>
      )}
      <nav className={s.bottomNav}>
        {links.map(([path, Icon, label]) => (
            <NavLink
              key={path}
              to={`/app/${path}`}
              className={({ isActive }) => (isActive ? s.bottomActive : "")}
            >
              <Icon size={21} />
              <span>{label}</span>
            </NavLink>
          ))}
      </nav>
    </div>
  );
}
function More() {
  return (
    <>
      <h1>Ещё</h1>
      <div className={s.moreLinks}>
        {[
          [ChartNoAxesCombined, "Прогресс", "progress"],
          [Folder, "Категории", "categories"],
          [Gift, "Магазин и награды", "rewards"],
          [Settings2, "Аккаунт и настройки", "account"],
        ].map(([Icon, title, path]) => {
          const Symbol = Icon as typeof Folder;
          return (
            <Link key={String(path)} to={`/app/${path}`}>
              <Symbol />
              {String(title)}
              <ArrowUpRight size={18} />
            </Link>
          );
        })}
      </div>
    </>
  );
}
function Entry() {
  const [token] = useState(() =>
    new URLSearchParams(location.search).get("token"),
  );
  if (token) return <Verification token={token} />;
  if (hasSession())
    return <Navigate to={`/app/today${location.search}`} replace />;
  return <Landing />;
}
class ErrorBoundary extends Component<
  { children: ReactNode },
  { failed: boolean }
> {
  state = { failed: false };
  static getDerivedStateFromError() {
    return { failed: true };
  }
  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error("UI failed", error, info.componentStack);
  }
  render() {
    return this.state.failed ? (
      <div className={s.boot}>
        <State
          title="Не удалось показать страницу"
          retry={() => location.reload()}
        >
          Обнови страницу. Сохранённые на сервере задачи останутся на месте.
        </State>
      </div>
    ) : (
      this.props.children
    );
  }
}
export function App() {
  useEffect(() => {
    const change = () => {
      if ((localStorage.getItem("todo:theme") || "system") === "system")
        setTheme("system");
    };
    const media = matchMedia("(prefers-color-scheme: dark)");
    media.addEventListener("change", change);
    return () => media.removeEventListener("change", change);
  }, []);
  return (
    <ErrorBoundary>
      <QueryClientProvider client={client}>
        <BrowserRouter>
          <Routes>
            <Route path="/" element={<Entry />} />
            <Route
              path="/forgot-password"
              element={<PasswordRecovery key="forgot" />}
            />
            <Route
              path="/reset-password"
              element={<PasswordRecovery key="reset" reset />}
            />
            <Route path="/login" element={<AuthPage />} />
            <Route path="/register" element={<AuthPage register />} />
            <Route path="/frontend" element={<Navigate to="/" replace />} />
            <Route path="/app" element={<Protected />}>
              <Route index element={<Navigate to="today" replace />} />
              <Route path="today" element={<TasksPage today />} />
              <Route path="tasks" element={<TasksPage />} />
              <Route path="focus" element={<FocusPage />} />
              <Route path="categories" element={<CategoriesPage />} />
              <Route path="progress" element={<ProgressPage />} />
              <Route path="rewards" element={<RewardsPage />} />
              <Route path="account" element={<AccountPage />} />
              <Route path="more" element={<More />} />
            </Route>
            <Route
              path="*"
              element={
                <div className={s.boot}>
                  <State title="Такой страницы нет" />
                  <Link className={s.button} to="/">
                    На главную
                  </Link>
                </div>
              }
            />
          </Routes>
        </BrowserRouter>
      </QueryClientProvider>
    </ErrorBoundary>
  );
}
