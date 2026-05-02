import { useState, useCallback } from "react";
import { useTimer } from "./useTimer";
import { api } from "../api/api";
import { todoStore } from "../stores/TodoStore";
import { useMutation } from "@tanstack/react-query";

interface PomodoroSession {
  id: number;
  task_id?: number;
  duration_minutes: number;
}

export const usePomodoro = () => {
  const [session, setSession] = useState<PomodoroSession | null>(null);
  const [mode, setMode] = useState<"focus" | "break">("focus");

  // 1. Мутация для старта
  const startMutation = useMutation({
    mutationFn: async (taskId?: number) => {
      const { data } = await api.post("/pomodoros/start", { task_id: taskId });
      return data.session;
    },
    onSuccess: (data: PomodoroSession) => {
      setSession(data);
      setMode("focus");
      start(data.duration_minutes * 60);
    },
    onError: (error: any) => {
      const status = error.response?.status;
      if (status === 409) {
        todoStore.showToast("Сессия уже запущена на сервере", "error");
      } else {
        todoStore.showToast("Ошибка при запуске таймера", "error");
      }
    },
  });

  // 2. Мутация для стопа
  const stopMutation = useMutation({
    mutationFn: async (action: "abandoned" | "completed") => {
      if (!session) return;
      await api.post(`/pomodoros/${session.id}/stop`, { action });
    },
    onSuccess: () => {
      stop();
      setSession(null);
      setMode("focus");
    },
    onError: () => {
      todoStore.showToast("Не удалось остановить сессию", "error");
    },
  });

  // Колбэк по окончанию таймера
  const handleExpire = useCallback(() => {
    if (mode === "focus") {
      // Когда время вышло, завершаем сессию как выполненную
      stopMutation.mutate("completed");
      // И переключаем на отдых (локально)
      setMode("break");
      start(5 * 60);
    } else {
      setSession(null);
      setMode("focus");
    }
  }, [mode, session, stopMutation]);

  const { timeLeft, start, stop, formatTime } = useTimer(0, handleExpire);

  return {
    session,
    mode,
    timeLeft,
    formatTime,
    isActive: !!session,
    isLoading: startMutation.isPending || stopMutation.isPending,
    startSession: (taskId?: number) => startMutation.mutate(taskId),
    stopSession: () => stopMutation.mutate("abandoned"),
  };
};
