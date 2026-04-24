import "./TaskItem.scss";
import { type Task } from "../../types/task"; // Убедись, что путь к файлу верный

// 1. Описываем интерфейс пропсов
interface TaskItemProps {
  task: Task;
  onToggle: (id: number, currentStatus: string) => void;
  onDelete: (id: number) => void;
}

// 2. Используем интерфейс в компоненте
const TaskItem = ({ task, onToggle, onDelete }: TaskItemProps) => {
  // Типизируем аргумент функции форматирования
  const formatDate = (dateStr: string | null | undefined) => {
    if (!dateStr) return "";
    const d = new Date(dateStr);

    // Проверка на валидность даты, чтобы не выводить "Invalid Date"
    if (isNaN(d.getTime())) return "";

    return d.toLocaleString("ru-RU", {
      day: "2-digit",
      month: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
    });
  };

  return (
    <div className={`task-card ${task.status}`}>
      <div className="task-content">
        <div className="task-main-info">
          <h4>{task.title}</h4>
          {/* Добавил динамический класс для статуса, чтобы в CSS красить badge */}
          <span className={`status-badge ${task.status}`}>
            {task.status === "done" ? "Выполнено" : "В процессе"}
          </span>
        </div>

        <p>{task.description}</p>

        <div className="task-date-info">
          {/* Используем поля из твоего OpenAPI: deadline и reminder_at */}
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
          className="action-btn btn-complete"
          onClick={() => onToggle(task.id, task.status)}
          title={task.status === "done" ? "Вернуть в работу" : "Завершить"}
        >
          {task.status === "done" ? "♻️" : "✅"}
        </button>

        <button
          className="action-btn btn-delete"
          onClick={() => onDelete(task.id)}
          title="Удалить"
        >
          🗑️
        </button>
      </div>
    </div>
  );
};

export default TaskItem;
