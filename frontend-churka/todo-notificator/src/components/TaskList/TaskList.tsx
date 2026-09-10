import TaskItem from "../TaskItem/TaskItem";
import "./TaskList.scss";
import { type Task } from "../../types/task";
import { usePomodoro } from "../../hooks/usePomodoro";

interface TaskListProps {
  tasks: Task[];
  loading: boolean;
  onToggle: (id: number, currentStatus: string) => void;
  onDelete: (id: number) => void;
}

const TaskList: React.FC<TaskListProps> = ({
  tasks,
  loading,
  onToggle,
  onDelete,
}) => {
  const { startSession } = usePomodoro();

  if (loading) return <div className="loading-state">Загрузка задач...</div>;

  return (
    <div className="task-list-container">
      {tasks.length === 0 ? (
        <div className="empty-state">Задач пока нет. Отдохните! ☕</div>
      ) : (
        tasks.map((task) => (
          <TaskItem
            key={task.id}
            task={task}
            onToggle={onToggle}
            onDelete={onDelete}
            onPomodoro={(id) => startSession(id)}
          />
        ))
      )}
    </div>
  );
};

export default TaskList;
