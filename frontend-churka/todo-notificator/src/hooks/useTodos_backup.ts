import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/api";
import { todoStore } from "../stores/TodoStore";
import { type Task, type Pagination } from "../types/task";
import { type AxiosError } from "axios";

// 1. Получение списка задач
export const useTasksQuery = () => {
  return useQuery({
    queryKey: [
      "tasks",
      todoStore.pagination.offset,
      todoStore.pagination.limit,
    ],
    queryFn: async () => {
      const { data } = await api.get<{ tasks: Task[]; pagination: Pagination }>(
        "/tasks",
        {
          params: {
            limit: todoStore.pagination.limit,
            offset: todoStore.pagination.offset,
          },
        },
      );
      todoStore.setTotal(data.pagination.total);
      return data.tasks;
    },
  });
};

// 2. Создание задачи
export const useCreateTaskMutation = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async () => {
      const payload = {
        title: todoStore.newTask.title.trim(),
        description: todoStore.newTask.description.trim(),
        deadline: todoStore.toUTCString(todoStore.newTask.deadline),
        reminder_at: todoStore.toUTCString(todoStore.newTask.reminder_at),
        category_id: todoStore.newTask.category_id
          ? Number(todoStore.newTask.category_id)
          : null,
      };
      return await api.post("/tasks", payload);
    },
    onSuccess: () => {
      todoStore.resetNewTask();
      queryClient.invalidateQueries({ queryKey: ["tasks"] });
      queryClient.invalidateQueries({ queryKey: ["stats"] }); // Обновляем цифры в хедере
      todoStore.showToast("Задача создана!", "success");
    },
  });
};

// 3. Обновление задачи (ЕДИНАЯ ВЕРСИЯ)
export const useUpdateTaskMutation = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({
      taskId,
      fields,
    }: {
      taskId: number;
      fields: Partial<Task>;
    }) => {
      return await api.patch(`/tasks/${taskId}`, fields);
    },
    onSuccess: (_, variables) => {
      todoStore.cancelEdit();
      queryClient.invalidateQueries({ queryKey: ["tasks"] });
      queryClient.invalidateQueries({ queryKey: ["stats"] }); // Статистика важна при любом изменении статуса

      if (variables.fields.status === "done") {
        todoStore.showToast("Задача выполнена! 🏆", "success");
      } else {
        todoStore.showToast("Обновлено", "success");
      }
    },
    onError: (error: AxiosError) => {
      const msg =
        error.response?.status === 401
          ? "Ошибка авторизации"
          : "Ошибка обновления";
      todoStore.showToast(msg, "error");
    },
  });
};

// 4. Удаление задачи
export const useDeleteTaskMutation = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (taskId: number) => {
      await api.delete(`/tasks/${taskId}`);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["tasks"] });
      queryClient.invalidateQueries({ queryKey: ["stats"] }); // Чтобы total_tasks уменьшился
      todoStore.showToast("Задача удалена", "success");
    },
  });
};

// 5. Статистика
export const useStatsQuery = () => {
  return useQuery({
    queryKey: ["stats"],
    queryFn: async () => {
      const { data } = await api.get("/me/stats");
      return data;
    },
    staleTime: 1000 * 60,
  });
};
