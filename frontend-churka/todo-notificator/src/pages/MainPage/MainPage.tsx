/* eslint-disable @typescript-eslint/no-explicit-any */
import { useState, useEffect, useCallback, useMemo } from "react";
import TaskHeader from "../../components/TaskHeader/TaskHeader";
import TaskForm from "../../components/TaskForm/TaskForm";
import TaskList from "../../components/TaskList/TaskList";
import Toast, { type ToastMessage } from "../../components/Toast/Toast";
import "./MainPage.scss";

const MainPage = () => {
  const [tasks, setTasks] = useState<any[]>([]);
  const [total, setTotal] = useState(0);
  const [offset, setOffset] = useState(0);
  const [loading, setLoading] = useState(false);
  const [toasts, setToasts] = useState<ToastMessage[]>([]);
  const limit = 5;

  const addToast = (text: string, type: "success" | "error") => {
    setToasts((prev) => [...prev, { id: Date.now(), text, type }]);
  };

  // Вычисляем статистику на лету из загруженных задач (или можно брать из бэка)
  const stats = useMemo(() => {
    return {
      total: total,
      done: tasks.filter((t) => t.status === "done").length, // Упрощенно, лучше иметь отдельный эндпоинт
      pending: tasks.filter((t) => t.status === "pending").length,
    };
  }, [tasks, total]);

  const fetchTasks = useCallback(async () => {
    const token = localStorage.getItem("token");
    setLoading(true);
    try {
      const res = await fetch(
        `http://localhost:8082/api/v1/tasks?limit=${limit}&offset=${offset}`,
        {
          headers: { Authorization: `Bearer ${token}` },
        },
      );
      const data = await res.json();
      if (res.ok) {
        setTasks(data.tasks || []);
        setTotal(data.total || 0);
      }
    } catch (err) {
      addToast("Ошибка загрузки задач", "error");
    } finally {
      setLoading(false);
    }
  }, [offset]);

  useEffect(() => {
    fetchTasks();
  }, [fetchTasks]);

  const handleToggleStatus = async (id: number, currentStatus: string) => {
    const token = localStorage.getItem("token");
    const newStatus = currentStatus === "pending" ? "done" : "pending";

    try {
      const res = await fetch(`http://localhost:8082/api/v1/tasks/${id}`, {
        method: "PATCH",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify({ status: newStatus }),
      });

      if (res.ok) {
        addToast(
          newStatus === "done" ? "Задача выполнена! 🏆" : "Вернули в работу",
          "success",
        );
        fetchTasks();
      }
    } catch (err) {
      addToast("Не удалось обновить статус", "error");
    }
  };

  const handleDeleteTask = async (id: number) => {
    if (!window.confirm("Удалить задачу?")) return;

    const token = localStorage.getItem("token");
    try {
      const res = await fetch(`http://localhost:8082/api/v1/tasks/${id}`, {
        method: "DELETE",
        headers: { Authorization: `Bearer ${token}` },
      });

      if (res.ok) {
        addToast("Задача удалена", "success");
        // Если удалили последнюю задачу на странице, откатываем офсет
        if (tasks.length === 1 && offset > 0) {
          setOffset(offset - limit);
        } else {
          fetchTasks();
        }
      }
    } catch (err) {
      addToast("Ошибка при удалении", "error");
    }
  };

  const handleLogout = () => {
    localStorage.removeItem("token");
    window.location.href = "/auth";
  };

  return (
    <div className="main-layout">
      <Toast
        toasts={toasts}
        onClose={(id) => setToasts((p) => p.filter((t) => t.id !== id))}
      />

      <div className="container">
        <TaskHeader stats={stats} onLogout={handleLogout} />

        <div className="content-grid">
          <aside className="side-panel">
            <TaskForm
              onTaskCreated={() => {
                fetchTasks();
                addToast("Добавлено!", "success");
              }}
            />
          </aside>

          <main className="main-panel">
            <TaskList
              tasks={tasks}
              loading={loading}
              onToggle={handleToggleStatus}
              onDelete={handleDeleteTask}
            />
          </main>
        </div>
      </div>
    </div>
  );
};

export default MainPage;
