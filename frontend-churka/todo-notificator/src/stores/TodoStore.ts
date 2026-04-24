import { makeAutoObservable } from "mobx";
import { type TaskStatus, type Task, type ToastMessage } from "../types/task";

class TodoStore {
  // Фильтры интерфейса
  filter: "all" | TaskStatus | "logs" = "all";

  // Пагинация (как в OpenAPI: limit = 20, offset = 0)
  pagination = { limit: 20, offset: 0, total: 0 };

  toasts: ToastMessage[] = [];

  // Форма новой задачи
  newTask = {
    title: "",
    description: "",
    deadline: "",
    reminder_at: "",
    category_id: null as number | null,
  };

  // Редактирование задачи
  editingTaskId: number | null = null;
  editForm = {
    title: "",
    description: "",
    deadline: "",
    reminder_at: "",
    status: "pending" as TaskStatus,
    category_id: null as number | null,
  };

  constructor() {
    makeAutoObservable(this);
  }

  // --- УПРАВЛЕНИЕ ФИЛЬТРАМИ ---
  setFilter(filter: typeof this.filter) {
    this.filter = filter;
  }

  // Клиентская фильтрация задач (поскольку сервер отдает все)
  getFilteredTasks(tasks: Task[]) {
    if (this.filter === "all" || this.filter === "logs") return tasks;
    return tasks.filter((t) => t.status === this.filter);
  }

  // --- УПРАВЛЕНИЕ ПАГИНАЦИЕЙ ---
  setTotal(total: number) {
    this.pagination.total = total;
  }

  changePage(direction: number) {
    this.pagination.offset = Math.max(
      0,
      this.pagination.offset + direction * this.pagination.limit,
    );
  }

  resetPagination() {
    this.pagination.offset = 0;
  }

  // --- УПРАВЛЕНИЕ ФОРМАМИ ---
  setNewTaskField<K extends keyof typeof this.newTask>(
    field: K,
    value: (typeof this.newTask)[K],
  ) {
    this.newTask[field] = value;
  }

  resetNewTask() {
    this.newTask = {
      title: "",
      description: "",
      deadline: "",
      reminder_at: "",
      category_id: null,
    };
  }

  // Подготовка формы редактирования (конвертация дат для datetime-local)
  startEdit(task: Task) {
    this.editingTaskId = task.id;
    this.editForm = {
      title: task.title,
      description: task.description || "",
      deadline: this.toDateTimeLocal(task.deadline),
      reminder_at: this.toDateTimeLocal(task.reminder_at),
      status: task.status,
      category_id: task.category_id,
    };
  }

  cancelEdit() {
    this.editingTaskId = null;
  }

  setEditFormField<K extends keyof typeof this.editForm>(
    field: K,
    value: (typeof this.editForm)[K],
  ) {
    this.editForm[field] = value;
  }
  // --- УТИЛИТЫ ---
  // Конвертация серверной даты (UTC) в формат для инпута datetime-local
  toDateTimeLocal(dateStr: string | null) {
    if (!dateStr) return "";
    const d = new Date(dateStr);
    if (Number.isNaN(d.getTime())) return "";
    const tzOffsetMs = d.getTimezoneOffset() * 60000;
    const local = new Date(d.getTime() - tzOffsetMs);
    return local.toISOString().slice(0, 16);
  }

  // Конвертация локальной даты в UTC для бэкенда
  toUTCString(dateStr: string) {
    if (!dateStr) return null;
    return new Date(dateStr).toISOString();
  }

  showToast(text: string, type: "success" | "error" = "success") {
    const id = Date.now();
    this.toasts.push({ id, text, type });

    // Автоматическое удаление через 3 секунды
    setTimeout(() => {
      this.removeToast(id);
    }, 3000);
  }

  // Удалить конкретное уведомление
  removeToast(id: number) {
    this.toasts = this.toasts.filter((t) => t.id !== id);
  }
}

export const todoStore = new TodoStore();
