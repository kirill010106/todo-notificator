import "./TaskItem.scss";
import { type Task } from "../../types/task";
import { useCategories } from "../../hooks/useCategories";
import dayjs from "dayjs";

interface TaskItemProps {
  task: Task;
  onToggle: (id: number, currentStatus: string) => void;
  onDelete: (id: number) => void;
  onPomodoro: (id: number) => void;
}

const TaskItem = ({ task, onToggle, onDelete, onPomodoro }: TaskItemProps) => {
  // Получаем список категорий из кэша TanStack Query
  const { data: categories = [] } = useCategories();

  // Находим нужную категорию по ID
  const category = categories.find((cat) => cat.id === task.category_id);

  const formatDate = (dateStr: string | null | undefined) => {
    if (!dateStr) return "";
    const d = dayjs(dateStr);
    return d.isValid() ? d.format("DD.MM HH:mm") : "";
  };

  return (
    <div className={`task-card ${task.status}`}>
      <div className="task-content">
        <div className="task-header-row">
          <div className="task-main-info">
            <h4>{task.title}</h4>
            <div className="task-badges">
              <span className={`status-badge ${task.status}`}>
                {task.status === "done" ? "Выполнено" : "В процессе"}
              </span>

              {/* Вывод категории, если она есть */}
              {category && (
                <span className="category-badge">{category.name}</span>
              )}
            </div>
          </div>
        </div>

        <p className="task-description">{task.description}</p>

        <div className="task-date-info">
          <span className="task-date">📅 {formatDate(task.deadline)}</span>
          {task.reminder_at && (
            <span className="task-time-badge">
              ⏰ {formatDate(task.reminder_at)}
            </span>
          )}
        </div>
      </div>

      <div className="task-actions">
        <button
          className="action-btn btn-pomodoro"
          onClick={() => onPomodoro(task.id)}
          title="Запустить фокус"
        >
          🍅
        </button>
        <button
          className="action-btn btn-complete"
          onClick={() => onToggle(task.id, task.status)}
        >
          {task.status === "done" ? "♻️" : "✅"}
        </button>
        <button
          className="action-btn btn-delete"
          onClick={() => onDelete(task.id)}
        >
          🗑️
        </button>
      </div>
    </div>
  );
};

export default TaskItem;
