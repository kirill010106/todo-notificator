import "./TaskItem.scss";

const TaskItem = ({ task, onToggle, onDelete }: any) => {
  // Функция для красивого формата даты (как в твоем старом проекте)
  const formatDate = (dateStr: string) => {
    if (!dateStr) return "";
    return new Date(dateStr).toLocaleString("ru-RU", {
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
          <span className="status-badge">
            {task.status === "done" ? "Выполнено" : "В процессе"}
          </span>
        </div>

        <p>{task.description}</p>

        <div className="task-date-info">
          <span className="task-date">📅 {formatDate(task.created_at)}</span>
          {task.notification_time && (
            <span className="task-time-badge">
              ⏰ {formatDate(task.notification_time)}
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
