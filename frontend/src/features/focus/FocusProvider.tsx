import {
  createContext,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiError, errorText } from "../../lib/api";
import { command, rewardLabel } from "../../lib/commands";
import type { Session, FocusSnapshot, ActionResponse } from "../../lib/types";
import { useApp } from "../../app/context";

export function secondsLeft(session: Session | null, now: number) {
  if (!session) return 1500;
  return Math.min(
    session.duration_minutes * 60,
    Math.max(
      0,
      Math.ceil(
        (Date.parse(session.started_at) +
          session.duration_minutes * 60000 -
          now) /
          1000,
      ),
    ),
  );
}
// Compatibility helper for consumers of the old read-only endpoint.
export async function loadActiveSession(signal?: AbortSignal) {
  try {
    const result = await api<{ active_session?: Session | null }>(
      "/pomodoros/active",
      { signal },
    );
    return { session: result.active_session || null };
  } catch (error) {
    if (error instanceof ApiError && error.status === 404)
      return { session: null };
    throw error;
  }
}
type Phase = "idle" | "focus" | "extra-break" | "rest";
interface FocusValue {
  session: Session | null;
  seconds: number;
  phase: Phase;
  busy: boolean;
  error: string;
  start: (taskId?: number) => Promise<void>;
  stop: (
    action: "completed" | "abandoned",
    finish: boolean,
    rest?: boolean,
  ) => Promise<boolean>;
  pause: () => Promise<boolean>;
  resume: () => void;
  finishRest: () => void;
  retry: () => void;
}
const Context = createContext<FocusValue | null>(null);
export const useFocus = () => {
  const value = useContext(Context);
  if (!value) throw new Error("Missing focus provider");
  return value;
};
export function FocusProvider({ children }: { children: ReactNode }) {
  const { data, notify, refresh, award } = useApp();
  const client = useQueryClient();
  const query = useQuery({
    queryKey: ["focus"],
    queryFn: ({ signal }) =>
      api<{ focus: FocusSnapshot }>("/me/focus", { signal }),
    ...(data.focus ? { initialData: { focus: data.focus } } : {}),
    staleTime: 5000,
    refetchInterval: 15000,
  });
  const [now, setNow] = useState(Date.now());
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const running = useRef(false);
  const attempted = useRef<number | null>(null);
  useEffect(() => {
    const tick = () => setNow(Date.now());
    const timer = setInterval(tick, 250);
    document.addEventListener("visibilitychange", tick);
    return () => {
      clearInterval(timer);
      document.removeEventListener("visibilitychange", tick);
    };
  }, []);
  const state = query.data?.focus;
  const session = state?.session || null;
  const elapsed = Math.max(0, now - query.dataUpdatedAt);
  const serverNow = state ? Date.parse(state.server_now) + elapsed : now;
  const breakSeconds = state?.phase_ends_at
    ? Math.max(
        0,
        Math.ceil((Date.parse(state.phase_ends_at) - serverNow) / 1000),
      )
    : 0;
  const phase: Phase =
    state?.phase === "recovery_break"
      ? "rest"
      : state?.phase === "extra_break" && breakSeconds > 0
        ? "extra-break"
        : session
          ? "focus"
          : "idle";
  const workSeconds = state
    ? Math.max(0, state.remaining_work_seconds - Math.floor(elapsed / 1000))
    : 1500;
  const seconds =
    phase === "rest" || phase === "extra-break"
      ? breakSeconds
      : phase === "idle"
        ? 1500
        : workSeconds;
  const perform = async (path: string, body?: unknown) => {
    if (running.current) return null;
    running.current = true;
    setBusy(true);
    setError("");
    await client.cancelQueries({ queryKey: ["focus"] });
    try {
      const result = await command(data.user.id, path, body);
      client.setQueryData(["focus"], { focus: result.focus });
      setNow(Date.now());
      if (result.result?.pomodoro_counted && session)
        award(`session:${session.id}`, 10);
      const suffix = rewardLabel(result);
      if (suffix) notify(suffix);
      await refresh();
      return result;
    } catch (error) {
      if (
        path === "/pomodoros/start" &&
        error instanceof ApiError &&
        error.status === 409
      ) {
        setError("");
        await query.refetch();
        notify("Восстановлена активная сессия.");
        return null;
      }
      setError(errorText(error));
      return null;
    } finally {
      running.current = false;
      setBusy(false);
    }
  };
  const start = async (taskId?: number) => {
    const result = await perform(
      "/pomodoros/start",
      taskId ? { task_id: taskId } : {},
    );
    if (result) {
      attempted.current = null;
      notify("Время сосредоточиться. Сессия началась.");
    } else if (!running.current) await query.refetch();
  };
  const stop = async (action: "completed" | "abandoned", finish: boolean) => {
    if (!session) return false;
    return !!(await perform(`/pomodoros/${session.id}/stop`, {
      action,
      finish_task: finish,
    }));
  };
  const pause = async () =>
    session ? !!(await perform(`/pomodoros/${session.id}/pause`)) : false;
  const resume = () => {
    void perform("/me/focus/resume");
  };
  useEffect(() => {
    if (
      session &&
      phase === "focus" &&
      seconds === 0 &&
      !running.current &&
      attempted.current !== session.id
    ) {
      attempted.current = session.id;
      void stop("completed", false);
    }
  }, [session?.id, phase, seconds]);
  return (
    <Context.Provider
      value={{
        session,
        seconds,
        phase,
        busy: busy || query.isPending,
        error: error || (query.isError ? errorText(query.error) : ""),
        start,
        stop,
        pause,
        resume,
        finishRest: () => {
          void perform("/me/focus/rest/finish");
        },
        retry: () => {
          setError("");
          if (session && phase === "focus" && seconds === 0)
            void stop("completed", false);
          else void query.refetch();
        },
      }}
    >
      {children}
    </Context.Provider>
  );
}
