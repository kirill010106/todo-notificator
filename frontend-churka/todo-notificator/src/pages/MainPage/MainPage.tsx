import { observer } from "mobx-react-lite";
import TaskHeader from "../../components/TaskHeader/TaskHeader";
import TaskForm from "../../components/TaskForm/TaskForm";
import TaskList from "../../components/TaskList/TaskList";
import Toast from "../../components/Toast/Toast";
import PomodoroTimer from "../../components/PomodoroTimer/PomodoroTimer";
import { todoStore } from "../../stores/TodoStore";
import {
  useTasksQuery,
  useStatsQuery,
  useUpdateTaskMutation,
  useDeleteTaskMutation,
} from "../../hooks/useTodos";
import "./MainPage.scss";
import CategoryForm from "../../components/CategoryForm/CategoryForm";

const MainPage = observer(() => {
  const { data: tasks = [], isLoading: tasksLoading } = useTasksQuery();
  const { data: stats } = useStatsQuery();

  console.log(stats);

  const updateMutation = useUpdateTaskMutation();
  const deleteMutation = useDeleteTaskMutation();

  const handleToggleStatus = (id: number, currentStatus: string) => {
    const newStatus = currentStatus === "done" ? "pending" : "done";
    updateMutation.mutate({ taskId: id, fields: { status: newStatus } });
  };

  const handleDelete = (id: number) => {
    if (window.confirm("Удалить задачу?")) {
      deleteMutation.mutate(id);
    }
  };

  const handleLogout = () => {
    localStorage.removeItem("token");
    window.location.href = "/auth";
  };

  // 4. Фильтрация на клиенте (как в твоем index.html)
  const displayTasks = todoStore.getFilteredTasks(tasks);

  return (
    <div className="main-layout">
      {/* Тосты теперь управляются глобально через MobX */}
      <Toast
        toasts={todoStore.toasts}
        onClose={(id) => {
          todoStore.toasts = todoStore.toasts.filter((t) => t.id !== id);
        }}
      />

      <div className="container">
        {/* Передаем реальную статистику с бэка */}
        <TaskHeader
          stats={{
            points: stats?.user_stats.points || 0,
            level: stats?.user_stats.level || 0,
            current_streak: stats?.user_stats.current_streak || 0,
          }}
          onLogout={handleLogout}
        />

        <div className="content-grid">
          <aside className="side-panel">
            <PomodoroTimer />

            {/* Сюда можно добавить блок фильтров, который будет менять todoStore.setFilter */}
            <div className="filter-panel">
              <button onClick={() => todoStore.setFilter("all")}>Все</button>
              <button onClick={() => todoStore.setFilter("pending")}>
                В работе
              </button>
              <button onClick={() => todoStore.setFilter("done")}>
                Готово
              </button>
            </div>
          </aside>

          <main className="main-panel">
            <TaskForm />
            <CategoryForm />
            <TaskList
              tasks={displayTasks}
              loading={tasksLoading}
              onToggle={handleToggleStatus}
              onDelete={handleDelete}
            />

            {/* Пагинация */}
            <div className="pagination-controls">
              <button
                disabled={todoStore.pagination.offset === 0}
                onClick={() => todoStore.changePage(-1)}
              >
                Назад
              </button>
              <button
                disabled={tasks.length < todoStore.pagination.limit}
                onClick={() => todoStore.changePage(1)}
              >
                Вперед
              </button>
            </div>
          </main>
        </div>
      </div>
    </div>
  );
});

export default MainPage;
