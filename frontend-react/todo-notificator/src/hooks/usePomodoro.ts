import { useCallback } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Modal } from "antd"; // Импортируем модалку из Antd
import { api } from "../api/api";
import { todoStore } from "../stores/TodoStore";
import { pomodoroStore } from "../stores/PomodoroStore";

export const usePomodoro = () => {
  const queryClient = useQueryClient();

  const startMutation = useMutation({
    mutationFn: async (taskId?: number) => {
      const { data } = await api.post("/pomodoros/start", { task_id: taskId });
      return data.session || data.active_session;
    },
    onSuccess: (data) => {
      pomodoroStore.setSession(data);
      pomodoroStore.start(data.duration_minutes * 60);
      queryClient.invalidateQueries({ queryKey: ["stats"] });
    },
    onError: (error: any) => {
      const status = error.response?.status;
      const data = error.response?.data;
      const activeSession = data?.active_session;

      if (status === 409 && activeSession) {
        pomodoroStore.setSession(activeSession);
        pomodoroStore.start(activeSession.duration_minutes * 60);
        todoStore.showToast("Сессия уже запущена", "error");
      } else {
        todoStore.showToast(data?.error || "Ошибка старта", "error");
      }
    },
  });

  const stopMutation = useMutation({
    mutationFn: async (action: "abandoned" | "completed") => {
      const session = pomodoroStore.session;
      if (!session) return;
      await api.post(`/pomodoros/${session.id}/stop`, { action });
    },
    onSuccess: () => {
      pomodoroStore.stop();
      queryClient.invalidateQueries({ queryKey: ["stats"] });
      queryClient.invalidateQueries({ queryKey: ["tasks"] });
    },
    onError: () => {
      todoStore.showToast("Не удалось остановить таймер", "error");
    },
  });

  const startSession = useCallback(
    (taskId?: number) => {
      if (pomodoroStore.isActive) {
        todoStore.showToast("Сначала завершите текущую сессию", "error");
        return;
      }
      startMutation.mutate(taskId);
    },
    [startMutation],
  );

  const stopSession = useCallback(() => {
    const isHardMode = !!pomodoroStore.session?.task_id;

    if (isHardMode) {
      // Используем Ant Design Modal вместо window.confirm
      Modal.confirm({
        title: "Вы уверены, что хотите прервать фокус?",
        content: "Прогресс по текущей задаче не будет сохранен.",
        okText: "Да, прервать",
        okType: "danger",
        cancelText: "Отмена",
        onOk() {
          stopMutation.mutate("abandoned");
        },
        // onCancel ничего не делает, просто закрывает окно
      });
    } else {
      // Если свободный режим — стопаем сразу
      stopMutation.mutate("abandoned");
    }
  }, [stopMutation]);

  return {
    isLoading: startMutation.isPending || stopMutation.isPending,
    startSession,
    stopSession,
  };
};
